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
	collectCount int64
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

	// 初始化带宽缓存写入器（减少磁盘写入频率）
	flushInterval := db.GetConfigFlushInterval()
	db.InitBandwidthBuffer(flushInterval)
	log.Printf("Bandwidth buffer flush interval: %ds", flushInterval)

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
		// 等待采集 goroutine 退出，带超时
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
		db.StopBandwidthBuffer()
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

	// 重置 per-user 速度缓存（防止上次守护进程运行时的残留数据）
	m.lastUserRx = make(map[int]int64)
	m.lastUserTx = make(map[int]int64)
	m.lastUserTime = time.Now()
	m.lastHandshake = make(map[int]int64)
	m.collectCount = 0

	// 启动时自动同步 DB 用户到 WireGuard 内核（防止之前新增用户时守护进程不在线）
	m.syncConfig()

	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.collect()
		}
	}
}

// syncConfig 读取 DB 配置和用户，同步到 WireGuard 内核
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
	m.collectCount++
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

	// Save global bandwidth point（缓存写入，非实时入库）
	db.BufferedSaveGlobalBandwidthPoint(currentRx, currentTx, rxSpeed, txSpeed)

	m.lastRx = currentRx
	m.lastTx = currentTx
	m.lastTime = now

	// Collect per-user bandwidth
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

		// 检测上线/离线（握手时间变化）
		prevHS := m.lastHandshake[userID]
		if prevHS == 0 && peer.LatestHandshake > 0 {
			// 上线：仅当没有活跃连接时才记录（去重）
			if !db.HasActiveConnection(userID) {
				if u, ok := userByID[userID]; ok {
					db.RecordConnection(userID, u.Username, u.InternalIP, peer.Endpoint)
				}
			}
		} else if prevHS > 0 && peer.LatestHandshake == 0 {
			// 离线
			db.UpdateConnectionOnDisconnect(userID, peer.TransferRx, peer.TransferTx)
		}
		m.lastHandshake[userID] = peer.LatestHandshake

		// 每 30s 更新活跃连接的流量（不是只在离线时写）
		if peer.LatestHandshake > 0 && m.collectCount%30 == 0 {
			db.UpdateActiveConnectionTraffic(userID, peer.TransferRx, peer.TransferTx)
		}

		// 计算实时速度
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

		// 缓存写入，非实时入库
		db.BufferedSaveBandwidthPoint(userID, peer.TransferRx, peer.TransferTx, rxSpeed, txSpeed)
	}
	m.lastUserTime = now
}
