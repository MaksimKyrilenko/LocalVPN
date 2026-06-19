package vpn

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/curve25519"
)

// Manager управляет WireGuard соединением
type Manager struct {
	privateKey string
	publicKey  string
	virtualIP  string
	peers      map[string]*PeerInfo
	mu         sync.RWMutex
	logger     *logrus.Logger
	ifaceName  string
}

// PeerConfig конфигурация пира
type PeerConfig struct {
	ID        string `json:"id"`
	PublicKey string `json:"public_key"`
	VirtualIP string `json:"virtual_ip"`
	Endpoint  string `json:"endpoint,omitempty"`
}

// PeerInfo информация о пире
type PeerInfo struct {
	ID           string    `json:"id"`
	PublicKey    string    `json:"public_key"`
	VirtualIP    string    `json:"virtual_ip"`
	Endpoint     string    `json:"endpoint,omitempty"`
	Connected    bool      `json:"connected"`
	LastSeen     time.Time `json:"last_seen"`
	Latency      int       `json:"latency_ms"`
}

// NewManager создает новый VPN менеджер
func NewManager(logger *logrus.Logger) *Manager {
	return &Manager{
		peers:     make(map[string]*PeerInfo),
		logger:    logger,
		ifaceName: "meshvpn0",
	}
}

// GenerateKeys генерирует ключевую пару WireGuard
func (m *Manager) GenerateKeys() error {
	// Генерируем приватный ключ
	var privateKey [32]byte
	if _, err := rand.Read(privateKey[:]); err != nil {
		return fmt.Errorf("failed to generate private key: %w", err)
	}

	// Корректируем для Curve25519
	privateKey[0] &= 248
	privateKey[31] &= 127
	privateKey[31] |= 64

	// Вычисляем публичный ключ
	var publicKey [32]byte
	curve25519.ScalarBaseMult(&publicKey, &privateKey)

	m.privateKey = base64.StdEncoding.EncodeToString(privateKey[:])
	m.publicKey = base64.StdEncoding.EncodeToString(publicKey[:])

	m.logger.Info("Generated WireGuard keys")
	return nil
}

// GetPublicKey возвращает публичный ключ
func (m *Manager) GetPublicKey() string {
	return m.publicKey
}

// GetVirtualIP возвращает виртуальный IP
func (m *Manager) GetVirtualIP() string {
	return m.virtualIP
}

// ConfigureInterface настраивает TUN интерфейс
func (m *Manager) ConfigureInterface(virtualIP string, peers []PeerConfig) error {
	m.virtualIP = virtualIP

	// Определяем платформу и вызываем соответствующий метод
	switch runtime.GOOS {
	case "windows":
		return m.configureWindows()
	case "linux":
		return m.configureLinux()
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

// configureWindows настраивает Windows интерфейс
func (m *Manager) configureWindows() error {
	// На Windows используем WireGuard userspace через wintun
	// В MVP делаем упрощенную версию через netsh
	
	m.logger.Info("Configuring Windows TUN interface")
	
	// Удаляем старый интерфейс если есть
	exec.Command("netsh", "interface", "delete", "interface", m.ifaceName).Run()

	// Создаем TUN интерфейс через wintun (требуется wintun.dll)
	// В полной реализации используем wireguard-windows
	
	// Настраиваем IP
	cmd := exec.Command("netsh", "interface", "ip", "set", "address",
		m.ifaceName, "static", m.virtualIP, "255.255.255.0")
	if output, err := cmd.CombinedOutput(); err != nil {
		m.logger.Warnf("Failed to set IP: %v, output: %s", err, string(output))
		// Не фатальная ошибка в MVP
	}

	m.logger.Infof("Interface %s configured with IP %s", m.ifaceName, m.virtualIP)
	return nil
}

// configureLinux настраивает Linux интерфейс
func (m *Manager) configureLinux() error {
	m.logger.Info("Configuring Linux TUN interface")
	
	// Создаем TUN интерфейс
	cmd := exec.Command("ip", "tuntap", "add", "dev", m.ifaceName, "mode", "tun")
	if output, err := cmd.CombinedOutput(); err != nil {
		// Интерфейс может уже существовать
		m.logger.Debugf("TUN add output: %s", string(output))
	}

	// Настраиваем IP
	cmd = exec.Command("ip", "addr", "add", m.virtualIP+"/24", "dev", m.ifaceName)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to add IP: %v, output: %s", err, string(output))
	}

	// Поднимаем интерфейс
	cmd = exec.Command("ip", "link", "set", "dev", m.ifaceName, "up")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to bring up interface: %v, output: %s", err, string(output))
	}

	m.logger.Infof("Interface %s configured with IP %s", m.ifaceName, m.virtualIP)
	return nil
}

