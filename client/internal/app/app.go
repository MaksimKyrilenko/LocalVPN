package app

import (
	"context"
	"encoding/json"
	"fmt"
	"meshvpn/client/internal/config"
	"meshvpn/client/internal/network"
	"meshvpn/client/internal/vpn"
	"runtime"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// App основная структура приложения
type App struct {
	ctx       context.Context
	config    *config.Config
	vpnMgr    *vpn.Manager
	networkMgr *network.Manager
	logger    *logrus.Logger
	peerID    string
}

// NewApp создает новое приложение
func NewApp() *App {
	logger := logrus.New()
	logger.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})

	return &App{
		logger: logger,
		peerID: uuid.New().String(),
	}
}

// Startup вызывается при старте приложения
func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
	
	// Загружаем конфигурацию
	cfg, err := config.Load()
	if err != nil {
		a.logger.Warnf("Failed to load config: %v", err)
		cfg = config.DefaultConfig()
	}
	a.config = cfg

	// Инициализируем VPN менеджер
	a.vpnMgr = vpn.NewManager(a.logger)
	
	// Инициализируем сетевой менеджер
	a.networkMgr = network.NewManager(a.config.ServerURL, a.peerID, a.logger)

	a.logger.Info("MeshVPN Client started")
	a.logger.Infof("Peer ID: %s", a.peerID)
}

// Shutdown вызывается при завершении
func (a *App) Shutdown(ctx context.Context) {
	if a.networkMgr != nil {
		a.networkMgr.Disconnect()
	}
	if a.vpnMgr != nil {
		a.vpnMgr.Cleanup()
	}
	a.logger.Info("MeshVPN Client stopped")
}

// GetAppInfo возвращает информацию о приложении
func (a *App) GetAppInfo() map[string]interface{} {
	return map[string]interface{}{
		"version":   "1.0.0",
		"peer_id":   a.peerID,
		"platform":  runtime.GOOS,
		"connected": a.networkMgr != nil && a.networkMgr.IsConnected(),
	}
}

// GetConfig возвращает текущую конфигурацию
func (a *App) GetConfig() config.Config {
	return *a.config
}

// SaveConfig сохраняет конфигурацию
func (a *App) SaveConfig(cfg config.Config) error {
	if err := cfg.Save(); err != nil {
		return err
	}
	a.config = &cfg
	
	// Обновляем URL сервера в менеджере
	if a.networkMgr != nil {
		a.networkMgr.SetServerURL(cfg.ServerURL)
	}
	
	return nil
}

// CreateNetwork создает новую сеть
func (a *App) CreateNetwork(name, password string) (map[string]interface{}, error) {
	if a.networkMgr == nil {
		return nil, fmt.Errorf("network manager not initialized")
	}

	resp, err := a.networkMgr.CreateNetwork(name, password)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

// JoinNetwork присоединяется к сети
func (a *App) JoinNetwork(networkID, password string) (map[string]interface{}, error) {
	if a.networkMgr == nil {
		return nil, fmt.Errorf("network manager not initialized")
	}

	// Генерируем ключи WireGuard если нужно
	if err := a.vpnMgr.GenerateKeys(); err != nil {
		return nil, fmt.Errorf("failed to generate keys: %w", err)
	}

	publicKey := a.vpnMgr.GetPublicKey()
	
	resp, err := a.networkMgr.JoinNetwork(networkID, password, publicKey, runtime.GOOS)
	if err != nil {
		return nil, err
	}

	// Настраиваем VPN интерфейс
	virtualIP, ok := resp["virtual_ip"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid response from server")
	}

	peersData, _ := json.Marshal(resp["peers"])
	var peers []vpn.PeerConfig
	if err := json.Unmarshal(peersData, &peers); err != nil {
		a.logger.Warnf("Failed to parse peers: %v", err)
	}

	if err := a.vpnMgr.ConfigureInterface(virtualIP, peers); err != nil {
		return nil, fmt.Errorf("failed to configure VPN: %w", err)
	}

	// Запускаем обмен ICE с пирами
	go a.startICESignaling(resp["peers"])

	// Обновляем UI
	wailsruntime.EventsEmit(a.ctx, "network:joined", resp)

	return resp, nil
}

// LeaveNetwork покидает сеть
func (a *App) LeaveNetwork() error {
	if a.networkMgr != nil {
		a.networkMgr.Disconnect()
	}
	if a.vpnMgr != nil {
		a.vpnMgr.Cleanup()
	}
	
	wailsruntime.EventsEmit(a.ctx, "network:left", nil)
	return nil
}

// GetNetworkStatus возвращает статус сети
func (a *App) GetNetworkStatus() map[string]interface{} {
	if a.networkMgr == nil {
		return map[string]interface{}{
			"connected": false,
			"message":   "Not initialized",
		}
	}

	return map[string]interface{}{
		"connected":    a.networkMgr.IsConnected(),
		"network_id":   a.networkMgr.GetNetworkID(),
		"virtual_ip":   a.vpnMgr.GetVirtualIP(),
		"peers":        a.vpnMgr.GetPeers(),
		"server_url":   a.config.ServerURL,
	}
}

// GetPeers возвращает список пиров
func (a *App) GetPeers() []vpn.PeerInfo {
	if a.vpnMgr == nil {
		return nil
	}
	return a.vpnMgr.GetPeers()
}

// SendTestPing отправляет тестовый пинг
func (a *App) SendTestPing(targetIP string) (string, error) {
	result, err := a.vpnMgr.Ping(targetIP)
	if err != nil {
		return "", err
	}
	return result, nil
}

// TestServerConnection тестирует соединение с сервером
func (a *App) TestServerConnection(serverURL string) (map[string]interface{}, error) {
	mgr := network.NewManager(serverURL, a.peerID, a.logger)
	info, err := mgr.GetServerInfo()
	if err != nil {
		return nil, err
	}
	return info, nil
}

// startICESignaling запускает ICE сигналинг с пирами
func (a *App) startICESignaling(peers interface{}) {
	// Здесь реализуется ICE signaling для P2P
	// В MVP используем polling через HTTP
	
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		if !a.networkMgr.IsConnected() {
			return
		}

		// Получаем ICE сообщения
		messages, err := a.networkMgr.GetICEMessages()
		if err != nil {
			a.logger.Debugf("Failed to get ICE messages: %v", err)
			continue
		}

		// Обрабатываем сообщения
		for _, msg := range messages {
			a.handleICEMessage(msg)
		}

		// Обновляем UI
		wailsruntime.EventsEmit(a.ctx, "peers:updated", a.vpnMgr.GetPeers())
	}
}

// handleICEMessage обрабатывает ICE сообщение
func (a *App) handleICEMessage(msg map[string]interface{}) {
	// Обработка ICE offer/answer/candidate
	// В MVP упрощаем - просто логируем
	a.logger.Debugf("ICE message: %+v", msg)
}

// ShowError показывает ошибку в UI
func (a *App) ShowError(message string) {
	wailsruntime.MessageDialog(a.ctx, wailsruntime.MessageDialogOptions{
		Type:    wailsruntime.ErrorDialog,
		Title:   "Error",
		Message: message,
	})
}

// ShowInfo показывает информацию в UI
func (a *App) ShowInfo(title, message string) {
	wailsruntime.MessageDialog(a.ctx, wailsruntime.MessageDialogOptions{
		Type:    wailsruntime.InfoDialog,
		Title:   title,
		Message: message,
	})
}
