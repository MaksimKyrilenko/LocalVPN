package api

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"meshvpn/server/internal/db"
	"meshvpn/server/internal/models"
	"meshvpn/server/internal/websocket"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

// Server представляет HTTP API сервер
type Server struct {
	router    *gin.Engine
	db        *db.Database
	hub       *websocket.Hub
	logger    *logrus.Logger
	stunAddr  string
	turnAddr  string
}

// Config конфигурация сервера
type Config struct {
	Port     string
	DBPath   string
	STUNAddr string
	TURNAddr string
}

// NewServer создает новый API сервер
func NewServer(cfg *Config, database *db.Database, hub *websocket.Hub, logger *logrus.Logger) *Server {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(corsMiddleware())
	router.Use(requestLogger(logger))

	s := &Server{
		router:   router,
		db:       database,
		hub:      hub,
		logger:   logger,
		stunAddr: cfg.STUNAddr,
		turnAddr: cfg.TURNAddr,
	}

	s.setupRoutes()
	return s
}

// setupRoutes настраивает маршруты
func (s *Server) setupRoutes() {
	// Health check
	s.router.GET("/health", s.healthCheck)

	// API v1
	api := s.router.Group("/api/v1")
	{
		api.POST("/networks", s.createNetwork)
		api.GET("/networks/:id", s.getNetwork)
		api.POST("/networks/:id/join", s.joinNetwork)
		api.DELETE("/networks/:id/leave", s.leaveNetwork)
		api.GET("/networks/:id/peers", s.getPeers)
		
		api.POST("/peers/:id/heartbeat", s.peerHeartbeat)
		
		// ICE signaling (polling версия)
		api.POST("/ice/:network_id/send", s.sendICE)
		api.GET("/ice/:network_id/receive", s.receiveICE)
	}

	// WebSocket endpoint
	s.router.GET("/ws", func(c *gin.Context) {
		websocket.HandleWebSocket(s.hub, c.Writer, c.Request)
	})

	// Server info
	s.router.GET("/info", s.serverInfo)
}

// Run запускает сервер
func (s *Server) Run(addr string) error {
	return s.router.Run(addr)
}

// healthCheck проверка здоровья
func (s *Server) healthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "healthy",
		"time":   time.Now().Unix(),
	})
}

// serverInfo информация о сервере
func (s *Server) serverInfo(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"stun":    s.stunAddr,
		"turn":    s.turnAddr,
		"version": "1.0.0",
	})
}

// createNetwork создает новую сеть
func (s *Server) createNetwork(c *gin.Context) {
	var req models.CreateNetworkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Хешируем пароль
	passwordHash := hashPassword(req.Password)

	// Генерируем уникальный ID сети
	networkID := generateNetworkID()

	// Выделяем подсеть (простой алгоритм - в продакшене нужен более умный)
	subnet := fmt.Sprintf("10.%d.%d.0/24", uuid.New().ID()%256, uuid.New().ID()%256)

	network := &models.Network{
		ID:           networkID,
		Name:         req.Name,
		PasswordHash: passwordHash,
		Subnet:       subnet,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	if err := s.db.CreateNetwork(network); err != nil {
		s.logger.Errorf("Failed to create network: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create network"})
		return
	}

	s.logger.Infof("Created network: %s (%s)", network.ID, network.Name)
	
	c.JSON(http.StatusCreated, gin.H{
		"network_id": network.ID,
		"name":       network.Name,
		"subnet":     network.Subnet,
		"message":    "Network created successfully. Share the Network ID and password with others.",
	})
}

