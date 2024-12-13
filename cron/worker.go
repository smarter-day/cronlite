package cron

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"os"
)

type IWorkerIdProvider interface {
	Id() (string, error)
}

type DefaultWorkerIDProvider struct{}

func (d DefaultWorkerIDProvider) Id() (string, error) {
	return GetWorkerID()
}

// GetWorkerID generates a unique machine ID based on system characteristics.
func GetWorkerID() (string, error) {
	var data string

	// Get MAC addresses
	interfaces, err := net.Interfaces()
	if err != nil {
		return "", fmt.Errorf("failed to get network interfaces: %w", err)
	}
	for _, iface := range interfaces {
		if iface.HardwareAddr != nil && len(iface.HardwareAddr) > 0 {
			data += iface.HardwareAddr.String()
		}
	}

	// Get IP addresses
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", fmt.Errorf("failed to get IP addresses: %w", err)
	}
	for _, addr := range addrs {
		data += addr.String()
	}

	// Get hostname
	hostname, err := os.Hostname()
	if err != nil {
		return "", fmt.Errorf("failed to get hostname: %w", err)
	}
	data += hostname

	// Include the process ID to differentiate instances on the same machine
	pid := os.Getpid()
	data += fmt.Sprintf("%d", pid)

	// Hash the collected data
	hasher := sha256.New()
	_, err = hasher.Write([]byte(data))
	if err != nil {
		return "", fmt.Errorf("failed to hash data: %w", err)
	}

	machineID := hex.EncodeToString(hasher.Sum(nil))
	return machineID, nil
}
