package network

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
)

// Manager управляет соединением с сервером
type Manager struct {
	serverURL  string
	peerID     string
	networkID  string
	wsConn     *websocket.Conn
	httpClient *http.Client
	logger     *logrus.Logger
	connected  bool
}

// NewManager создает новый менеджер сети
func NewManager(serverURL, peerID string, logger *logrus.Logger) *Manager {
	return &Manager{
		serverURL: serverURL,
		peerID:    peerID,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: logger,
	}
}

// SetServerURL обновляет URL сервера
func (m *Manager) SetServerURL(url string) {
	m.serverURL = url
}

// CreateNetwork создает новую сеть
func (m *Manager) CreateNetwork(name, password string) (map[string]interface{}, error) {
	reqBody := map[string]string{
		"name":     name,
		"password": password,
	}

	resp, err := m.post("/api/v1/networks", reqBody)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

// JoinNetwork присоединяется к сети
func (m *Manager) JoinNetwork(networkID, password, publicKey, os string) (map[string]interface{}, error) {
	reqBody := map[string]interface{}{
		"network_id":  networkID,
		"password":    password,
		"public_key":  publicKey,
		"hostname":    getHostname(),
		"os":          os,
		"version":     "1.0.0",
		"listen_port": 0, // Авто
	}

	resp, err := m.post(fmt.Sprintf("/api/v1/networks/%s/join", networkID), reqBody)
	if err != nil {
		return nil, err
	}

	m.networkID = networkID
	m.connected = true

	// Подключаемся к WebSocket для real-time обновлений
	go m.connectWebSocket()

	return resp, nil
}

// Disconnect отключается от сети
func (m *Manager) Disconnect() {
	m.connected = false
	
	if m.wsConn != nil {
		m.wsConn.Close()
		m.wsConn = nil
	}

	// Отправляем запрос на выход
	if m.networkID != "" {
		m.post(fmt.Sprintf("/api/v1/networks/%s/leave", m.networkID), map[string]string{
			"peer_id": m.peerID,
		})
	}

	m.networkID = ""
}

// IsConnected возвращает статус подключения
func (m *Manager) IsConnected() bool {
	return m.connected
}

// GetNetworkID возвращает ID текущей сети
func (m *Manager) GetNetworkID() string {
	return m.networkID
}

// GetServerInfo получает информацию о сервере
func (m *Manager) GetServerInfo() (map[string]interface{}, error) {
	return m.get("/info")
}

// GetICEMessages получает ICE сообщения для пира
func (m *Manager) GetICEMessages() ([]map[string]interface{}, error) {
	if m.networkID == "" {
		return nil, fmt.Errorf("not connected to network")
	}

	url := fmt.Sprintf("/api/v1/ice/%s/receive?peer_id=%s", m.networkID, m.peerID)
	resp, err := m.get(url)
	if err != nil {
		return nil, err
	}

	messages, ok := resp["messages"].([]interface{})
	if !ok {
		return []map[string]interface{}{}, nil
	}

	result := make([]map[string]interface{}, 0, len(messages))
	for _, msg := range messages {
		if m, ok := msg.(map[string]interface{}); ok {
			result = append(result, m)
		}
	}

	return result, nil
}

// SendICEMessage отправляет ICE сообщение
func (m *Manager) SendICEMessage(targetPeerID, msgType, payload string) error {
	if m.networkID == "" {
		return fmt.Errorf("not connected to network")
	}

	reqBody := map[string]string{
		"peer_id":        m.peerID,
		"target_peer_id": targetPeerID,
		"type":           msgType,
		"payload":        payload,
	}

	url := fmt.Sprintf("/api/v1/ice/%s/send", m.networkID)
	_, err := m.post(url, reqBody)
	return err
}

// connectWebSocket подключается к WebSocket
func (m *Manager) connectWebSocket() {
	wsURL := m.serverURL + "/ws"
	// Заменяем http на ws
	if len(wsURL) > 4 && wsURL[:4] == "http" {
		wsURL = "ws" + wsURL[4:]
	}

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		m.logger.Errorf("Failed to connect WebSocket: %v", err)
		return
	}

	m.wsConn = conn

	// Отправляем регистрацию
	reg := map[string]string{
		"type":       "register",
		"peer_id":    m.peerID,
		"network_id": m.networkID,
	}
	if err := conn.WriteJSON(reg); err != nil {
		m.logger.Errorf("Failed to send registration: %v", err)
		conn.Close()
		return
	}

	m.logger.Info("WebSocket connected")

	// Читаем сообщения
	for m.connected {
		var msg map[string]interface{}
		if err := conn.ReadJSON(&msg); err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				m.logger.Errorf("WebSocket error: %v", err)
			}
			break
		}
		
		m.handleWebSocketMessage(msg)
	}

	m.logger.Info("WebSocket disconnected")
}

// handleWebSocketMessage обрабатывает сообщение от WebSocket
func (m *Manager) handleWebSocketMessage(msg map[string]interface{}) {
	msgType, _ := msg["type"].(string)
	
	switch msgType {
	case "peer_list":
		m.logger.Debug("Received peer list update")
	case "offer", "answer", "ice_candidate":
		// ICE signaling - передаем выше
		m.logger.Debugf("Received ICE message: %s", msgType)
	case "heartbeat":
		// Игнорируем
	default:
		m.logger.Debugf("Received message: %s", msgType)
	}
}

// HTTP helpers

func (m *Manager) get(path string) (map[string]interface{}, error) {
	url := m.serverURL + path
	
	resp, err := m.httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result, nil
}

func (m *Manager) post(path string, body interface{}) (map[string]interface{}, error) {
	url := m.serverURL + path
	
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	resp, err := m.httpClient.Post(url, "application/json", bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var result map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		return nil, err
	}

	return result, nil
}

func getHostname() string {
	// В MVP просто возвращаем имя компьютера
	return "meshvpn-peer"
}
