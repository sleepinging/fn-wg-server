package daemon

import (
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"
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
	// 配置触发文件：CGI 写入 -> 守护进程读取并执行（守护进程有 root 权限）
	configTriggerFile string
}

// NewMonitor creates a new bandwidth monitor.
func NewMonitor(interfaceName, dataDir string) *Monitor {
	// 设置对等端缓存目录，供 CGI 读取
	wg.SetPeersCacheDir(dataDir)
	return &Monitor{
		interfaceName:     interfaceName,
		dataDir:           dataDir,
		stopCh:            make(chan struct{}),
		pidFile:           filepath.Join(dataDir, "monitor.pid"),
		configTriggerFile: filepath.Join(dataDir, "config.trigger"),
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
			m.checkConfigTrigger()
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

// checkConfigTrigger 检查是否有配置触发文件，有则应用 WireGuard 配置（守护进程有 root 权限）
func (m *Monitor) checkConfigTrigger() {
	if _, err := os.Stat(m.configTriggerFile); err != nil {
		return // 无触发文件
	}
	
	// 读取触发文件内容（可以包含特定指令）
	data, err := os.ReadFile(m.configTriggerFile)
	if err != nil {
		log.Printf("Config trigger read error: %v", err)
		os.Remove(m.configTriggerFile)
		return
	}
	
	// 删除触发文件，防止重复执行
	os.Remove(m.configTriggerFile)
	
	action := strings.TrimSpace(string(data))
	log.Printf("Config trigger: %s", action)
	
	switch action {
	case "apply":
		// 从数据库读取配置并应用
		cfg, err := wg.LoadConfig()
		if err != nil {
			log.Printf("Load config error: %v", err)
			return
		}
		users, err := db.ListUsers()
		if err != nil {
			log.Printf("List users error: %v", err)
			return
		}
		if err := wg.ApplyConfig(*cfg, users); err != nil {
			log.Printf("Apply config error: %v", err)
		}
	case "init":
		if err := wg.InitInterface(); err != nil {
			log.Printf("Init interface error: %v", err)
		}
	default:
		log.Printf("Unknown trigger action: %s", action)
	}
}

// WriteConfigTrigger 供 CGI 调用的触发函数（写入触发文件，由守护进程执行）
func WriteConfigTrigger(dataDir, action string) error {
	triggerFile := filepath.Join(dataDir, "config.trigger")
	return os.WriteFile(triggerFile, []byte(action), 0644)
}

func (m *Monitor) collectPerUserBandwidth() {
	// 守护进程以 root 权限运行，可以直接使用 wgctrl 读取对等端信息
	peers, err := wg.GetPeersFromWgctl(m.interfaceName)
	if err != nil {
		log.Printf("GetPeersFromWgctl error: %v", err)
		return
	}

	// 保存到缓存文件供 CGI 读取
	if err := wg.SavePeersToCache(peers); err != nil {
		log.Printf("SavePeersToCache error: %v", err)
	}

	users, err := db.ListUsers()
	if err != nil {
		return
	}

	pubKeyToUser := make(map[string]int)
	for _, u := range users {
		pubKeyToUser[u.PublicKey] = u.ID
	}

	for _, peer := range peers {
		userID, exists := pubKeyToUser[peer.PublicKey]
		if !exists {
			continue
		}

		if err := db.SaveBandwidthPoint(userID, peer.TransferRx, peer.TransferTx, 0, 0); err != nil {
			log.Printf("Failed to save user bandwidth (user %d): %v", userID, err)
		}
	}
}
