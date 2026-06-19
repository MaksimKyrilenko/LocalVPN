package main

import (
	"context"
	"flag"
	"meshvpn/server/internal/api"
	"meshvpn/server/internal/db"
	"meshvpn/server/internal/stun"
	"meshvpn/server/internal/websocket"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
)

func main() {
	// Парсим флаги
	var (
		httpAddr = flag.String("http", ":8080", "HTTP server address")
		dbPath   = flag.String("db", "./data/meshvpn.db", "Database path")
		stunAddr = flag.String("stun", ":3478", "STUN server address")
		turnAddr = flag.String("turn", "", "TURN server address (optional)")
		debug    = flag.Bool("debug", false, "Enable debug logging")
	)
	flag.Parse()

	// Настройка логгера
	logger := logrus.New()
	if *debug {
		logger.SetLevel(logrus.DebugLevel)
	}
	logger.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})

	logger.Info("Starting MeshVPN Server...")

	// Инициализация БД
	logger.Infof("Opening database: %s", *dbPath)
	database, err := db.NewDatabase(*dbPath)
	if err != nil {
		logger.Fatalf("Failed to initialize database: %v", err)
	}
	defer database.Close()

	// Создаем WebSocket Hub
	hub := websocket.NewHub(logger)
	go hub.Run()

	// Создаем STUN сервер
	stunServer := stun.NewServer(*stunAddr, logger)
	if err := stunServer.Start(); err != nil {
		logger.Errorf("Failed to start STUN server: %v", err)
		// Не фатальная ошибка - можно работать без STUN
	}
	defer stunServer.Stop()

	// Конфигурация API сервера
	apiConfig := &api.Config{
		Port:     *httpAddr,
		DBPath:   *dbPath,
		STUNAddr: *stunAddr,
		TURNAddr: *turnAddr,
	}

	// Создаем и запускаем API сервер
	apiServer := api.NewServer(apiConfig, database, hub, logger)

	// Запускаем в отдельной goroutine
	go func() {
		logger.Infof("HTTP server starting on %s", *httpAddr)
		if err := apiServer.Run(*httpAddr); err != nil && err != http.ErrServerClosed {
			logger.Fatalf("Failed to start HTTP server: %v", err)
		}
	}()

	// Запускаем cleanup горутину
	go cleanupTask(database, logger)

	// Ждем сигнала завершения
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down server...")

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Здесь можно добавить graceful shutdown HTTP сервера если нужно
	_ = ctx

	logger.Info("Server stopped")
}

// cleanupTask периодически очищает неактивных пиров
func cleanupTask(db *db.Database, logger *logrus.Logger) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		if err := db.CleanupDisconnectedPeers(24 * time.Hour); err != nil {
			logger.Errorf("Cleanup failed: %v", err)
		} else {
			logger.Debug("Cleanup completed")
		}
	}
}
