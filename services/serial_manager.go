package services

import (
	"fmt"
	"path/filepath"
	"runtime"
	"sync"

	"web-modem/modem"
)

var (
	managerOnce     sync.Once
	managerInstance *SerialManager
)

// SerialManager 管理多个串口连接。
type SerialManager struct {
	pool map[string]*modem.SerialService
	mu   sync.Mutex
}

// GetSerialManager 返回 SerialManager 的单例实例。
func GetSerialManager() *SerialManager {
	managerOnce.Do(func() {
		managerInstance = &SerialManager{
			pool: make(map[string]*modem.SerialService),
		}
	})
	return managerInstance
}

// candidatePorts 返回当前平台上可能的串口设备路径。
// Linux/macOS: /dev/ttyUSB*、/dev/ttyACM*、/dev/tty.usb* 等
// Windows: COM1–COM32（实际可用性在连接时验证）
func candidatePorts() []string {
	switch runtime.GOOS {
	case "windows":
		ports := make([]string, 0, 32)
		for i := 1; i <= 32; i++ {
			ports = append(ports, fmt.Sprintf("COM%d", i))
		}
		return ports
	default:
		patterns := []string{
			"/dev/ttyUSB*",
			"/dev/ttyACM*",
			"/dev/tty.usbserial*",
			"/dev/tty.usbmodem*",
			"/dev/cu.usbserial*",
			"/dev/cu.usbmodem*",
		}
		var ports []string
		for _, p := range patterns {
			if matched, err := filepath.Glob(p); err == nil {
				ports = append(ports, matched...)
			}
		}
		return ports
	}
}

// Scan 扫描可用的调制解调器并连接到它们。
func (m *SerialManager) Scan(baudRate int) ([]modem.SerialPort, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, p := range candidatePorts() {
		if _, exists := m.pool[p]; exists {
			continue
		}
		broadcast := GetEventListener().Broadcast
		if svc, err := modem.NewSerialService(p, baudRate, broadcast); err == nil {
			m.pool[p] = svc
			svc.Start()
		}
	}

	// 从活动连接构建结果列表
	var result []modem.SerialPort
	for name := range m.pool {
		result = append(result, modem.SerialPort{
			Name:      name,
			Path:      name,
			Connected: true,
		})
	}
	return result, nil
}

// GetService 返回给定端口名称的 SerialService。
func (m *SerialManager) GetService(name string) (*modem.SerialService, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	service, ok := m.pool[name]
	if !ok {
		return nil, fmt.Errorf("port not connected: %s", name)
	}
	return service, nil
}
