package db

import (
	"database/sql"
	"sort"
)

// BandwidthPoint represents a single bandwidth data point.
type BandwidthPoint struct {
	Ts      int64   `json:"ts"`
	RxBytes int64   `json:"rxBytes"`
	TxBytes int64   `json:"txBytes"`
	RxSpeed float64 `json:"rxSpeed"`
	TxSpeed float64 `json:"txSpeed"`
}

// GetBandwidthHistoryAgg 查询带宽历史，支持采样/聚合
// startTs/endTs 是毫秒时间戳，0 表示不限
// maxPoints > 0 时均匀采样/聚合到 maxPoints 个点
func GetBandwidthHistoryAgg(userID int, startTs, endTs int64, maxPoints int, aggregate string) ([]BandwidthPoint, error) {
	return getBandwidthHistoryAgg(userID, startTs, endTs, maxPoints, aggregate)
}

func getBandwidthHistoryAgg(userID int, startTs, endTs int64, maxPoints int, aggregate string) ([]BandwidthPoint, error) {
	dbLock.RLock()
	defer dbLock.RUnlock()

	query := `SELECT ts, rx_bytes, tx_bytes, rx_speed, tx_speed 
		FROM bandwidth_history WHERE user_id = ?`
	args := []interface{}{userID}

	if startTs > 0 {
		query += " AND ts > ?"
		args = append(args, startTs)
	}
	if endTs > 0 {
		query += " AND ts <= ?"
		args = append(args, endTs)
	}
	query += " ORDER BY ts ASC"

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// 用 map 去重：时间戳 → 点（缓冲区中更新的覆盖 DB 的）
	pointMap := make(map[int64]BandwidthPoint)
	for rows.Next() {
		var p BandwidthPoint
		if err := rows.Scan(&p.Ts, &p.RxBytes, &p.TxBytes, &p.RxSpeed, &p.TxSpeed); err != nil {
			continue
		}
		pointMap[p.Ts] = p
	}

	// 从缓冲区补充尚未 flush 的数据
	bufferPoints := GetBufferedPointsAfter(userID, startTs)
	for _, p := range bufferPoints {
		pointMap[p.Ts] = p
	}

	// 转为有序列表
	points := make([]BandwidthPoint, 0, len(pointMap))
	for _, p := range pointMap {
		points = append(points, p)
	}
	sort.Slice(points, func(i, j int) bool {
		return points[i].Ts < points[j].Ts
	})

	// 均匀采样/聚合：如果点数超过 maxPoints
	if maxPoints > 0 && len(points) > maxPoints {
		step := float64(len(points)) / float64(maxPoints)
		sampled := make([]BandwidthPoint, 0, maxPoints)

		if aggregate == "max" || aggregate == "avg" {
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
				p.Ts = window[0].Ts
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
				p.RxBytes = window[len(window)-1].RxBytes
				p.TxBytes = window[len(window)-1].TxBytes
				sampled = append(sampled, p)
			}
		} else {
			for i := 0; i < maxPoints; i++ {
				idx := int(float64(i) * step)
				if idx >= len(points) {
					idx = len(points) - 1
				}
				sampled = append(sampled, points[idx])
			}
			if len(sampled) > 0 && sampled[len(sampled)-1].Ts != points[len(points)-1].Ts {
				sampled[len(sampled)-1] = points[len(points)-1]
			}
		}
		return sampled, nil
	}

	return points, nil
}

// CountBandwidthAfter 统计 user_id 在 sinceMs 之后有多少条记录（含缓冲区中未 flush 的数据）
func CountBandwidthAfter(userID int, sinceMs int64) (int64, error) {
	dbLock.RLock()
	defer dbLock.RUnlock()

	var count int64
	err := db.QueryRow("SELECT COUNT(*) FROM bandwidth_history WHERE user_id = ? AND ts > ?", userID, sinceMs).Scan(&count)
	if err != nil {
		count = 0
	}

	// 加上缓冲区中尚未 flush 的点
	count += int64(CountBufferedAfter(userID, sinceMs))

	return count, nil
}

// GetLatestBandwidth gets the latest bandwidth point (buffer first, then DB).
func GetLatestBandwidth(userID int) (*BandwidthPoint, error) {
	// Buffer always has freshest data (within last 10s)
	if bufLatest := GetLatestBufferedPoint(userID); bufLatest != nil {
		return bufLatest, nil
	}

	dbLock.RLock()
	defer dbLock.RUnlock()

	p := &BandwidthPoint{}
	err := db.QueryRow(`SELECT ts, rx_bytes, tx_bytes, rx_speed, tx_speed
		FROM bandwidth_history WHERE user_id = ?
		ORDER BY ts DESC LIMIT 1`, userID).
		Scan(&p.Ts, &p.RxBytes, &p.TxBytes, &p.RxSpeed, &p.TxSpeed)
	if err != nil {
		return nil, err
	}
	return p, nil
}

// GetLatestBufferedPoint returns the latest buffered point for a user.
func GetLatestBufferedPoint(userID int) *BandwidthPoint {
	writeMu.Lock()
	defer writeMu.Unlock()
	var latest *BandwidthPoint
	for _, p := range pointBuf {
		if p.UserID == userID || (userID == 0 && p.UserID == 0) {
			if latest == nil || p.GoTs > latest.Ts {
				latest = &BandwidthPoint{Ts: p.GoTs, RxBytes: p.RxBytes, TxBytes: p.TxBytes, RxSpeed: p.RxSpeed, TxSpeed: p.TxSpeed}
			}
		}
	}
	return latest
}

// GetUserTotalTraffic gets total traffic for a user across all sessions.
func GetUserTotalTraffic(userID int) (rx int64, tx int64, err error) {
	dbLock.RLock()
	defer dbLock.RUnlock()

	err = db.QueryRow(`SELECT rx_bytes, tx_bytes FROM bandwidth_history 
		WHERE user_id = ? ORDER BY id DESC LIMIT 1`, userID).Scan(&rx, &tx)
	if err == sql.ErrNoRows {
		err = nil
		rx = 0
		tx = 0
	}
	return
}
