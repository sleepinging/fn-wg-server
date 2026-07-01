package db

import (
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

	startTime = NormalizeTimeParam(startTime)
	endTime = NormalizeTimeParam(endTime)

	// 从 DB 查
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

	// 用 map 去重：时间戳 → 点（缓冲区中更新的覆盖 DB 的）
	pointMap := make(map[string]BandwidthPoint)
	for rows.Next() {
		var p BandwidthPoint
		if err := rows.Scan(&p.Timestamp, &p.RxBytes, &p.TxBytes, &p.RxSpeed, &p.TxSpeed); err != nil {
			continue
		}
		p.Timestamp = toLocalTime(p.Timestamp)
		pointMap[p.Timestamp] = p
	}

	// 从缓冲区补充尚未 flush 的数据
	var bufSince time.Time
	if startTime != "" {
		bufSince, _ = time.Parse("2006-01-02 15:04:05", startTime)
	}
	bufferPoints := GetBufferedPointsAfter(userID, bufSince)
	for _, p := range bufferPoints {
		pointMap[p.Timestamp] = p
	}

	// 转为有序列表
	points := make([]BandwidthPoint, 0, len(pointMap))
	for _, p := range pointMap {
		points = append(points, p)
	}
	// 按时间排序
	for i := 0; i < len(points); i++ {
		for j := i + 1; j < len(points); j++ {
			if points[i].Timestamp > points[j].Timestamp {
				points[i], points[j] = points[j], points[i]
			}
		}
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

// CountBandwidthAfter 统计 user_id 在 since 之后有多少条记录（含缓冲区中未 flush 的数据）
func CountBandwidthAfter(userID int, since string) (int64, error) {
	dbLock.RLock()
	defer dbLock.RUnlock()

	var count int64
	err := db.QueryRow("SELECT COUNT(*) FROM bandwidth_history WHERE user_id = ? AND timestamp > ?", userID, since).Scan(&count)
	if err != nil {
		count = 0
	}

	// 加上缓冲区中尚未 flush 的点
	sinceTime, _ := time.Parse("2006-01-02 15:04:05", since)
	if !sinceTime.IsZero() {
		count += int64(CountBufferedAfter(userID, sinceTime))
	}

	return count, nil
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

// NormalizeTimeParam 统一时间参数格式
// 前端可能传 ISO 8601（如 2026-07-01T12:36:55.000Z），DB 存储的是 Asia/Shanghai YYYY-MM-DD HH:MM:SS
func NormalizeTimeParam(t string) string {
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
