package db

import (
	"database/sql"
	"fmt"
	"meshvpn/server/internal/models"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// Database обертка для работы с БД
type Database struct {
	db *sql.DB
}

// NewDatabase создает новое подключение к БД
func NewDatabase(dbPath string) (*Database, error) {
	// Создаем директорию если не существует
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Настройки соединения
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	database := &Database{db: db}

	// Применяем миграции
	if err := database.migrate(); err != nil {
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}

	return database, nil
}

// Close закрывает соединение с БД
func (d *Database) Close() error {
	return d.db.Close()
}

// migrate применяет миграции
func (d *Database) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS networks (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		password_hash TEXT NOT NULL,
		owner_id TEXT,
		subnet TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_networks_owner ON networks(owner_id);

	CREATE TABLE IF NOT EXISTS peers (
		id TEXT PRIMARY KEY,
		network_id TEXT NOT NULL,
		public_key TEXT UNIQUE NOT NULL,
		private_key TEXT,
		virtual_ip TEXT UNIQUE NOT NULL,
		endpoint TEXT,
		listen_port INTEGER DEFAULT 0,
		hostname TEXT NOT NULL,
		os TEXT NOT NULL,
		version TEXT NOT NULL,
		last_seen DATETIME,
		connected INTEGER DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (network_id) REFERENCES networks(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_peers_network ON peers(network_id);
	CREATE INDEX IF NOT EXISTS idx_peers_pubkey ON peers(public_key);
	CREATE INDEX IF NOT EXISTS idx_peers_connected ON peers(connected);

	CREATE TABLE IF NOT EXISTS ice_messages (
		id TEXT PRIMARY KEY,
		peer_id TEXT NOT NULL,
		network_id TEXT NOT NULL,
		target_peer_id TEXT NOT NULL,
		type TEXT NOT NULL,
		payload TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (peer_id) REFERENCES peers(id) ON DELETE CASCADE,
		FOREIGN KEY (network_id) REFERENCES networks(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_ice_target ON ice_messages(target_peer_id, created_at);

	-- Очистка старых ICE сообщений
	CREATE TRIGGER IF NOT EXISTS cleanup_old_ice 
	AFTER INSERT ON ice_messages
	BEGIN
		DELETE FROM ice_messages 
		WHERE created_at < datetime('now', '-5 minutes');
	END;
	`

	_, err := d.db.Exec(schema)
	return err
}

// CreateNetwork создает новую сеть
func (d *Database) CreateNetwork(network *models.Network) error {
	query := `
		INSERT INTO networks (id, name, password_hash, owner_id, subnet, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`
	_, err := d.db.Exec(query, network.ID, network.Name, network.PasswordHash, 
		network.OwnerID, network.Subnet, network.CreatedAt, network.UpdatedAt)
	return err
}

// GetNetwork получает сеть по ID
func (d *Database) GetNetwork(id string) (*models.Network, error) {
	query := `SELECT * FROM networks WHERE id = ?`
	row := d.db.QueryRow(query, id)
	
	var n models.Network
	err := row.Scan(&n.ID, &n.Name, &n.PasswordHash, &n.OwnerID, &n.Subnet, &n.CreatedAt, &n.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &n, nil
}

// GetNetworkByPassword получает сеть по ID и проверяет пароль
func (d *Database) GetNetworkByPassword(id string, passwordHash string) (*models.Network, error) {
	query := `SELECT * FROM networks WHERE id = ? AND password_hash = ?`
	row := d.db.QueryRow(query, id, passwordHash)
	
	var n models.Network
	err := row.Scan(&n.ID, &n.Name, &n.PasswordHash, &n.OwnerID, &n.Subnet, &n.CreatedAt, &n.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &n, nil
}

// DeleteNetwork удаляет сеть
func (d *Database) DeleteNetwork(id string) error {
	query := `DELETE FROM networks WHERE id = ?`
	_, err := d.db.Exec(query, id)
	return err
}

// CreatePeer создает нового пира
func (d *Database) CreatePeer(peer *models.Peer) error {
	query := `
		INSERT INTO peers (id, network_id, public_key, private_key, virtual_ip, endpoint, 
		listen_port, hostname, os, version, last_seen, connected, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := d.db.Exec(query, peer.ID, peer.NetworkID, peer.PublicKey, peer.PrivateKey,
		peer.VirtualIP, peer.Endpoint, peer.ListenPort, peer.Hostname, peer.OS, 
		peer.Version, peer.LastSeen, peer.Connected, peer.CreatedAt)
	return err
}

// GetPeer получает пира по ID
func (d *Database) GetPeer(id string) (*models.Peer, error) {
	query := `SELECT * FROM peers WHERE id = ?`
	row := d.db.QueryRow(query, id)
	
	var p models.Peer
	err := row.Scan(&p.ID, &p.NetworkID, &p.PublicKey, &p.PrivateKey, &p.VirtualIP,
		&p.Endpoint, &p.ListenPort, &p.Hostname, &p.OS, &p.Version, 
		&p.LastSeen, &p.Connected, &p.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// GetPeerByPublicKey получает пира по публичному ключу
func (d *Database) GetPeerByPublicKey(pubKey string) (*models.Peer, error) {
	query := `SELECT * FROM peers WHERE public_key = ?`
	row := d.db.QueryRow(query, pubKey)
	
	var p models.Peer
	err := row.Scan(&p.ID, &p.NetworkID, &p.PublicKey, &p.PrivateKey, &p.VirtualIP,
		&p.Endpoint, &p.ListenPort, &p.Hostname, &p.OS, &p.Version, 
		&p.LastSeen, &p.Connected, &p.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// GetPeersByNetwork получает всех пиров в сети
func (d *Database) GetPeersByNetwork(networkID string) ([]models.Peer, error) {
	query := `SELECT * FROM peers WHERE network_id = ? AND connected = 1`
	rows, err := d.db.Query(query, networkID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var peers []models.Peer
	for rows.Next() {
		var p models.Peer
		err := rows.Scan(&p.ID, &p.NetworkID, &p.PublicKey, &p.PrivateKey, &p.VirtualIP,
			&p.Endpoint, &p.ListenPort, &p.Hostname, &p.OS, &p.Version, 
			&p.LastSeen, &p.Connected, &p.CreatedAt)
		if err != nil {
			continue
		}
		peers = append(peers, p)
	}
	return peers, nil
}

// UpdatePeerStatus обновляет статус подключения пира
func (d *Database) UpdatePeerStatus(id string, connected bool, endpoint string) error {
	query := `
		UPDATE peers 
		SET connected = ?, endpoint = ?, last_seen = ? 
		WHERE id = ?
	`
	_, err := d.db.Exec(query, connected, endpoint, time.Now(), id)
	return err
}

// DeletePeer удаляет пира
func (d *Database) DeletePeer(id string) error {
	query := `DELETE FROM peers WHERE id = ?`
	_, err := d.db.Exec(query, id)
	return err
}

// SaveICEMessage сохраняет ICE сообщение
func (d *Database) SaveICEMessage(msg *models.PeerICE) error {
	query := `
		INSERT INTO ice_messages (id, peer_id, network_id, target_peer_id, type, payload, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`
	_, err := d.db.Exec(query, msg.ID, msg.PeerID, msg.NetworkID, msg.TargetPeerID, 
		msg.Type, msg.Payload, msg.CreatedAt)
	return err
}

// GetICEMessages получает ICE сообщения для пира
func (d *Database) GetICEMessages(targetPeerID string, since time.Time) ([]models.PeerICE, error) {
	query := `
		SELECT * FROM ice_messages 
		WHERE target_peer_id = ? AND created_at > ?
		ORDER BY created_at ASC
	`
	rows, err := d.db.Query(query, targetPeerID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []models.PeerICE
	for rows.Next() {
		var m models.PeerICE
		err := rows.Scan(&m.ID, &m.PeerID, &m.NetworkID, &m.TargetPeerID, &m.Type, &m.Payload, &m.CreatedAt)
		if err != nil {
			continue
		}
		messages = append(messages, m)
	}
	return messages, nil
}

// CleanupDisconnectedPeers удаляет неактивных пиров
func (d *Database) CleanupDisconnectedPeers(maxAge time.Duration) error {
	query := `
		DELETE FROM peers 
		WHERE connected = 0 AND last_seen < ?
	`
	cutoff := time.Now().Add(-maxAge)
	_, err := d.db.Exec(query, cutoff)
	return err
}
