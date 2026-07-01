package db

import (
	"fmt"
	"log"
	"sync"
	"time"
)

// bufferedPoint 带 userID 的带宽数据点
type bufferedPoint struct {
	UserID  int
	RxBytes int64
	TxBytes int64
	RxSpeed float64
	TxSpeed float64
	Ts      int64 // 毫秒时间戳
}

var (
	bufMu          sync.Mutex
	pointBuf       []bufferedPoint
	flushInterval  time.Duration
	bufRunning     bool
	bufStopCh      chan struct{}
)

// InitBandwidthBuffer 初始化带宽缓存写入器
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
	if !bufRunning {
		bufMu.Unlock()
		return
	}
	close(bufStopCh)
	bufRunning = false
	bufMu.Unlock()

	done := make(chan struct{}, 1)
	go func() {
		flushNow()
		done <- struct{}{}
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		log.Println("Bandwidth buffer flush timed out, discarding")
	}
}

// BufferedSaveBandwidthPoint 将带宽点写入缓存
func BufferedSaveBandwidthPoint(userID int, rxBytes, txBytes int64, rxSpeed, txSpeed float64) {
	bufMu.Lock()
	defer bufMu.Unlock()
	pointBuf = append(pointBuf, bufferedPoint{
		UserID:  userID,
		RxBytes: rxBytes,
		TxBytes: txBytes,
		RxSpeed: rxSpeed,
		TxSpeed: txSpeed,
		Ts:      time.Now().UnixMilli(),
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
	pointBuf = nil
	bufMu.Unlock()

	if err := batchInsert(batch); err != nil {
		log.Printf("Bandwidth batch insert error: %v", err)
	}

	cleanupOnce()
}

// batchInsert 批量插入带宽数据（ms 时间戳）
func batchInsert(batch []bufferedPoint) error {
	dbLock.RLock()
	defer dbLock.RUnlock()

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`INSERT INTO bandwidth_history 
		(user_id, rx_bytes, tx_bytes, rx_speed, tx_speed, ts)
		VALUES (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare: %w", err)
	}
	defer stmt.Close()

	for _, p := range batch {
		if _, err := stmt.Exec(p.UserID, p.RxBytes, p.TxBytes, p.RxSpeed, p.TxSpeed, p.Ts); err != nil {
			return fmt.Errorf("insert: %w", err)
		}
	}

	return tx.Commit()
}

var lastCleanup time.Time

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
	cutoff := time.Now().AddDate(0, 0, -days).UnixMilli()
	db.Exec("DELETE FROM bandwidth_history WHERE ts < ?", cutoff)
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

// CountBufferedAfter 统计缓冲区中 user_id 在 sinceMs 之后的点数
func CountBufferedAfter(userID int, sinceMs int64) int {
	bufMu.Lock()
	defer bufMu.Unlock()
	count := 0
	for _, p := range pointBuf {
		if (userID == 0 || p.UserID == userID) && p.Ts > sinceMs {
			count++
		}
	}
	return count
}

// GetBufferedPointsAfter 返回缓冲区中 user_id 在 sinceMs 之后的点
func GetBufferedPointsAfter(userID int, sinceMs int64) []BandwidthPoint {
	bufMu.Lock()
	defer bufMu.Unlock()
	var result []BandwidthPoint
	for _, p := range pointBuf {
		if (userID == 0 || p.UserID == userID) && p.Ts > sinceMs {
			result = append(result, BandwidthPoint{
				Ts:      p.Ts,
				RxBytes: p.RxBytes,
				TxBytes: p.TxBytes,
				RxSpeed: p.RxSpeed,
				TxSpeed: p.TxSpeed,
			})
		}
	}
	return result
}
