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
// maxPoints 限制返回最大点数（0 表示不限制），超过时均匀采样
func GetBandwidthHistory(userID int, startTime, endTime string) ([]BandwidthPoint, error) {
	return GetBandwidthHistoryLimit(userID, startTime, endTime, 0)
}

// GetBandwidthHistoryLimit 与 GetBandwidthHistory 相同，但限制最大返回点数
// aggregate 支持 ""（采样）、"max"（窗口内最大值）、"avg"（窗口内平均值）
func GetBandwidthHistoryLimit(userID int, startTime, endTime string, maxPoints int) ([]BandwidthPoint, error) {
	return getBandwidthHistoryAgg(userID, startTime, endTime, maxPoints, "")
}

// GetBandwidthHistoryAgg 支持聚合模式
func GetBandwidthHistoryAgg(userID int, startTime, endTime string, maxPoints int, aggregate string) ([]BandwidthPoint, error) {
	return getBandwidthHistoryAgg(userID, startTime, endTime, maxPoints, aggregate)
}

func getBandwidthHistoryAgg(userID int, startTime, endTime string, maxPoints int, aggregate string) ([]BandwidthPoint, error) {
	dbLock.RLock()
	defer dbLock.RUnlock()

	startTime = normalizeTimeParam(startTime)
	endTime = normalizeTimeParam(endTime)

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

	// 均匀采样/聚合：如果点数超过 maxPoints
	if maxPoints > 0 && len(points) > maxPoints {
		step := float64(len(points)) / float64(maxPoints)
		sampled := make([]BandwidthPoint, 0, maxPoints)

		if aggregate == "max" || aggregate == "avg" {
			// 按窗口聚合
			for i := 0; i < maxPoints; i++ {
				startIdx := int(float64(i) * step)
				endIdx := int(float64(i+1) * step)
				if endIdx > len(points) {
					endIdx = len(points)
				}
				if startIdx >= len(points) {
					startIdx = len(points) - 1
				}
				if startIdx >= endIdx {
					continue
				}

				window := points[startIdx:endIdx]
				if len(window) == 0 {
					continue
				}

				var p BandwidthPoint
				p.Timestamp = window[0].Timestamp
				if aggregate == "max" {
					for _, wp := range window {
						if wp.RxSpeed > p.RxSpeed {
							p.RxSpeed = wp.RxSpeed
						}
						if wp.TxSpeed > p.TxSpeed {
							p.TxSpeed = wp.TxSpeed
						}
					}
				} else { // avg
					var sumRx, sumTx float64
					for _, wp := range window {
						sumRx += wp.RxSpeed
						sumTx += wp.TxSpeed
					}
					p.RxSpeed = sumRx / float64(len(window))
					p.TxSpeed = sumTx / float64(len(window))
				}
				// 累计流量取窗口最后一个
				p.RxBytes = window[len(window)-1].RxBytes
				p.TxBytes = window[len(window)-1].TxBytes
				sampled = append(sampled, p)
			}
		} else {
			// 均匀采样
			for i := 0; i < maxPoints; i++ {
				idx := int(float64(i) * step)
				if idx >= len(points) {
					idx = len(points) - 1
				}
				sampled = append(sampled, points[idx])
			}
			if len(sampled) > 0 && sampled[len(sampled)-1].Timestamp != points[len(points)-1].Timestamp {
				sampled[len(sampled)-1] = points[len(points)-1]
			}
		}
		return sampled, nil
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

// normalizeTimeParam 统一时间参数格式
// 前端可能传 ISO 8601（如 2026-07-01T12:36:55.000Z），DB 存储的是 Asia/Shanghai YYYY-MM-DD HH:MM:SS
func normalizeTimeParam(t string) string {
	if t == "" {
		return t
	}
	var parsed time.Time
	var err error
	formats := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02T15:04:05.000Z",
	}
	for _, f := range formats {
		parsed, err = time.Parse(f, t)
		if err == nil {
			break
		}
	}
	if err != nil {
		return t
	}
	return parsed.In(time.Local).Format("2006-01-02 15:04:05")
}
