package db

import (
	"fmt"
	"log"
	"sync"
	"time"
)

// ==================== 带宽数据缓冲 ====================

type bufferedPoint struct {
	UserID  int
	RxBytes int64
	TxBytes int64
	RxSpeed float64
	TxSpeed float64
	GoTs    int64 // 仅内存比较用，不写入 DB
}

// ==================== 全局写缓冲 ====================

type bufferedWrite struct {
	query string
	args  []interface{}
}

var (
	writeMu        sync.Mutex
	writeBuf       []bufferedWrite
	pointBuf       []bufferedPoint
	sessionTraffic = make(map[int][2]int64)
	flushInterval  time.Duration
	bufRunning     bool
	bufStopCh      chan struct{}
	lastCleanup    time.Time
)

// StartWriteBuffer initializes the global write buffer (server mode only).
func StartWriteBuffer(intervalSec int) {
	writeMu.Lock()
	defer writeMu.Unlock()
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
	log.Printf("Write buffer started (flush every %ds)", intervalSec)
}

// StopWriteBuffer stops the buffer and does a final flush.
func StopWriteBuffer() {
	writeMu.Lock()
	if !bufRunning {
		writeMu.Unlock()
		return
	}
	close(bufStopCh)
	bufRunning = false
	writeMu.Unlock()

	done := make(chan struct{}, 1)
	go func() {
		FlushBuffer()
		done <- struct{}{}
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		log.Println("Write buffer flush timed out")
	}
}

// BufferedExec adds a generic write to the buffer.
func BufferedExec(query string, args ...interface{}) {
	writeMu.Lock()
	writeBuf = append(writeBuf, bufferedWrite{query, args})
	writeMu.Unlock()
}

// FlushBuffer flushes ALL buffered writes (bandwidth + session traffic + generic) in one transaction.
func FlushBuffer() {
	writeMu.Lock()
	batch := pointBuf
	pointBuf = nil
	traffic := make(map[int][2]int64, len(sessionTraffic))
	for k, v := range sessionTraffic {
		traffic[k] = v
	}
	writes := writeBuf
	writeBuf = nil
	writeMu.Unlock()

	total := len(batch) + len(traffic) + len(writes)
	if total == 0 {
		return
	}

	dbLock.RLock()
	defer dbLock.RUnlock()

	tx, err := db.Begin()
	if err != nil {
		log.Printf("Buffer flush begin tx error: %v", err)
		return
	}

	// 1. Bandwidth batch insert (prepared statement)
	if len(batch) > 0 {
		stmt, err := tx.Prepare(`INSERT INTO bandwidth_history 
			(user_id, rx_bytes, tx_bytes, rx_speed, tx_speed, ts)
			VALUES (?, ?, ?, ?, ?, CAST(strftime('%s','now') AS INTEGER) * 1000)`)
		if err == nil {
			for _, p := range batch {
				stmt.Exec(p.UserID, p.RxBytes, p.TxBytes, p.RxSpeed, p.TxSpeed)
			}
			stmt.Close()
		} else {
			log.Printf("Buffer flush prepare error: %v", err)
		}
	}

	// 2. Session traffic updates
	for userID, v := range traffic {
		tx.Exec(`UPDATE connection_log SET rx_bytes = ?, tx_bytes = ?
			WHERE user_id = ? AND disconnected_at IS NULL`, v[0], v[1], userID)
	}

	// 3. Generic buffered writes
	for _, w := range writes {
		tx.Exec(w.query, w.args...)
	}

	if err := tx.Commit(); err != nil {
		log.Printf("Buffer flush commit error: %v", err)
	}

	cleanupOnce()
}

func flushLoop() {
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			FlushBuffer()
		case <-bufStopCh:
			return
		}
	}
}

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

// ==================== 带宽点 ====================

func BufferedSaveBandwidthPoint(userID int, rxBytes, txBytes int64, rxSpeed, txSpeed float64) {
	writeMu.Lock()
	pointBuf = append(pointBuf, bufferedPoint{
		UserID: userID, RxBytes: rxBytes, TxBytes: txBytes,
		RxSpeed: rxSpeed, TxSpeed: txSpeed,
		GoTs: time.Now().UnixMilli(),
	})
	writeMu.Unlock()
}

func BufferedSaveGlobalBandwidthPoint(rxBytes, txBytes int64, rxSpeed, txSpeed float64) {
	BufferedSaveBandwidthPoint(0, rxBytes, txBytes, rxSpeed, txSpeed)
}

func BufferedSessionTraffic(userID int, rx, tx int64) {
	writeMu.Lock()
	sessionTraffic[userID] = [2]int64{rx, tx}
	writeMu.Unlock()
}

// ==================== 缓存查询（for chart API）====================

func CountBufferedAfter(userID int, sinceMs int64) int {
	writeMu.Lock()
	defer writeMu.Unlock()
	count := 0
	// 用 SQLite 时间作为 buffer 点的时间戳（Go 的时钟不可靠）
	dbTs := getDBTimestamp()
	for i, p := range pointBuf {
		if p.UserID == userID {
			ts := dbTs - int64(len(pointBuf)-1-i)*1000
			if ts > sinceMs {
				count++
			}
		}
	}
	return count
}

func GetBufferedPointsAfter(userID int, sinceMs int64) []BandwidthPoint {
	writeMu.Lock()
	defer writeMu.Unlock()
	var result []BandwidthPoint
	// 用 SQLite 时间作为 buffer 点的时间戳（Go 的时钟不可靠）
	dbTs := getDBTimestamp()
	for i, p := range pointBuf {
		if p.UserID == userID {
			// 倒推时间：最新的 buffer 点 = dbTs，之前每个点 -1 秒
			ts := dbTs - int64(len(pointBuf)-1-i)*1000
			if ts > sinceMs {
				result = append(result, BandwidthPoint{
					Ts: ts, RxBytes: p.RxBytes, TxBytes: p.TxBytes,
					RxSpeed: p.RxSpeed, TxSpeed: p.TxSpeed,
				})
			}
		}
	}
	return result
}

func getDBTimestamp() int64 {
	dbLock.RLock()
	defer dbLock.RUnlock()
	var ts int64
	db.QueryRow("SELECT CAST(strftime('%s','now') AS INTEGER) * 1000").Scan(&ts)
	return ts
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
