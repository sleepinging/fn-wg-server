package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

var (
	db     *sql.DB
	dbLock sync.RWMutex
	dbPath string
)

// Init opens the main database and creates tables.
func Init(dataDir string) error {
	dbLock.Lock()
	defer dbLock.Unlock()

	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}

	dbPath = filepath.Join(dataDir, "wg-server.db")
	var err error
	db, err = sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}

	if err := db.Ping(); err != nil {
		return fmt.Errorf("ping db: %w", err)
	}

	if err := createTables(); err != nil {
		return fmt.Errorf("create tables: %w", err)
	}

	return nil
}

func createTables() error {
	tables := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT UNIQUE NOT NULL,
			public_key TEXT UNIQUE NOT NULL,
			private_key TEXT NOT NULL,
			preshared_key TEXT DEFAULT '',
			allowed_ips TEXT NOT NULL,
			internal_ip TEXT UNIQUE NOT NULL,
			dns TEXT DEFAULT '',
			mtu INTEGER DEFAULT 1420,
			persistent_keepalive INTEGER DEFAULT 25,
			enabled INTEGER DEFAULT 1,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS bandwidth_history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER,
			rx_bytes INTEGER DEFAULT 0,
			tx_bytes INTEGER DEFAULT 0,
			rx_speed REAL DEFAULT 0,
			tx_speed REAL DEFAULT 0,
			timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS connection_log (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER,
			username TEXT,
			internal_ip TEXT,
			external_ip TEXT,
			connected_at DATETIME,
			disconnected_at DATETIME,
			rx_bytes INTEGER DEFAULT 0,
			tx_bytes INTEGER DEFAULT 0,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE SET NULL
		)`,
		`CREATE TABLE IF NOT EXISTS config (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS system_log (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			level TEXT DEFAULT 'INFO',
			message TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
	}

	for _, t := range tables {
		if _, err := db.Exec(t); err != nil {
			return err
		}
	}
	return nil
}

// GetDB returns the database handle.
func GetDB() *sql.DB {
	return db
}

// Close closes the database.
func Close() {
	if db != nil {
		db.Close()
	}
}

// ==================== Config ====================

// GetConfig retrieves a config value by key.
func GetConfig(key string) (string, error) {
	var value string
	err := db.QueryRow("SELECT value FROM config WHERE key = ?", key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

// SetConfig sets a config value.
func SetConfig(key, value string) error {
	_, err := db.Exec(`INSERT INTO config (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = ?`, key, value, value)
	return err
}

// GetAllConfig returns all config entries.
func GetAllConfig() (map[string]string, error) {
	rows, err := db.Query("SELECT key, value FROM config")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cfg := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		cfg[k] = v
	}
	return cfg, nil
}

// ==================== System Log ====================

// Log inserts a system log entry.
func Log(level, message string) {
	dbLock.RLock()
	defer dbLock.RUnlock()
	if db == nil {
		return
	}
	// Keep only last 10000 logs to prevent bloat
	db.Exec("INSERT INTO system_log (level, message) VALUES (?, ?)", level, message)
	db.Exec("DELETE FROM system_log WHERE id NOT IN (SELECT id FROM system_log ORDER BY id DESC LIMIT 10000)")
}

// GetLogs retrieves logs with pagination, level filter, and search.
func GetLogs(page, pageSize int, level, search string) ([]map[string]interface{}, int, error) {
	dbLock.RLock()
	defer dbLock.RUnlock()

	var total int
	countQuery := "SELECT COUNT(*) FROM system_log WHERE 1=1"
	countArgs := []interface{}{}
	if level != "" {
		countQuery += " AND level = ?"
		countArgs = append(countArgs, level)
	}
	if search != "" {
		countQuery += " AND message LIKE ?"
		countArgs = append(countArgs, "%"+search+"%")
	}
	if err := db.QueryRow(countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := "SELECT id, level, message, created_at FROM system_log WHERE 1=1"
	whereArgs := []interface{}{}
	if level != "" {
		query += " AND level = ?"
		whereArgs = append(whereArgs, level)
	}
	if search != "" {
		query += " AND message LIKE ?"
		whereArgs = append(whereArgs, "%"+search+"%")
	}
	query += " ORDER BY id DESC LIMIT ? OFFSET ?"
	whereArgs = append(whereArgs, pageSize, (page-1)*pageSize)

	rows, err := db.Query(query, whereArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	logs := make([]map[string]interface{}, 0)
	for rows.Next() {
		var id int
		var lvl, msg, ts string
		if err := rows.Scan(&id, &lvl, &msg, &ts); err != nil {
			continue
		}
		logs = append(logs, map[string]interface{}{
			"id":        id,
			"level":     lvl,
			"message":   msg,
			"createdAt": ts,
		})
	}
	return logs, total, nil
}

// CleanLogsByDays removes logs older than specified days.
func CleanLogsByDays(days int) error {
	cutoff := time.Now().AddDate(0, 0, -days)
	_, err := db.Exec("DELETE FROM system_log WHERE created_at < ?", cutoff.Format("2006-01-02 15:04:05"))
	return err
}

// GetLogSize returns the database file size in bytes.
func GetLogSize() (int64, error) {
	// Calculate total DB file sizes
	var size int64
	baseDir := filepath.Dir(dbPath)
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return 0, err
	}
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) != ".db-journal" {
			info, err := e.Info()
			if err == nil {
				size += info.Size()
			}
		}
	}
	return size, nil
}

// CleanBandwidthHistory removes bandwidth records older than days.
func CleanBandwidthHistory(days int) error {
	cutoff := time.Now().AddDate(0, 0, -days)
	_, err := db.Exec("DELETE FROM bandwidth_history WHERE timestamp < ?", cutoff.Format("2006-01-02 15:04:05"))
	return err
}
