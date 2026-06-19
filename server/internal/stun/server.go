package stun

import (
	"fmt"
	"net"

	"github.com/pion/stun/v2"
	"github.com/sirupsen/logrus"
)

// Server STUN сервер для определения публичного адреса
type Server struct {
	addr   string
	logger *logrus.Logger
	conn   *net.UDPConn
}

// NewServer создает новый STUN сервер
func NewServer(addr string, logger *logrus.Logger) *Server {
	return &Server{
		addr:   addr,
		logger: logger,
	}
}

// Start запускает STUN сервер
func (s *Server) Start() error {
	udpAddr, err := net.ResolveUDPAddr("udp", s.addr)
	if err != nil {
		return fmt.Errorf("failed to resolve address: %w", err)
	}

	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	s.conn = conn
	s.logger.Infof("STUN server started on %s", s.addr)

	go s.serve()
	return nil
}

// Stop останавливает STUN сервер
func (s *Server) Stop() error {
	if s.conn != nil {
		return s.conn.Close()
	}
	return nil
}

// serve обрабатывает входящие запросы
func (s *Server) serve() {
	buf := make([]byte, 1024)
	
	for {
		n, addr, err := s.conn.ReadFromUDP(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Temporary() {
				continue
			}
			s.logger.Errorf("STUN read error: %v", err)
			return
		}

		go s.handlePacket(buf[:n], addr)
	}
}

// handlePacket обрабатывает STUN пакет
func (s *Server) handlePacket(data []byte, addr *net.UDPAddr) {
	msg := &stun.Message{
		Raw: append([]byte{}, data...),
	}

	if err := msg.Decode(); err != nil {
		s.logger.Debugf("Failed to decode STUN message: %v", err)
		return
	}

	// Проверяем, что это Binding Request
	if msg.Type != stun.BindingRequest {
		s.logger.Debugf("Unsupported STUN message type: %v", msg.Type)
		return
	}

	// Создаем Binding Success Response
	response := stun.MustBuild(
		stun.NewTransactionIDSetter(msg.TransactionID),
		stun.BindingSuccess,
		stun.XORMappedAddress{
			IP:   addr.IP,
			Port: addr.Port,
		},
		stun.MessageIntegrity([]byte("meshvpn")), // В продакшене использовать реальный ключ
		stun.Fingerprint,
	)

	// Отправляем ответ
	if _, err := s.conn.WriteToUDP(response.Raw, addr); err != nil {
		s.logger.Errorf("Failed to send STUN response: %v", err)
	}
}

// GetPublicAddress определяет публичный адрес через внешний STUN сервер
func GetPublicAddress(stunServer string) (string, error) {
	conn, err := net.Dial("udp", stunServer)
	if err != nil {
		return "", fmt.Errorf("failed to connect to STUN server: %w", err)
	}
	defer conn.Close()

	// Создаем Binding Request
	request := stun.MustBuild(stun.TransactionID, stun.BindingRequest)

	// Отправляем запрос
	if _, err := conn.Write(request.Raw); err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}

	// Читаем ответ
	response := make([]byte, 1024)
	n, err := conn.Read(response)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	// Парсим ответ
	msg := &stun.Message{Raw: response[:n]}
	if err := msg.Decode(); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	var addr stun.XORMappedAddress
	if err := addr.GetFrom(msg); err != nil {
		return "", fmt.Errorf("failed to get mapped address: %w", err)
	}

	return fmt.Sprintf("%s:%d", addr.IP.String(), addr.Port), nil
}
