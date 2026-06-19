package models

import (
	"time"
)

// Network представляет виртуальную сеть
type Network struct {
	ID          string    `json:"id" db:"id"`
	Name        string    `json:"name" db:"name"`
	PasswordHash string   `json:"-" db:"password_hash"`
	OwnerID     string    `json:"owner_id" db:"owner_id"`
	Subnet      string    `json:"subnet" db:"subnet"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

// Peer представляет устройство в сети
type Peer struct {
	ID           string    `json:"id" db:"id"`
	NetworkID    string    `json:"network_id" db:"network_id"`
	PublicKey    string    `json:"public_key" db:"public_key"`
	PrivateKey   string    `json:"-" db:"private_key"` // Хранится только если сгенерирован сервером
	VirtualIP    string    `json:"virtual_ip" db:"virtual_ip"`
	Endpoint     string    `json:"endpoint,omitempty" db:"endpoint"` // Публичный адрес (обновляется)
	ListenPort   int       `json:"listen_port" db:"listen_port"`
	Hostname     string    `json:"hostname" db:"hostname"`
	OS           string    `json:"os" db:"os"`
	Version      string    `json:"version" db:"version"`
	LastSeen     time.Time `json:"last_seen" db:"last_seen"`
	Connected    bool      `json:"connected" db:"connected"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
}

// PeerICE представляет ICE кандидата для NAT traversal
type PeerICE struct {
	ID        string    `json:"id"`
	PeerID    string    `json:"peer_id"`
	NetworkID string    `json:"network_id"`
	TargetPeerID string `json:"target_peer_id"` // Кому предназначается
	Type      string    `json:"type"` // offer, answer, candidate
	Payload   string    `json:"payload"` // JSON с SDP или ICE candidate
	CreatedAt time.Time `json:"created_at"`
}

// Message представляет сообщение сигналинга
type Message struct {
	Type      string      `json:"type"`
	From      string      `json:"from"`
	To        string      `json:"to,omitempty"`
	NetworkID string      `json:"network_id"`
	Payload   interface{} `json:"payload"`
	Timestamp int64       `json:"timestamp"`
}

// SignalMessage типы сообщений
type SignalType string

const (
	SignalJoinNetwork   SignalType = "join_network"
	SignalLeaveNetwork  SignalType = "leave_network"
	SignalPeerList      SignalType = "peer_list"
	SignalOffer         SignalType = "offer"
	SignalAnswer        SignalType = "answer"
	SignalICE           SignalType = "ice_candidate"
	SignalHeartbeat     SignalType = "heartbeat"
	SignalError         SignalType = "error"
)

// CreateNetworkRequest запрос на создание сети
type CreateNetworkRequest struct {
	Name     string `json:"name" binding:"required,min=3,max=50"`
	Password string `json:"password" binding:"required,min=6,max=32"`
}

// JoinNetworkRequest запрос на присоединение к сети
type JoinNetworkRequest struct {
	NetworkID  string `json:"network_id" binding:"required"`
	Password   string `json:"password" binding:"required"`
	PublicKey  string `json:"public_key" binding:"required"`
	Hostname   string `json:"hostname" binding:"required,max=64"`
	OS         string `json:"os" binding:"required"`
	Version    string `json:"version" binding:"required"`
	ListenPort int    `json:"listen_port"`
}

// NetworkResponse ответ с информацией о сети
type NetworkResponse struct {
	Network
	Peers []Peer `json:"peers"`
}

// ICECandidate представляет ICE кандидата
type ICECandidate struct {
	Candidate        string `json:"candidate"`
	SDPMid           string `json:"sdpMid"`
	SDPMLineIndex    int    `json:"sdpMLineIndex"`
	UsernameFragment string `json:"usernameFragment,omitempty"`
}
