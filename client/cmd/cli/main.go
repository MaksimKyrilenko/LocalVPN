package main

import (
	"bufio"
	"fmt"
	"meshvpn/client/internal/config"
	"meshvpn/client/internal/network"
	"meshvpn/client/internal/vpn"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

func main() {
	logger := logrus.New()
	logger.SetLevel(logrus.InfoLevel)

	fmt.Println("╔════════════════════════════════════════╗")
	fmt.Println("║         MeshVPN CLI Client             ║")
	fmt.Println("║            Version 1.0.0               ║")
	fmt.Println("╚════════════════════════════════════════╝")
	fmt.Println()

	// Загружаем конфигурацию
	cfg, err := config.Load()
	if err != nil {
		logger.Warnf("Failed to load config: %v", err)
		cfg = config.DefaultConfig()
	}

	peerID := uuid.New().String()[:8]
	logger.Infof("Peer ID: %s", peerID)

	// Создаем менеджеры
	networkMgr := network.NewManager(cfg.ServerURL, peerID, logger)
	vpnMgr := vpn.NewManager(logger)

	// Обработка завершения
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	
	go func() {
		<-sigChan
		fmt.Println("\nDisconnecting...")
		networkMgr.Disconnect()
		vpnMgr.Cleanup()
		os.Exit(0)
	}()

	// Главный цикл
	reader := bufio.NewReader(os.Stdin)
	
	for {
		fmt.Println()
		fmt.Println("Commands:")
		fmt.Println("  1. Connect to network")
		fmt.Println("  2. Create network")
		fmt.Println("  3. Disconnect")
		fmt.Println("  4. Show status")
		fmt.Println("  5. List peers")
		fmt.Println("  6. Ping peer")
		fmt.Println("  7. Test server")
		fmt.Println("  8. Settings")
		fmt.Println("  9. Exit")
		fmt.Println()
		fmt.Print("Select: ")

		choice, _ := reader.ReadString('\n')
		choice = strings.TrimSpace(choice)

		switch choice {
		case "1":
			connectToNetwork(reader, networkMgr, vpnMgr, logger)
		case "2":
			createNetwork(reader, networkMgr, vpnMgr, logger)
		case "3":
			networkMgr.Disconnect()
			vpnMgr.Cleanup()
			fmt.Println("Disconnected")
		case "4":
			showStatus(networkMgr, vpnMgr)
		case "5":
			listPeers(vpnMgr)
		case "6":
			sendPing(reader, vpnMgr)
		case "7":
			testServer(reader, networkMgr)
		case "8":
			changeSettings(reader, cfg)
		case "9":
			fmt.Println("Goodbye!")
			networkMgr.Disconnect()
			vpnMgr.Cleanup()
			return
		default:
			fmt.Println("Invalid choice")
		}
	}
}

func connectToNetwork(reader *bufio.Reader, networkMgr *network.Manager, vpnMgr *vpn.Manager, logger *logrus.Logger) {
	fmt.Print("Server URL [" + networkMgr.GetServerURL() + "]: ")
	serverURL, _ := reader.ReadString('\n')
	serverURL = strings.TrimSpace(serverURL)
	if serverURL != "" {
		networkMgr.SetServerURL(serverURL)
	}

	fmt.Print("Network ID: ")
	networkID, _ := reader.ReadString('\n')
	networkID = strings.TrimSpace(networkID)
	if networkID == "" {
		fmt.Println("Network ID is required")
		return
	}

	fmt.Print("Password: ")
	password, _ := reader.ReadString('\n')
	password = strings.TrimSpace(password)

	fmt.Println("Connecting...")

	// Генерируем ключи
	if err := vpnMgr.GenerateKeys(); err != nil {
		logger.Errorf("Failed to generate keys: %v", err)
		return
	}

	publicKey := vpnMgr.GetPublicKey()
	
	resp, err := networkMgr.JoinNetwork(networkID, password, publicKey, "linux")
	if err != nil {
		logger.Errorf("Failed to join: %v", err)
		return
	}

	virtualIP, ok := resp["virtual_ip"].(string)
	if !ok {
		logger.Error("Invalid response from server")
		return
	}

	// Настраиваем VPN
	var peers []vpn.PeerConfig
	if peersData, ok := resp["peers"].([]interface{}); ok {
		for _, p := range peersData {
			if peerMap, ok := p.(map[string]interface{}); ok {
				peers = append(peers, vpn.PeerConfig{
					ID:        getString(peerMap, "id"),
					PublicKey: getString(peerMap, "public_key"),
					VirtualIP: getString(peerMap, "virtual_ip"),
					Endpoint:  getString(peerMap, "endpoint"),
				})
			}
		}
	}

	if err := vpnMgr.ConfigureInterface(virtualIP, peers); err != nil {
		logger.Errorf("Failed to configure VPN: %v", err)
		return
	}

	fmt.Printf("Connected! Virtual IP: %s\n", virtualIP)
	fmt.Printf("Peers in network: %d\n", len(peers))
}

func createNetwork(reader *bufio.Reader, networkMgr *network.Manager, vpnMgr *vpn.Manager, logger *logrus.Logger) {
	fmt.Print("Server URL [" + networkMgr.GetServerURL() + "]: ")
	serverURL, _ := reader.ReadString('\n')
	serverURL = strings.TrimSpace(serverURL)
	if serverURL != "" {
		networkMgr.SetServerURL(serverURL)
	}

	fmt.Print("Network name: ")
	name, _ := reader.ReadString('\n')
	name = strings.TrimSpace(name)
	if name == "" {
		name = "My Network"
	}

	fmt.Print("Password (min 6 chars): ")
	password, _ := reader.ReadString('\n')
	password = strings.TrimSpace(password)
	if len(password) < 6 {
		fmt.Println("Password too short")
		return
	}

	fmt.Println("Creating network...")

	resp, err := networkMgr.CreateNetwork(name, password)
	if err != nil {
		logger.Errorf("Failed to create: %v", err)
		return
	}

	networkID, ok := resp["network_id"].(string)
	if !ok {
		logger.Error("Invalid response from server")
		return
	}

	fmt.Printf("Network created!\n")
	fmt.Printf("Network ID: %s\n", networkID)
	fmt.Printf("Password: %s\n", password)
	fmt.Println()
	fmt.Println("Share this with others to let them join.")
	fmt.Println()

	// Авто-подключение
	fmt.Print("Connect now? (y/n): ")
	answer, _ := reader.ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))
	if answer == "y" || answer == "yes" {
		// Генерируем ключи
		if err := vpnMgr.GenerateKeys(); err != nil {
			logger.Errorf("Failed to generate keys: %v", err)
			return
		}

		publicKey := vpnMgr.GetPublicKey()
		
		joinResp, err := networkMgr.JoinNetwork(networkID, password, publicKey, "linux")
		if err != nil {
			logger.Errorf("Failed to join: %v", err)
			return
		}

		virtualIP, _ := joinResp["virtual_ip"].(string)
		
		var peers []vpn.PeerConfig
		if peersData, ok := joinResp["peers"].([]interface{}); ok {
			for _, p := range peersData {
				if peerMap, ok := p.(map[string]interface{}); ok {
					peers = append(peers, vpn.PeerConfig{
						ID:        getString(peerMap, "id"),
						PublicKey: getString(peerMap, "public_key"),
						VirtualIP: getString(peerMap, "virtual_ip"),
					})
				}
			}
		}

		if err := vpnMgr.ConfigureInterface(virtualIP, peers); err != nil {
			logger.Errorf("Failed to configure VPN: %v", err)
			return
		}

		fmt.Printf("Connected! Virtual IP: %s\n", virtualIP)
	}
}