// getNetwork получает информацию о сети
func (s *Server) getNetwork(c *gin.Context) {
	networkID := c.Param("id")
	
	network, err := s.db.GetNetwork(networkID)
	if err != nil {
		s.logger.Errorf("Failed to get network: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}
	if network == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Network not found"})
		return
	}

	// Получаем список пиров
	peers, err := s.db.GetPeersByNetwork(networkID)
	if err != nil {
		s.logger.Errorf("Failed to get peers: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	c.JSON(http.StatusOK, models.NetworkResponse{
		Network: *network,
		Peers:   peers,
	})
}

// joinNetwork присоединение к сети
func (s *Server) joinNetwork(c *gin.Context) {
	networkID := c.Param("id")
	
	var req models.JoinNetworkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Проверяем сеть и пароль
	passwordHash := hashPassword(req.Password)
	network, err := s.db.GetNetworkByPassword(networkID, passwordHash)
	if err != nil {
		s.logger.Errorf("Failed to verify network: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}
	if network == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid network ID or password"})
		return
	}

	// Проверяем, существует ли уже пир с таким ключом
	existingPeer, err := s.db.GetPeerByPublicKey(req.PublicKey)
	if err != nil {
		s.logger.Errorf("Failed to check peer: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	var peerID string
	var virtualIP string

	if existingPeer != nil {
		// Обновляем существующего пира
		peerID = existingPeer.ID
		virtualIP = existingPeer.VirtualIP
		s.db.UpdatePeerStatus(peerID, true, c.ClientIP())
	} else {
		// Создаем нового пира
		peerID = uuid.New().String()
		virtualIP = generateVirtualIP(network.Subnet)

		peer := &models.Peer{
			ID:        peerID,
			NetworkID: networkID,
			PublicKey: req.PublicKey,
			VirtualIP: virtualIP,
			Hostname:  req.Hostname,
			OS:        req.OS,
			Version:   req.Version,
			ListenPort: req.ListenPort,
			Connected: true,
			LastSeen:  time.Now(),
			CreatedAt: time.Now(),
		}

		if err := s.db.CreatePeer(peer); err != nil {
			s.logger.Errorf("Failed to create peer: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create peer"})
			return
		}
	}

	// Получаем список других пиров в сети
	allPeers, err := s.db.GetPeersByNetwork(networkID)
	if err != nil {
		s.logger.Errorf("Failed to get peers: %v", err)
	}

	// Фильтруем текущего пира
	var otherPeers []models.Peer
	for _, p := range allPeers {
		if p.ID != peerID {
			otherPeers = append(otherPeers, p)
		}
	}

	s.logger.Infof("Peer %s joined network %s", peerID, networkID)

	c.JSON(http.StatusOK, gin.H{
		"peer_id":    peerID,
		"network_id": networkID,
		"virtual_ip": virtualIP,
		"subnet":     network.Subnet,
		"stun":       s.stunAddr,
		"turn":       s.turnAddr,
		"peers":      otherPeers,
	})
}

// leaveNetwork выход из сети
func (s *Server) leaveNetwork(c *gin.Context) {
	peerID := c.Query("peer_id")
	if peerID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "peer_id required"})
		return
	}

	if err := s.db.UpdatePeerStatus(peerID, false, ""); err != nil {
		s.logger.Errorf("Failed to update peer status: %v", err)
	}

	s.logger.Infof("Peer %s left network", peerID)
	c.JSON(http.StatusOK, gin.H{"message": "Left network successfully"})
}

// getPeers получает список пиров в сети
func (s *Server) getPeers(c *gin.Context) {
	networkID := c.Param("id")
	
	peers, err := s.db.GetPeersByNetwork(networkID)
	if err != nil {
		s.logger.Errorf("Failed to get peers: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"peers": peers})
}

// peerHeartbeat обновление статуса пира
func (s *Server) peerHeartbeat(c *gin.Context) {
	peerID := c.Param("id")
	
	if err := s.db.UpdatePeerStatus(peerID, true, c.ClientIP()); err != nil {
		s.logger.Errorf("Failed to update heartbeat: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// sendICE отправляет ICE сообщение
func (s *Server) sendICE(c *gin.Context) {
	networkID := c.Param("network_id")
	
	var msg models.PeerICE
	if err := c.ShouldBindJSON(&msg); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	msg.ID = uuid.New().String()
	msg.NetworkID = networkID
	msg.CreatedAt = time.Now()

	if err := s.db.SaveICEMessage(&msg); err != nil {
		s.logger.Errorf("Failed to save ICE message: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save message"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "ICE message sent"})
}

// receiveICE получает ICE сообщения
func (s *Server) receiveICE(c *gin.Context) {
	targetPeerID := c.Query("peer_id")
	if targetPeerID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "peer_id required"})
		return
	}

	// Получаем сообщения за последние 30 секунд
	since := time.Now().Add(-30 * time.Second)
	
	messages, err := s.db.GetICEMessages(targetPeerID, since)
	if err != nil {
		s.logger.Errorf("Failed to get ICE messages: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"messages": messages})
}

// Вспомогательные функции

func hashPassword(password string) string {
	hash := sha256.Sum256([]byte(password))
	return hex.EncodeToString(hash[:])
}

func generateNetworkID() string {
	// Генерируем короткий, запоминающийся ID
	return uuid.New().String()[:8]
}

func generateVirtualIP(subnet string) string {
	// Простая генерация IP из подсети
	// В продакшене нужна проверка занятости
	return fmt.Sprintf("10.%d.%d.%d", 
		uuid.New().ID()%256, 
		uuid.New().ID()%256, 
		2+uuid.New().ID()%253)
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		
		c.Next()
	}
}

func requestLogger(logger *logrus.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		raw := c.Request.URL.RawQuery

		c.Next()

		latency := time.Since(start)
		clientIP := c.ClientIP()
		method := c.Request.Method
		statusCode := c.Writer.Status()

		if raw != "" {
			path = path + "?" + raw
		}

		logger.Infof("[%s] %s %s %d %v", method, path, clientIP, statusCode, latency)
	}
}
