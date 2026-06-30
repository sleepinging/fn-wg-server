package db

import (
	"fmt"
	"sync"
	"time"
)

// BandwidthPoint represents a single bandwidth data point.
type BandwidthPoint struct {
	Timestamp string  `json:"timestamp"`
	RxBytes   int64   `json:"rxBytes"`
	TxBytes   int64   `json:"txBytes"`
	RxSpeed   float64 `json:"rxSpeed"`
	TxSpeed   float64 `json:"txSpeed"`
}

// SaveBandwidthPoint saves a bandwidth data point.
func SaveBandwidthPoint(userID int, rxBytes, txBytes int64, rxSpeed, txSpeed float64) error {
	dbLock.RLock()
	defer dbLock.RUnlock()

	_, err := db.Exec(`INSERT INTO bandwidth_history 
		(user_id, rx_bytes, tx_bytes, rx_speed, tx_speed, timestamp)
		VALUES (?, ?, ?, ?, ?, ?)`,
		userID, rxBytes, txBytes, rxSpeed, txSpeed, time.Now().Format("2006-01-02 15:04:05"))
	if err != nil {
		return fmt.Errorf("save bandwidth point: %w", err)
	}

	// Clean old data - keep configurable days (default 7)
	days := 7
	if d, err := GetConfig("history_retention_days"); err == nil && d != "" {
		fmt.Sscanf(d, "%d", &days)
	}
	if days <= 0 {
		days = 7
	}
	cutoff := time.Now().AddDate(0, 0, -days)
	db.Exec("DELETE FROM bandwidth_history WHERE timestamp < ?", cutoff.Format("2006-01-02 15:04:05"))

	return nil
}

// SaveGlobalBandwidthPoint saves a global bandwidth data point (user_id = 0).
func SaveGlobalBandwidthPoint(rxBytes, txBytes int64, rxSpeed, txSpeed float64) error {
	return SaveBandwidthPoint(0, rxBytes, txBytes, rxSpeed, txSpeed)
}

var bandwidthCache []BandwidthPoint
var bandwidthCacheLock sync.RWMutex
var lastCacheTime time.Time

// GetBandwidthHistory retrieves bandwidth history for a user.
func GetBandwidthHistory(userID int, startTime, endTime string) ([]BandwidthPoint, error) {
	dbLock.RLock()
	defer dbLock.RUnlock()

	query := `SELECT timestamp, rx_bytes, tx_bytes, rx_speed, tx_speed 
		FROM bandwidth_history WHERE user_id = ?`
	args := []interface{}{userID}

	if startTime != "" {
		query += " AND timestamp >= ?"
		args = append(args, startTime)
	}
	if endTime != "" {
		query += " AND timestamp <= ?"
		args = append(args, endTime)
	}
	query += " ORDER BY timestamp ASC"

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	points := make([]BandwidthPoint, 0)
	for rows.Next() {
		var p BandwidthPoint
		if err := rows.Scan(&p.Timestamp, &p.RxBytes, &p.TxBytes, &p.RxSpeed, &p.TxSpeed); err != nil {
			continue
		}
		p.Timestamp = toLocalTime(p.Timestamp)
		points = append(points, p)
	}
	return points, nil
}

// GetAggregatedBandwidth returns aggregated bandwidth for a time range.
func GetAggregatedBandwidth(userID int, startTime, endTime string) (rxSpeedAvg, txSpeedAvg float64, totalRx, totalTx int64, err error) {
	dbLock.RLock()
	defer dbLock.RUnlock()

	query := `SELECT COALESCE(AVG(rx_speed),0), COALESCE(AVG(tx_speed),0),
		COALESCE(MAX(rx_bytes),0), COALESCE(MAX(tx_bytes),0)
		FROM bandwidth_history WHERE user_id = ?`
	args := []interface{}{userID}

	if startTime != "" {
		query += " AND timestamp >= ?"
		args = append(args, startTime)
	}
	if endTime != "" {
		query += " AND timestamp <= ?"
		args = append(args, endTime)
	}

	err = db.QueryRow(query, args...).Scan(&rxSpeedAvg, &txSpeedAvg, &totalRx, &totalTx)
	return
}

// GetLatestBandwidth gets the latest bandwidth point for a user.
func GetLatestBandwidth(userID int) (*BandwidthPoint, error) {
	dbLock.RLock()
	defer dbLock.RUnlock()

	p := &BandwidthPoint{}
	err := db.QueryRow(`SELECT timestamp, rx_bytes, tx_bytes, rx_speed, tx_speed
		FROM bandwidth_history WHERE user_id = ?
		ORDER BY timestamp DESC LIMIT 1`, userID).
		Scan(&p.Timestamp, &p.RxBytes, &p.TxBytes, &p.RxSpeed, &p.TxSpeed)
	if err != nil {
		return nil, err
	}
	return p, nil
}

// GetCurrentBandwidthStats gets the latest bandwidth stats for all users.
func GetCurrentBandwidthStats() (map[int]*BandwidthPoint, error) {
	dbLock.RLock()
	defer dbLock.RUnlock()

	rows, err := db.Query(`SELECT b1.user_id, b1.timestamp, b1.rx_bytes, b1.tx_bytes, b1.rx_speed, b1.tx_speed
		FROM bandwidth_history b1
		INNER JOIN (
			SELECT user_id, MAX(timestamp) as max_ts
			FROM bandwidth_history
			GROUP BY user_id
		) b2 ON b1.user_id = b2.user_id AND b1.timestamp = b2.max_ts
		WHERE b1.user_id > 0`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	stats := make(map[int]*BandwidthPoint)
	for rows.Next() {
		var uid int
		var p BandwidthPoint
		if err := rows.Scan(&uid, &p.Timestamp, &p.RxBytes, &p.TxBytes, &p.RxSpeed, &p.TxSpeed); err != nil {
			continue
		}
		stats[uid] = &p
	}
	return stats, nil
}
