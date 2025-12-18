package services

import (
	"fmt"
	"path/filepath"
	"sync"

	"modem-manager/models"
)

var (
	managerOnce     sync.Once
	managerInstance *SerialManager
)

// SerialManager manages multiple serial connections.
type SerialManager struct {
	pool map[string]*SerialService
	mu   sync.Mutex
}

// GetSerialManager returns the singleton instance of SerialManager.
func GetSerialManager() *SerialManager {
	managerOnce.Do(func() {
		managerInstance = &SerialManager{
			pool: make(map[string]*SerialService),
		}
	})
	return managerInstance
}

// Scan scans for available modems and connects to them.
// It looks for devices matching /dev/ttyUSB* and /dev/ttyACM*.
func (m *SerialManager) Scan(baudRate int) ([]models.SerialPort, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Find potential devices
	usb, _ := filepath.Glob("/dev/ttyUSB*")
	acm, _ := filepath.Glob("/dev/ttyACM*")
	candidates := append(usb, acm...)
	
	// Try to connect to new devices
	for _, p := range candidates {
		if _, exists := m.pool[p]; !exists {
			if svc, err := NewSerialService(p, baudRate); err == nil {
				m.pool[p] = svc
				svc.Start()
			}
		}
	}

	// Build result list from active connections
	var result []models.SerialPort
	for name := range m.pool {
		result = append(result, models.SerialPort{
			Name:      name,
			Path:      name,
			Connected: true,
		})
	}
	return result, nil
}

// GetService returns the SerialService for a given port name.
func (m *SerialManager) GetService(name string) (*SerialService, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	service, ok := m.pool[name]
	if !ok {
		return nil, fmt.Errorf("port not connected: %s", name)
	}
	return service, nil
}
