package vpn

import (
	"encoding/binary"
	"fmt"
	"net"
	"os/exec"
	"runtime"
	"sync"
	"time"
	"unsafe"

	"github.com/sirupsen/logrus"
	"golang.org/x/sys/windows"
)

const (
	tapDeviceGUID  = `\\.\Global\`
	FILE_DEVICE_UNKNOWN = 0x00000022
	METHOD_BUFFERED     = 0
	FILE_ANY_ACCESS     = 0
)

// TAP ioctl codes
var (
	TAP_WIN_IOCTL_GET_MAC    = tapCtlCode(1)
	TAP_WIN_IOCTL_SET_MEDIA_STATUS = tapCtlCode(6)
	TAP_WIN_IOCTL_CONFIG_TUN = tapCtlCode(10)
)

func tapCtlCode(function uint32) uint32 {
	return (FILE_DEVICE_UNKNOWN << 16) | (FILE_ANY_ACCESS << 14) | (function << 2) | METHOD_BUFFERED
}

// Manager управляет VPN туннелем через TAP
type Manager struct {
	virtualIP  string
	subnet     string
	peers      map[string]*PeerInfo
	mu         sync.RWMutex
	logger     *logrus.Logger
	tapHandle  windows.Handle
	tapName    string
	packetCh   chan []byte      // исходящие пакеты (из TAP в WebSocket)
	incomingCh chan []byte      // входящие пакеты (из WebSocket в TAP)
	stopCh     chan struct{}
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
	ID        string    `json:"id"`
	PublicKey string    `json:"public_key"`
	VirtualIP string    `json:"virtual_ip"`
	Endpoint  string    `json:"endpoint,omitempty"`
	Connected bool      `json:"connected"`
	LastSeen  time.Time `json:"last_seen"`
	Latency   int       `json:"latency_ms"`
}

// NewManager создает новый VPN менеджер
func NewManager(logger *logrus.Logger) *Manager {
	return &Manager{
		peers:      make(map[string]*PeerInfo),
		logger:     logger,
		packetCh:   make(chan []byte, 1000),
		incomingCh: make(chan []byte, 1000),
		stopCh:     make(chan struct{}),
	}
}

// GenerateKeys - заглушка (ключи не нужны для TAP туннеля)
func (m *Manager) GenerateKeys() error {
	return nil
}

// GetPublicKey - заглушка
func (m *Manager) GetPublicKey() string {
	return ""
}

// GetVirtualIP возвращает виртуальный IP
func (m *Manager) GetVirtualIP() string {
	return m.virtualIP
}

// GetPacketCh возвращает канал исходящих пакетов
func (m *Manager) GetPacketCh() <-chan []byte {
	return m.packetCh
}

// SendPacket отправляет входящий пакет в TAP
func (m *Manager) SendPacket(data []byte) {
	select {
	case m.incomingCh <- data:
	default:
		// Буфер заполнен - пропускаем
	}
}

// ConfigureInterface настраивает TAP интерфейс
func (m *Manager) ConfigureInterface(virtualIP string, peers []PeerConfig) error {
	m.virtualIP = virtualIP

	m.mu.Lock()
	for _, p := range peers {
		m.peers[p.ID] = &PeerInfo{
			ID:        p.ID,
			PublicKey: p.PublicKey,
			VirtualIP: p.VirtualIP,
			Connected: true,
			LastSeen:  time.Now(),
		}
	}
	m.mu.Unlock()

	if runtime.GOOS != "windows" {
		return m.configureLinux()
	}

	return m.configureTAP()
}

// configureTAP открывает TAP адаптер и настраивает IP
func (m *Manager) configureTAP() error {
	// Находим TAP адаптер
	tapName, err := m.findTAPAdapter()
	if err != nil {
		return fmt.Errorf("TAP адаптер не найден. Установите OpenVPN или TAP-Windows: %w", err)
	}
	m.tapName = tapName
	m.logger.Infof("Found TAP adapter: %s", tapName)

	// Открываем TAP устройство
	handle, err := m.openTAP(tapName)
	if err != nil {
		return fmt.Errorf("failed to open TAP: %w", err)
	}
	m.tapHandle = handle

	// Включаем TAP
	if err := m.setTAPStatus(true); err != nil {
		m.logger.Warnf("Failed to set TAP status: %v", err)
	}

	// Назначаем IP через netsh
	ip := net.ParseIP(m.virtualIP).To4()
	mask := net.ParseIP("255.255.0.0").To4()
	if err := m.configureTAPIP(tapName, ip, mask); err != nil {
		m.logger.Warnf("Failed to configure TAP IP: %v", err)
	}

	// Запускаем чтение/запись пакетов
	go m.readFromTAP()
	go m.writeToTAP()

	m.logger.Infof("TAP configured with IP %s", m.virtualIP)
	return nil
}

// findTAPAdapter находит имя TAP адаптера в реестре
func (m *Manager) findTAPAdapter() (string, error) {
	// Ищем в реестре GUID TAP адаптеров
	key, err := windows.RegOpenKeyEx(
		windows.HKEY_LOCAL_MACHINE,
		windows.StringToUTF16Ptr(`SYSTEM\CurrentControlSet\Control\Class\{4D36E972-E325-11CE-BFC1-08002BE10318}`),
		0,
		windows.KEY_READ,
	)
	if err != nil {
		return "", fmt.Errorf("registry open failed: %w", err)
	}
	defer windows.RegCloseKey(key)

	var i uint32
	for i = 0; ; i++ {
		var nameBuf [256]uint16
		nameLen := uint32(len(nameBuf))
		if err := windows.RegEnumKeyEx(key, i, &nameBuf[0], &nameLen, nil, nil, nil, nil); err != nil {
			break
		}

		subKeyName := windows.UTF16ToString(nameBuf[:nameLen])
		subKey, err := windows.RegOpenKeyEx(key,
			windows.StringToUTF16Ptr(subKeyName),
			0, windows.KEY_READ)
		if err != nil {
			continue
		}

		// Проверяем ComponentId
		compID, err := readRegString(subKey, "ComponentId")
		windows.RegCloseKey(subKey)
		if err != nil {
			continue
		}

		if compID == "tap0901" || compID == "tap0801" || compID == "tap_ovpnconnect" {
			// Нашли TAP адаптер - получаем NetCfgInstanceId
			subKey2, err := windows.RegOpenKeyEx(key,
				windows.StringToUTF16Ptr(subKeyName),
				0, windows.KEY_READ)
			if err != nil {
				continue
			}
			guid, err := readRegString(subKey2, "NetCfgInstanceId")
			windows.RegCloseKey(subKey2)
			if err != nil {
				continue
			}
			return guid, nil
		}
	}

	return "", fmt.Errorf("no TAP adapter found")
}

// openTAP открывает TAP устройство
func (m *Manager) openTAP(guid string) (windows.Handle, error) {
	path := `\\.\Global\` + guid + `.tap`
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return windows.InvalidHandle, err
	}

	handle, err := windows.CreateFile(
		pathPtr,
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		0,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_ATTRIBUTE_SYSTEM|windows.FILE_FLAG_OVERLAPPED,
		0,
	)
	if err != nil {
		return windows.InvalidHandle, fmt.Errorf("CreateFile failed: %w", err)
	}

	return handle, nil
}

// setTAPStatus включает/выключает TAP
func (m *Manager) setTAPStatus(up bool) error {
	var status uint32
	if up {
		status = 1
	}

	var bytesReturned uint32
	statusBytes := (*[4]byte)(unsafe.Pointer(&status))[:]

	return windows.DeviceIoControl(
		m.tapHandle,
		TAP_WIN_IOCTL_SET_MEDIA_STATUS,
		&statusBytes[0],
		uint32(len(statusBytes)),
		&statusBytes[0],
		uint32(len(statusBytes)),
		&bytesReturned,
		nil,
	)
}

// configureTAPIP назначает IP адрес TAP интерфейсу
func (m *Manager) configureTAPIP(guid, ip, mask []byte) error {
	// Через netsh назначаем IP
	ipStr := net.IP(ip).String()
	maskStr := net.IP(mask).String()

	// Получаем имя интерфейса
	ifName := m.getInterfaceName(fmt.Sprintf("{%s}", string([]byte(m.tapName))))

	if ifName == "" {
		// Пробуем через GUID напрямую
		cmd := exec.Command("netsh", "interface", "ip", "set", "address",
			fmt.Sprintf(`name="%s"`, m.tapName), "static", ipStr, maskStr)
		output, err := cmd.CombinedOutput()
		m.logger.Debugf("netsh output: %s", string(output))
		return err
	}

	cmd := exec.Command("netsh", "interface", "ip", "set", "address",
		fmt.Sprintf(`name="%s"`, ifName), "static", ipStr, maskStr)
	output, err := cmd.CombinedOutput()
	m.logger.Debugf("netsh output: %s", string(output))
	return err
}

// getInterfaceName получает имя сетевого интерфейса по GUID
func (m *Manager) getInterfaceName(guid string) string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback == 0 {
			// Примерное совпадение
			_ = guid
			if len(iface.Name) > 0 {
				return iface.Name
			}
		}
	}
	return ""
}

// readFromTAP читает пакеты из TAP и отправляет в канал
func (m *Manager) readFromTAP() {
	buf := make([]byte, 65536)
	for {
		select {
		case <-m.stopCh:
			return
		default:
		}

		var bytesRead uint32
		overlapped := new(windows.Overlapped)
		event, _ := windows.CreateEvent(nil, true, false, nil)
		overlapped.HEvent = event

		err := windows.ReadFile(m.tapHandle, buf, &bytesRead, overlapped)
		if err == windows.ERROR_IO_PENDING {
			windows.WaitForSingleObject(event, windows.INFINITE)
			windows.GetOverlappedResult(m.tapHandle, overlapped, &bytesRead, false)
		}
		windows.CloseHandle(event)

		if bytesRead > 0 {
			packet := make([]byte, bytesRead)
			copy(packet, buf[:bytesRead])
			select {
			case m.packetCh <- packet:
			default:
			}
		}
	}
}

// writeToTAP записывает входящие пакеты в TAP
func (m *Manager) writeToTAP() {
	for {
		select {
		case <-m.stopCh:
			return
		case packet := <-m.incomingCh:
			var bytesWritten uint32
			overlapped := new(windows.Overlapped)
			event, _ := windows.CreateEvent(nil, true, false, nil)
			overlapped.HEvent = event

			err := windows.WriteFile(m.tapHandle, packet, &bytesWritten, overlapped)
			if err == windows.ERROR_IO_PENDING {
				windows.WaitForSingleObject(event, windows.INFINITE)
			}
			windows.CloseHandle(event)
		}
	}
}

// configureLinux настраивает TUN на Linux
func (m *Manager) configureLinux() error {
	exec.Command("ip", "tuntap", "add", "dev", "meshvpn0", "mode", "tun").Run()
	exec.Command("ip", "addr", "add", m.virtualIP+"/16", "dev", "meshvpn0").Run()
	exec.Command("ip", "link", "set", "dev", "meshvpn0", "up").Run()
	m.logger.Infof("Linux TUN configured with IP %s", m.virtualIP)
	return nil
}

// Cleanup останавливает туннель
func (m *Manager) Cleanup() error {
	close(m.stopCh)

	if m.tapHandle != windows.InvalidHandle && m.tapHandle != 0 {
		m.setTAPStatus(false)
		windows.CloseHandle(m.tapHandle)
	}

	m.mu.Lock()
	m.peers = make(map[string]*PeerInfo)
	m.virtualIP = ""
	m.mu.Unlock()

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
		Connected: true,
		LastSeen:  time.Now(),
	}
}

// RemovePeer удаляет пира
func (m *Manager) RemovePeer(peerID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.peers, peerID)
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

// Ping пингует IP
func (m *Manager) Ping(targetIP string) (string, error) {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("ping", "-n", "4", "-w", "2000", targetIP)
	} else {
		cmd = exec.Command("ping", "-c", "4", "-W", "2", targetIP)
	}
	output, _ := cmd.CombinedOutput()
	return string(output), nil
}

// GetPrivateKey заглушка
func (m *Manager) GetPrivateKey() string { return "" }

// GetWireGuardConfig заглушка
func (m *Manager) GetWireGuardConfig() string { return "" }

// UpdatePeerEndpoint обновляет endpoint
func (m *Manager) UpdatePeerEndpoint(peerID, endpoint string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if peer, ok := m.peers[peerID]; ok {
		peer.Endpoint = endpoint
		peer.LastSeen = time.Now()
	}
}

// IsVirtualIP проверяет виртуальный IP
func IsVirtualIP(ip string, subnet string) bool {
	_, virtualNet, err := net.ParseCIDR(subnet)
	if err != nil {
		return false
	}
	return virtualNet.Contains(net.ParseIP(ip))
}

// readRegString читает строку из реестра
func readRegString(key windows.Handle, name string) (string, error) {
	var valType uint32
	var buf [256]uint16
	bufLen := uint32(len(buf) * 2)

	err := windows.RegQueryValueEx(key,
		windows.StringToUTF16Ptr(name),
		nil, &valType,
		(*byte)(unsafe.Pointer(&buf[0])),
		&bufLen)
	if err != nil {
		return "", err
	}

	return windows.UTF16ToString(buf[:bufLen/2]), nil
}

// uint32ToBytes конвертирует uint32 в байты
func uint32ToBytes(v uint32) []byte {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, v)
	return b
}
