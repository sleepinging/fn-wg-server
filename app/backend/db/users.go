package db

import (
	"database/sql"
	"fmt"
	"time"
)

// User represents a WireGuard peer user.
type User struct {
	ID                  int     `json:"id"`
	Username            string  `json:"username"`
	PublicKey           string  `json:"publicKey"`
	PrivateKey          string  `json:"privateKey,omitempty"`
	PresharedKey        string  `json:"presharedKey"`
	AllowedIPs          string  `json:"allowedIPs"`
	InternalIP          string  `json:"internalIP"`
	DNS                 string  `json:"dns"`
	MTU                 int     `json:"mtu"`
	PersistentKeepalive int     `json:"persistentKeepalive"`
	Enabled             bool    `json:"enabled"`
	CreatedAt           int64   `json:"createdAt"`
	UpdatedAt           int64   `json:"updatedAt"`
	RxBytes             int64   `json:"rxBytes,omitempty"`
	TxBytes             int64   `json:"txBytes,omitempty"`
	RxSpeed             float64 `json:"rxSpeed,omitempty"`
	TxSpeed             float64 `json:"txSpeed,omitempty"`
	Online              bool    `json:"online,omitempty"`
	ExternalIP          string  `json:"externalIP,omitempty"`
	OnlineSince         string  `json:"onlineSince,omitempty"`
}

// CreateUser inserts a new user.
func CreateUser(u User) (int64, error) {
	dbLock.RLock()
	defer dbLock.RUnlock()

	now := time.Now().UnixMilli()
	result, err := db.Exec(`INSERT INTO users 
		(username, public_key, private_key, preshared_key, allowed_ips, internal_ip, dns, mtu, persistent_keepalive, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		u.Username, u.PublicKey, u.PrivateKey, u.PresharedKey,
		u.AllowedIPs, u.InternalIP, u.DNS, u.MTU, u.PersistentKeepalive, u.Enabled,
		now, now)
	if err != nil {
		return 0, fmt.Errorf("create user: %w", err)
	}
	return result.LastInsertId()
}

// GetUserByID retrieves a user by ID.
func GetUserByID(id int) (*User, error) {
	dbLock.RLock()
	defer dbLock.RUnlock()

	u := &User{}
	err := db.QueryRow(`SELECT id, username, public_key, private_key, preshared_key,
		allowed_ips, internal_ip, dns, mtu, persistent_keepalive, enabled, created_at, updated_at
		FROM users WHERE id = ?`, id).Scan(
		&u.ID, &u.Username, &u.PublicKey, &u.PrivateKey, &u.PresharedKey,
		&u.AllowedIPs, &u.InternalIP, &u.DNS, &u.MTU, &u.PersistentKeepalive,
		&u.Enabled, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return u, nil
}

// GetUserByUsername retrieves a user by username.
func GetUserByUsername(username string) (*User, error) {
	dbLock.RLock()
	defer dbLock.RUnlock()

	u := &User{}
	err := db.QueryRow(`SELECT id, username, public_key, private_key, preshared_key,
		allowed_ips, internal_ip, dns, mtu, persistent_keepalive, enabled, created_at, updated_at
		FROM users WHERE username = ?`, username).Scan(
		&u.ID, &u.Username, &u.PublicKey, &u.PrivateKey, &u.PresharedKey,
		&u.AllowedIPs, &u.InternalIP, &u.DNS, &u.MTU, &u.PersistentKeepalive,
		&u.Enabled, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return u, nil
}

// ListUsers returns all users.
func ListUsers() ([]User, error) {
	dbLock.RLock()
	defer dbLock.RUnlock()

	rows, err := db.Query(`SELECT id, username, public_key, private_key, preshared_key,
		allowed_ips, internal_ip, dns, mtu, persistent_keepalive, enabled, created_at, updated_at
		FROM users ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	users := make([]User, 0)
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Username, &u.PublicKey, &u.PrivateKey, &u.PresharedKey,
			&u.AllowedIPs, &u.InternalIP, &u.DNS, &u.MTU, &u.PersistentKeepalive,
			&u.Enabled, &u.CreatedAt, &u.UpdatedAt); err != nil {
			continue
		}
		users = append(users, u)
	}
	return users, nil
}

// UpdateUser updates a user.
func UpdateUser(u User) error {
	dbLock.RLock()
	defer dbLock.RUnlock()

	_, err := db.Exec(`UPDATE users SET username=?, public_key=?, private_key=?, preshared_key=?,
		allowed_ips=?, internal_ip=?, dns=?, mtu=?, persistent_keepalive=?, enabled=?, updated_at=?
		WHERE id=?`,
		u.Username, u.PublicKey, u.PrivateKey, u.PresharedKey,
		u.AllowedIPs, u.InternalIP, u.DNS, u.MTU, u.PersistentKeepalive,
		u.Enabled, time.Now().UnixMilli(), u.ID)
	return err
}

// DeleteUser deletes a user by ID.
func DeleteUser(id int) error {
	dbLock.RLock()
	defer dbLock.RUnlock()

	_, err := db.Exec("DELETE FROM users WHERE id = ?", id)
	return err
}

