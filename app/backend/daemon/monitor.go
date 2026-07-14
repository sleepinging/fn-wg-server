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

// Monitor handles periodic bandwidth data collection and privileged WireGuard operations.
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
	// 每个用户上次流量值（实时速度计算用），daemon 重启时重置
	lastUserRx   map[int]int64
	lastUserTx   map[int]int64
	lastUserTime time.Time
	// 连接追踪：记录上次握手时间，检测上线/离线
	lastHandshake map[int]int64
}

// NewMonitor creates a new bandwidth monitor.
func NewMonitor(interfaceName, dataDir string) *Monitor {
	return &Monitor{
		interfaceName: interfaceName,
		dataDir:       dataDir,
		stopCh:        make(chan struct{}),
		pidFile:       filepath.Join(dataDir, "monitor.pid"),
		lastUserRx:    make(map[int]int64),
		lastUserTx:    make(map[int]int64),
		lastUserTime:  time.Now(),
		lastHandshake: make(map[int]int64),
	}
}

// Start begins the bandwidth monitoring loop.
func (m *Monitor) Start() {
	m.wg.Add(1)
	m.running = true

	flushInterval := db.GetConfigFlushInterval()
	db.StartWriteBuffer(flushInterval)

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
		done := make(chan struct{}, 1)
		go func() {
			m.wg.Wait()
			done <- struct{}{}
		}()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			log.Println("Monitor goroutine stop timed out")
		}
		m.running = false
		db.StopWriteBuffer()
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

	m.lastRx, m.lastTx, _ = wg.GetInterfaceTransfer(m.interfaceName)
	m.lastTime = time.Now()

	m.lastUserRx = make(map[int]int64)
	m.lastUserTx = make(map[int]int64)
	m.lastUserTime = time.Now()
	m.lastHandshake = make(map[int]int64)

	m.syncConfig()

	// 收尾上次未正常断开的连接记录（进程被杀/重启后遗留的僵尸会话）
	if n, err := db.CloseAllStaleSessions(); err != nil {
		log.Printf("CloseAllStaleSessions error: %v", err)
	} else if n > 0 {
		log.Printf("Closed %d stale connection session(s) on startup", n)
	}

	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.collect()
		}
	}
}

func (m *Monitor) syncConfig() {
	cfg, err := wg.LoadConfig()
	if err != nil {
		log.Printf("syncConfig: load config error: %v", err)
		return
	}
	users, err := db.ListUsers()
	if err != nil {
		log.Printf("syncConfig: list users error: %v", err)
		return
	}
	if err := wg.ApplyConfig(*cfg, users); err != nil {
		log.Printf("syncConfig: apply config error: %v", err)
	} else {
		log.Printf("syncConfig: synced %d users to WireGuard", len(users))
	}
}

func (m *Monitor) collect() {
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

	db.BufferedSaveGlobalBandwidthPoint(currentRx, currentTx, rxSpeed, txSpeed)

	m.lastRx = currentRx
	m.lastTx = currentTx
	m.lastTime = now

	m.collectPerUserBandwidth()
}

func (m *Monitor) collectPerUserBandwidth() {
	peers, err := wg.GetPeersFromWgctl(m.interfaceName)
	if err != nil {
		log.Printf("GetPeersFromWgctl error: %v", err)
		return
	}

	wg.SavePeersToCache(peers)

	users, err := db.ListUsers()
	if err != nil {
		return
	}

	pubKeyToUser := make(map[string]int)
	userByID := make(map[int]db.User)
	for _, u := range users {
		pubKeyToUser[u.PublicKey] = u.ID
		userByID[u.ID] = u
	}

	now := time.Now()
	elapsed := now.Sub(m.lastUserTime).Seconds()
	if elapsed <= 0 {
		elapsed = 1
	}

	for _, peer := range peers {
		userID, exists := pubKeyToUser[peer.PublicKey]
		if !exists {
			continue
		}

		// 检测上线/离线
		prevHS := m.lastHandshake[userID]
		if prevHS == 0 && peer.LatestHandshake > 0 {
			if !db.HasActiveConnection(userID) {
				if u, ok := userByID[userID]; ok {
					db.RecordConnection(userID, u.Username, u.InternalIP, peer.Endpoint)
				}
			}
		} else if prevHS > 0 && peer.LatestHandshake == 0 {
			db.UpdateConnectionOnDisconnect(userID, peer.TransferRx, peer.TransferTx)
		}
		m.lastHandshake[userID] = peer.LatestHandshake

		// 会话流量缓存，随带宽 buffer 10s 一并 flush 到 DB
		db.BufferedSessionTraffic(userID, peer.TransferRx, peer.TransferTx)

		// 实时速度
		rxSpeed := float64(0)
		txSpeed := float64(0)
		if prevRx, ok := m.lastUserRx[userID]; ok {
			rxSpeed = math.Max(0, float64(peer.TransferRx-prevRx)/elapsed)
		}
		if prevTx, ok := m.lastUserTx[userID]; ok {
			txSpeed = math.Max(0, float64(peer.TransferTx-prevTx)/elapsed)
		}
		m.lastUserRx[userID] = peer.TransferRx
		m.lastUserTx[userID] = peer.TransferTx

		db.BufferedSaveBandwidthPoint(userID, peer.TransferRx, peer.TransferTx, rxSpeed, txSpeed)
	}
	m.lastUserTime = now
}
