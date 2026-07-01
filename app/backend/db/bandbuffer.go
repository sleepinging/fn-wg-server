package db

import (
	"fmt"
	"log"
	"sync"
	"time"
)

// bufferedPoint 带 userID 的带宽数据点
type bufferedPoint struct {
	UserID   int
	RxBytes  int64
	TxBytes  int64
	RxSpeed  float64
	TxSpeed  float64
	Time     time.Time
}

var (
	bufMu      sync.Mutex
	pointBuf   []bufferedPoint
	flushInterval time.Duration // 刷新间隔
	bufRunning bool
	bufStopCh  chan struct{}
)

// InitBandwidthBuffer 初始化带宽缓存写入器
// intervalSec: 缓存多少秒后批量写入（默认 10）
func InitBandwidthBuffer(intervalSec int) {
	bufMu.Lock()
	defer bufMu.Unlock()

	if bufRunning {
		return
	}

	if intervalSec <= 0 {
		intervalSec = 10
	}
	flushInterval = time.Duration(intervalSec) * time.Second
	bufStopCh = make(chan struct{})
	bufRunning = true

	go flushLoop()
	log.Printf("Bandwidth buffer initialized (flush every %ds)", intervalSec)
}

// StopBandwidthBuffer 停止缓存写入器
func StopBandwidthBuffer() {
	bufMu.Lock()
	defer bufMu.Unlock()
	if bufRunning {
		close(bufStopCh)
		bufRunning = false
		flushNow() // 最后一次刷入
	}
}

// BufferedSaveBandwidthPoint 将带宽点写入缓存（替代直接 SaveBandwidthPoint）
func BufferedSaveBandwidthPoint(userID int, rxBytes, txBytes int64, rxSpeed, txSpeed float64) {
	bufMu.Lock()
	defer bufMu.Unlock()
	pointBuf = append(pointBuf, bufferedPoint{
		UserID:  userID,
		RxBytes: rxBytes,
		TxBytes: txBytes,
		RxSpeed: rxSpeed,
		TxSpeed: txSpeed,
		Time:    time.Now(),
	})
}

// BufferedSaveGlobalBandwidthPoint 全局带宽点缓存
func BufferedSaveGlobalBandwidthPoint(rxBytes, txBytes int64, rxSpeed, txSpeed float64) {
	BufferedSaveBandwidthPoint(0, rxBytes, txBytes, rxSpeed, txSpeed)
}

func flushLoop() {
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			flushNow()
		case <-bufStopCh:
			return
		}
	}
}

func flushNow() {
	bufMu.Lock()
	if len(pointBuf) == 0 {
		bufMu.Unlock()
		return
	}
	batch := pointBuf
	pointBuf = nil // 清空
	bufMu.Unlock()

	// 批量写入 DB
	if err := batchInsert(batch); err != nil {
		log.Printf("Bandwidth batch insert error: %v", err)
		// 失败时加回缓冲区？简化处理：丢弃，下个周期再采集
	}

	// 清理旧数据（每天只执行一次）
	cleanupOnce()
}

// batchInsert 批量插入带宽数据
func batchInsert(batch []bufferedPoint) error {
	dbLock.RLock()
	defer dbLock.RUnlock()

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`INSERT INTO bandwidth_history 
		(user_id, rx_bytes, tx_bytes, rx_speed, tx_speed, timestamp)
		VALUES (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare: %w", err)
	}
	defer stmt.Close()

	for _, p := range batch {
		ts := p.Time.Format("2006-01-02 15:04:05")
		if _, err := stmt.Exec(p.UserID, p.RxBytes, p.TxBytes, p.RxSpeed, p.TxSpeed, ts); err != nil {
			return fmt.Errorf("insert: %w", err)
		}
	}

	return tx.Commit()
}

var lastCleanup time.Time

// cleanupOnce 每天删除一次过期数据
func cleanupOnce() {
	if time.Since(lastCleanup) < 24*time.Hour {
		return
	}
	lastCleanup = time.Now()

	dbLock.RLock()
	defer dbLock.RUnlock()

	days := 7
	if d, err := GetConfig("history_retention_days"); err == nil && d != "" {
		fmt.Sscanf(d, "%d", &days)
	}
	if days <= 0 {
		days = 7
	}
	cutoff := time.Now().AddDate(0, 0, -days)
	db.Exec("DELETE FROM bandwidth_history WHERE timestamp < ?", cutoff.Format("2006-01-02 15:04:05"))
}

// GetConfigFlushInterval 获取缓存刷新间隔（秒）
func GetConfigFlushInterval() int {
	v, err := GetConfig("bandwidth_flush_interval")
	if err != nil || v == "" {
		return 10
	}
	var sec int
	fmt.Sscanf(v, "%d", &sec)
	if sec <= 0 {
		return 10
	}
	return sec
}