func showStatus(networkMgr *network.Manager, vpnMgr *vpn.Manager) {
	fmt.Println()
	fmt.Println("Status:")
	fmt.Printf("  Connected: %v\n", networkMgr.IsConnected())
	fmt.Printf("  Network ID: %s\n", networkMgr.GetNetworkID())
	fmt.Printf("  Virtual IP: %s\n", vpnMgr.GetVirtualIP())
	fmt.Printf("  Public Key: %s...\n", vpnMgr.GetPublicKey()[:20])
	fmt.Printf("  Peers: %d\n", len(vpnMgr.GetPeers()))
	fmt.Println()
}

func listPeers(vpnMgr *vpn.Manager) {
	peers := vpnMgr.GetPeers()
	if len(peers) == 0 {
		fmt.Println("No peers connected")
		return
	}

	fmt.Println()
	fmt.Println("Peers:")
	fmt.Println("  Virtual IP    | Status   | Last Seen")
	fmt.Println("  " + strings.Repeat("-", 50))
	
	for _, peer := range peers {
		status := "Offline"
		if peer.Connected {
			status = "Online"
		}
		fmt.Printf("  %-14s | %-8s | %s\n", 
			peer.VirtualIP, status, 
			peer.LastSeen.Format("15:04:05"))
	}
	fmt.Println()
}

func sendPing(reader *bufio.Reader, vpnMgr *vpn.Manager) {
	fmt.Print("Target IP: ")
	targetIP, _ := reader.ReadString('\n')
	targetIP = strings.TrimSpace(targetIP)

	fmt.Printf("Pinging %s...\n", targetIP)
	
	result, err := vpnMgr.Ping(targetIP)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Println(result)
}

func testServer(reader *bufio.Reader, networkMgr *network.Manager) {
	fmt.Print("Server URL [" + networkMgr.GetServerURL() + "]: ")
	serverURL, _ := reader.ReadString('\n')
	serverURL = strings.TrimSpace(serverURL)
	if serverURL == "" {
		serverURL = networkMgr.GetServerURL()
	}

	fmt.Println("Testing connection...")
	
	tempMgr := network.NewManager(serverURL, "test", networkMgr.GetLogger())
	info, err := tempMgr.GetServerInfo()
	if err != nil {
		fmt.Printf("Failed: %v\n", err)
		return
	}

	fmt.Println("Server is online!")
	fmt.Printf("  STUN: %v\n", info["stun"])
	fmt.Printf("  TURN: %v\n", info["turn"])
	fmt.Printf("  Version: %v\n", info["version"])
}

func changeSettings(reader *bufio.Reader, cfg *config.Config) {
	fmt.Printf("Current server URL: %s\n", cfg.ServerURL)
	fmt.Print("New server URL (empty to keep): ")
	newURL, _ := reader.ReadString('\n')
	newURL = strings.TrimSpace(newURL)
	if newURL != "" {
		cfg.ServerURL = newURL
	}

	fmt.Printf("Auto-start: %v\n", cfg.AutoStart)
	fmt.Print("Enable auto-start? (y/n): ")
	answer, _ := reader.ReadString('\n')
	answer = strings.ToLower(strings.TrimSpace(answer))
	cfg.AutoStart = answer == "y" || answer == "yes"

	if err := cfg.Save(); err != nil {
		fmt.Printf("Failed to save: %v\n", err)
		return
	}

	fmt.Println("Settings saved!")
}

func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func (m *network.Manager) GetServerURL() string {
	return m.GetServerURL()
}

func (m *network.Manager) GetLogger() *logrus.Logger {
	return nil
}
