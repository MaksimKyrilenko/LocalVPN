package websocket

import (
	"encoding/json"
	"meshvpn/server/internal/models"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
)

// Hub управляет WebSocket соединениями
type Hub struct {
	clients    map[string]*Client // peer_id -> Client
	broadcast  chan *models.Message
	register   chan *Client
	unregister chan *Client
	mu         sync.RWMutex
	logger     *logrus.Logger
	upgrader   websocket.Upgrader
}

// Client представляет WebSocket клиента
type Client struct {
	hub       *Hub
	conn      *websocket.Conn
	send      chan []byte
	peerID    string
	networkID string
}

// NewHub создает новый Hub
func NewHub(logger *logrus.Logger) *Hub {
	return &Hub{
		clients:    make(map[string]*Client),
		broadcast:  make(chan *models.Message),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		logger:     logger,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				return true // Разрешаем все origin (в продакшене настроить строже)
			},
		},
	}
}

// Run запускает Hub
func (h *Hub) Run() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client.peerID] = client
			h.mu.Unlock()
			h.logger.Infof("Client registered: %s (network: %s)", client.peerID, client.networkID)

			// Отправляем подтверждение регистрации
			ack := &models.Message{
				Type:      "registered",
				From:      "server",
				Payload:   map[string]string{"peer_id": client.peerID},
				Timestamp: time.Now().Unix(),
			}
			client.send <- mustJSON(ack)

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client.peerID]; ok {
				delete(h.clients, client.peerID)
				close(client.send)
			}
			h.mu.Unlock()
			h.logger.Infof("Client unregistered: %s", client.peerID)

		case message := <-h.broadcast:
			h.broadcastMessage(message)

		case <-ticker.C:
			h.sendHeartbeats()
		}
	}
}

// broadcastMessage отправляет сообщение целевым клиентам
func (h *Hub) broadcastMessage(msg *models.Message) {
	data, err := json.Marshal(msg)
	if err != nil {
		h.logger.Errorf("Failed to marshal message: %v", err)
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	// Если указан конкретный получатель
	if msg.To != "" {
		if client, ok := h.clients[msg.To]; ok {
			select {
			case client.send <- data:
			default:
				close(client.send)
				delete(h.clients, client.peerID)
			}
		}
		return
	}

	// Иначе рассылаем всем в сети
	for _, client := range h.clients {
		if client.networkID == msg.NetworkID && client.peerID != msg.From {
			select {
			case client.send <- data:
			default:
				close(client.send)
				delete(h.clients, client.peerID)
			}
		}
	}
}

// sendHeartbeats отправляет heartbeat всем клиентам
func (h *Hub) sendHeartbeats() {
	h.mu.RLock()
	defer h.mu.RUnlock()

	msg := &models.Message{
		Type:      string(models.SignalHeartbeat),
		From:      "server",
		Payload:   map[string]int64{"timestamp": time.Now().Unix()},
		Timestamp: time.Now().Unix(),
	}
	data := mustJSON(msg)

	for _, client := range h.clients {
		select {
		case client.send <- data:
		default:
			close(client.send)
			delete(h.clients, client.peerID)
		}
	}
}

// HandleWebSocket обрабатывает WebSocket соединение
func HandleWebSocket(hub *Hub, w http.ResponseWriter, r *http.Request) {
	conn, err := hub.upgrader.Upgrade(w, r, nil)
	if err != nil {
		hub.logger.Errorf("Failed to upgrade connection: %v", err)
		return
	}

	// Ожидаем первое сообщение с регистрацией
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	
	var regMsg struct {
		Type      string `json:"type"`
		PeerID    string `json:"peer_id"`
		NetworkID string `json:"network_id"`
	}

	if err := conn.ReadJSON(&regMsg); err != nil {
		hub.logger.Errorf("Failed to read registration: %v", err)
		conn.Close()
		return
	}

	if regMsg.Type != "register" || regMsg.PeerID == "" {
		hub.logger.Error("Invalid registration message")
		conn.Close()
		return
	}

	conn.SetReadDeadline(time.Time{})

	client := &Client{
		hub:       hub,
		conn:      conn,
		send:      make(chan []byte, 256),
		peerID:    regMsg.PeerID,
		networkID: regMsg.NetworkID,
	}

	hub.register <- client

	// Запускаем goroutines для чтения и записи
	go client.writePump()
	go client.readPump()
}

// readPump читает сообщения от клиента
func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(65536)
	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		var msg models.Message
		err := c.conn.ReadJSON(&msg)
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				c.hub.logger.Errorf("WebSocket error: %v", err)
			}
			break
		}

		msg.From = c.peerID
		msg.NetworkID = c.networkID
		msg.Timestamp = time.Now().Unix()

		// Обрабатываем разные типы сообщений
		switch msg.Type {
		case string(models.SignalJoinNetwork):
			c.hub.logger.Infof("Peer %s joined network %s", c.peerID, c.networkID)
			c.hub.broadcast <- &msg

		case string(models.SignalLeaveNetwork):
			c.hub.logger.Infof("Peer %s left network %s", c.peerID, c.networkID)
			c.hub.broadcast <- &msg

		case string(models.SignalOffer), string(models.SignalAnswer), string(models.SignalICE):
			// ICE signaling - пересылаем целевому пиру
			c.hub.broadcast <- &msg

		case string(models.SignalHeartbeat):
			// Heartbeat - просто обновляем deadline
			c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))

		default:
			c.hub.logger.Warnf("Unknown message type: %s", msg.Type)
		}
	}
}

// writePump пишет сообщения клиенту
func (c *Client) writePump() {
	ticker := time.NewTicker(54 * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			// Добавляем буферированные сообщения
			n := len(c.send)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'})
				w.Write(<-c.send)
			}

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// mustJSON сериализует сообщение в JSON (паника при ошибке)
func mustJSON(msg *models.Message) []byte {
	data, err := json.Marshal(msg)
	if err != nil {
		panic(err)
	}
	return data
}
