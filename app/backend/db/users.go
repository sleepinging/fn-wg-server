package db

import (
	"database/sql"
	"fmt"
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
	u.CreatedAt = toLocalTime(u.CreatedAt)
	u.UpdatedAt = toLocalTime(u.UpdatedAt)
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
	u.CreatedAt = toLocalTime(u.CreatedAt)
	u.UpdatedAt = toLocalTime(u.UpdatedAt)
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
		u.CreatedAt = toLocalTime(u.CreatedAt)
		u.UpdatedAt = toLocalTime(u.UpdatedAt)
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

	// 从 bandwidth_history 获取最新一条记录的累计流量
	err = db.QueryRow(`SELECT rx_bytes, tx_bytes FROM bandwidth_history 
		WHERE user_id = ? ORDER BY id DESC LIMIT 1`, userID).Scan(&rx, &tx)
	if err == sql.ErrNoRows {
		err = nil
		rx = 0
		tx = 0
	}
	return
}

// GetUserHistory returns empty history (connection tracking deprecated).
func GetUserHistory(userID int, page, pageSize int) ([]map[string]interface{}, int, error) {
	return []map[string]interface{}{}, 0, nil
}
