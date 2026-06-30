package daemon

import (
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"

	"wg-server/db"
	"wg-server/wg"
)

// Monitor handles periodic bandwidth data collection.
type Monitor struct {
	interfaceName string
	dataDir       string
	stopCh        chan struct{}
	wg            sync.WaitGroup
	running       bool
	lastRx        int64
	lastTx        int64
	lastTime      time.Time
	pidFile       string
}

// NewMonitor creates a new bandwidth monitor.
func NewMonitor(interfaceName, dataDir string) *Monitor {
	return &Monitor{
		interfaceName: interfaceName,
		dataDir:       dataDir,
		stopCh:        make(chan struct{}),
		pidFile:       filepath.Join(dataDir, "monitor.pid"),
	}
}

// Start begins the bandwidth monitoring loop.
func (m *Monitor) Start() {
	m.wg.Add(1)
	m.running = true

	// Write PID file
	pid := os.Getpid()
	os.WriteFile(m.pidFile, []byte(fmt.Sprintf("%d", pid)), 0644)

	go func() {
		defer m.wg.Done()
		m.collectLoop()
	}()

	log.Println("Bandwidth monitor started (PID:", pid, ")")
}

// Stop stops the monitoring loop.
func (m *Monitor) Stop() {
	if m.running {
		close(m.stopCh)
		m.wg.Wait()
		m.running = false
		os.Remove(m.pidFile)
		log.Println("Bandwidth monitor stopped")
	}
}

// IsRunning checks if the monitor is currently running.
func (m *Monitor) IsRunning() bool {
	return m.running
}

func (m *Monitor) collectLoop() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	// Initialize with current values
	m.lastRx, m.lastTx, _ = wg.GetInterfaceTransfer(m.interfaceName)
	m.lastTime = time.Now()

	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.collect()
		}
	}
}

func (m *Monitor) collect() {
	// Collect global bandwidth
	currentRx, currentTx, err := wg.GetInterfaceTransfer(m.interfaceName)
	if err != nil {
		return
	}

	now := time.Now()
	elapsed := now.Sub(m.lastTime).Seconds()
	if elapsed <= 0 {
		elapsed = 1
	}

	rxSpeed := math.Max(0, float64(currentRx-m.lastRx)/elapsed)
	txSpeed := math.Max(0, float64(currentTx-m.lastTx)/elapsed)

	// Save global bandwidth point
	if err := db.SaveGlobalBandwidthPoint(currentRx, currentTx, rxSpeed, txSpeed); err != nil {
		log.Printf("Failed to save global bandwidth: %v", err)
	}

	m.lastRx = currentRx
	m.lastTx = currentTx
	m.lastTime = now

	// Collect per-user bandwidth
	m.collectPerUserBandwidth()
}

func (m *Monitor) collectPerUserBandwidth() {
	peers, err := wg.GetPeers(m.interfaceName)
	if err != nil {
		return
	}

	users, err := db.ListUsers()
	if err != nil {
		return
	}

	// Build a map of public key -> user ID
	pubKeyToUser := make(map[string]int)
	for _, u := range users {
		pubKeyToUser[u.PublicKey] = u.ID
	}

	for _, peer := range peers {
		userID, exists := pubKeyToUser[peer.PublicKey]
		if !exists {
			continue
		}

		// Save per-user bandwidth point
		if err := db.SaveBandwidthPoint(userID, peer.TransferRx, peer.TransferTx, 0, 0); err != nil {
			log.Printf("Failed to save user bandwidth (user %d): %v", userID, err)
		}

		// Check if user just connected (recent handshake)
		// We handle connection tracking separately via API
		_ = peer.LatestHandshake
	}
}