// GetSmallestUnusedIP finds the smallest unused IP in the subnet.
func GetSmallestUnusedIP(subnet string) (string, error) {
	dbLock.RLock()
	defer dbLock.RUnlock()

	var baseIP string
	fmt.Sscanf(subnet, "%s/", &baseIP)

	rows, err := db.Query("SELECT internal_ip FROM users ORDER BY internal_ip")
	if err != nil {
		return "", err
	}
	defer rows.Close()

	usedIPs := make(map[string]bool)
	for rows.Next() {
		var ip string
		rows.Scan(&ip)
		usedIPs[ip] = true
	}

	var a, b, c, d int
	fmt.Sscanf(baseIP, "%d.%d.%d.%d", &a, &b, &c, &d)

	startIP := d + 1
	if startIP < 10 {
		startIP = 10
	}
	for i := startIP; i < 255; i++ {
		candidate := fmt.Sprintf("%d.%d.%d.%d/32", a, b, c, i)
		if !usedIPs[candidate] {
			return candidate, nil
		}
	}
	if c < 255 {
		for i := 1; i < 255; i++ {
			candidate := fmt.Sprintf("%d.%d.%d.%d/32", a, b, c+1, i)
			if !usedIPs[candidate] {
				return candidate, nil
			}
		}
	}

	return "", fmt.Errorf("no unused IP available in subnet %s", subnet)
}

// GetUserHistory gets connection history for a user.
func GetUserHistory(userID int, page, pageSize int) ([]map[string]interface{}, int, error) {
	dbLock.RLock()
	defer dbLock.RUnlock()

	var total int
	db.QueryRow("SELECT COUNT(*) FROM connection_log WHERE user_id = ?", userID).Scan(&total)

	rows, err := db.Query(`SELECT id, user_id, username, internal_ip, external_ip,
		connected_at, disconnected_at, rx_bytes, tx_bytes
		FROM connection_log WHERE user_id = ?
		ORDER BY connected_at DESC LIMIT ? OFFSET ?`,
		userID, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	history := make([]map[string]interface{}, 0)
	for rows.Next() {
		var id, uid int
		var uname, intIP, extIP string
		var connAt int64
		var discAt sql.NullInt64
		var rx, tx int64
		if err := rows.Scan(&id, &uid, &uname, &intIP, &extIP, &connAt, &discAt, &rx, &tx); err != nil {
			continue
		}
		discTime := int64(0)
		if discAt.Valid {
			discTime = discAt.Int64
		}
		history = append(history, map[string]interface{}{
			"id":             id,
			"userId":         uid,
			"username":       uname,
			"internalIP":     intIP,
			"externalIP":     extIP,
			"connectedAt":    connAt,
			"disconnectedAt": discTime,
			"rxBytes":        rx,
			"txBytes":        tx,
		})
	}
	return history, total, nil
}

// RecordConnection logs a connection event.
func RecordConnection(userID int, username, internalIP, externalIP string) error {
	dbLock.RLock()
	defer dbLock.RUnlock()

	_, err := db.Exec(`INSERT INTO connection_log 
		(user_id, username, internal_ip, external_ip, connected_at)
		VALUES (?, ?, ?, ?, ?)`,
		userID, username, internalIP, externalIP, time.Now().UnixMilli())
	return err
}

// UpdateConnectionOnDisconnect updates the disconnect time and traffic.
func UpdateConnectionOnDisconnect(userID int, rx, tx int64) error {
	dbLock.RLock()
	defer dbLock.RUnlock()

	_, err := db.Exec(`UPDATE connection_log SET 
		disconnected_at = ?, rx_bytes = ?, tx_bytes = ?
		WHERE user_id = ? AND disconnected_at IS NULL`,
		time.Now().UnixMilli(), rx, tx, userID)
	return err
}

// HasActiveConnection checks if user has an undisconnected connection.
func HasActiveConnection(userID int) bool {
	dbLock.RLock()
	defer dbLock.RUnlock()

	var count int
	db.QueryRow("SELECT COUNT(*) FROM connection_log WHERE user_id = ? AND disconnected_at IS NULL", userID).Scan(&count)
	return count > 0
}

// UpdateActiveConnectionTraffic updates rx/tx for active connections (periodic, not only on disconnect).
func UpdateActiveConnectionTraffic(userID int, rx, tx int64) {
	dbLock.RLock()
	defer dbLock.RUnlock()

	db.Exec(`UPDATE connection_log SET rx_bytes = ?, tx_bytes = ?
		WHERE user_id = ? AND disconnected_at IS NULL`, rx, tx, userID)
}

// GetActiveSessionTraffic returns the session rx/tx for display.
// Falls back to peer transfer values if no active connection record.
func GetActiveSessionTraffic(userID int, peerRx, peerTx int64) (int64, int64) {
	dbLock.RLock()
	defer dbLock.RUnlock()

	var rx, tx int64
	err := db.QueryRow(`SELECT rx_bytes, tx_bytes FROM connection_log
		WHERE user_id = ? AND disconnected_at IS NULL ORDER BY id DESC LIMIT 1`, userID).Scan(&rx, &tx)
	if err != nil {
		return peerRx, peerTx
	}
	return rx, tx
}