// Cleanup удаляет VPN конфигурацию
func (m *Manager) Cleanup() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.peers = make(map[string]*PeerInfo)
	m.virtualIP = ""

	switch runtime.GOOS {
	case "windows":
		exec.Command("netsh", "interface", "delete", "interface", m.ifaceName).Run()
	case "linux":
		exec.Command("ip", "link", "delete", m.ifaceName).Run()
	}

	m.logger.Info("VPN cleanup completed")
	return nil
}

// AddPeer добавляет пира
func (m *Manager) AddPeer(config PeerConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.peers[config.ID] = &PeerInfo{
		ID:        config.ID,
		PublicKey: config.PublicKey,
		VirtualIP: config.VirtualIP,
		Endpoint:  config.Endpoint,
		Connected: true,
		LastSeen:  time.Now(),
	}

	m.logger.Infof("Added peer %s (%s)", config.ID, config.VirtualIP)
}

// RemovePeer удаляет пира
func (m *Manager) RemovePeer(peerID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.peers, peerID)
	m.logger.Infof("Removed peer %s", peerID)
}

// UpdatePeerEndpoint обновляет endpoint пира
func (m *Manager) UpdatePeerEndpoint(peerID, endpoint string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if peer, ok := m.peers[peerID]; ok {
		peer.Endpoint = endpoint
		peer.LastSeen = time.Now()
	}
}

// GetPeers возвращает список пиров
func (m *Manager) GetPeers() []PeerInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]PeerInfo, 0, len(m.peers))
	for _, peer := range m.peers {
		result = append(result, *peer)
	}
	return result
}

// Ping отправляет пинг на виртуальный IP
func (m *Manager) Ping(targetIP string) (string, error) {
	var cmd *exec.Cmd
	
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("ping", "-n", "4", "-w", "1000", targetIP)
	case "linux":
		cmd = exec.Command("ping", "-c", "4", "-W", "1", targetIP)
	default:
		return "", fmt.Errorf("unsupported platform")
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("ping failed: %w", err)
	}

	return string(output), nil
}

// GetPrivateKey возвращает приватный ключ (осторожно!)
func (m *Manager) GetPrivateKey() string {
	return m.privateKey
}

// GetWireGuardConfig генерирует конфиг WireGuard
func (m *Manager) GetWireGuardConfig() string {
	var sb strings.Builder
	
	// [Interface] секция
	sb.WriteString("[Interface]\n")
	sb.WriteString(fmt.Sprintf("PrivateKey = %s\n", m.privateKey))
	sb.WriteString(fmt.Sprintf("Address = %s/24\n", m.virtualIP))
	sb.WriteString("ListenPort = 0\n") // Авто
	sb.WriteString("DNS = 1.1.1.1, 8.8.8.8\n")
	sb.WriteString("\n")
	
	// [Peer] секции
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	for _, peer := range m.peers {
		sb.WriteString("[Peer]\n")
		sb.WriteString(fmt.Sprintf("PublicKey = %s\n", peer.PublicKey))
		if peer.Endpoint != "" {
		sb.WriteString(fmt.Sprintf("Endpoint = %s\n", peer.Endpoint))
		}
		sb.WriteString(fmt.Sprintf("AllowedIPs = %s/32\n", peer.VirtualIP))
		sb.WriteString("PersistentKeepalive = 25\n")
		sb.WriteString("\n")
	}
	
	return sb.String()
}

// IsVirtualIP проверяет, является ли IP виртуальным
func IsVirtualIP(ip string, subnet string) bool {
	_, virtualNet, err := net.ParseCIDR(subnet)
	if err != nil {
		return false
	}
	
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false
	}
	
	return virtualNet.Contains(parsedIP)
}
