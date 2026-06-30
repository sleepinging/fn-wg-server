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
	CreatedAt           string  `json:"createdAt"`
	UpdatedAt           string  `json:"updatedAt"`
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

	result, err := db.Exec(`INSERT INTO users 
		(username, public_key, private_key, preshared_key, allowed_ips, internal_ip, dns, mtu, persistent_keepalive, enabled)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		u.Username, u.PublicKey, u.PrivateKey, u.PresharedKey,
		u.AllowedIPs, u.InternalIP, u.DNS, u.MTU, u.PersistentKeepalive, u.Enabled)
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
		allowed_ips=?, internal_ip=?, dns=?, mtu=?, persistent_keepalive=?, enabled=?, updated_at=CURRENT_TIMESTAMP
		WHERE id=?`,
		u.Username, u.PublicKey, u.PrivateKey, u.PresharedKey,
		u.AllowedIPs, u.InternalIP, u.DNS, u.MTU, u.PersistentKeepalive,
		u.Enabled, u.ID)
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

	// subnet format: "192.168.5.0/24"
	// Extract base IP
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

	// Parse the base IP
	var a, b, c, d int
	fmt.Sscanf(baseIP, "%d.%d.%d.%d", &a, &b, &c, &d)

	// 从 10 开始分配，保留 .1-.9 给网关和其他用途
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
	// Try next C segment
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

// GetUserTotalTraffic gets total traffic for a user across all sessions.
func GetUserTotalTraffic(userID int) (rx int64, tx int64, err error) {
	dbLock.RLock()
	defer dbLock.RUnlock()

	err = db.QueryRow(`SELECT COALESCE(SUM(rx_bytes),0), COALESCE(SUM(tx_bytes),0) 
		FROM connection_log WHERE user_id = ?`, userID).Scan(&rx, &tx)
	return
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
		var uname, intIP, extIP, connAt string
		var discAt sql.NullString
		var rx, tx int64
		if err := rows.Scan(&id, &uid, &uname, &intIP, &extIP, &connAt, &discAt, &rx, &tx); err != nil {
			continue
		}
		discTime := ""
		if discAt.Valid {
			discTime = discAt.String
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
		userID, username, internalIP, externalIP, time.Now().Format("2006-01-02 15:04:05"))
	return err
}

// UpdateConnectionOnDisconnect updates the disconnect time and traffic.
func UpdateConnectionOnDisconnect(userID int, rx, tx int64) error {
	dbLock.RLock()
	defer dbLock.RUnlock()

	_, err := db.Exec(`UPDATE connection_log SET 
		disconnected_at = ?, rx_bytes = ?, tx_bytes = ?
		WHERE user_id = ? AND disconnected_at IS NULL`,
		time.Now().Format("2006-01-02 15:04:05"), rx, tx, userID)
	return err
}
