package api

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"wg-server/db"
	"wg-server/wg"
)

// NewRouter creates a new HTTP handler for all API routes.
func NewRouter() *http.ServeMux {
	mux := http.NewServeMux()

	// API routes
	mux.HandleFunc("/api/users", handleUsers)
	mux.HandleFunc("/api/users/", handleUserByID)
	mux.HandleFunc("/api/stats", handleStats)
	mux.HandleFunc("/api/stats/history", handleStatsHistory)
	mux.HandleFunc("/api/config", handleConfig)
	mux.HandleFunc("/api/config/backup", handleConfigBackup)
	mux.HandleFunc("/api/config/restore", handleConfigRestore)
	mux.HandleFunc("/api/config/export-all", handleExportAll)
	mux.HandleFunc("/api/service/", handleService)
	mux.HandleFunc("/api/system", handleSystem)
	mux.HandleFunc("/api/wg/kernel", handleWGKernel)
	mux.HandleFunc("/api/logs", handleLogs)
	mux.HandleFunc("/api/logs/clean", handleLogsClean)
	mux.HandleFunc("/api/ip/hint", handleIPHint)

	// DB stats
	mux.HandleFunc("/api/db/stats", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, db.GetDBStats())
	})

	// Health check
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "version": Version})
	})

	// Static files (for dev mode)
	mux.HandleFunc("/", handleStatic)

	return mux
}

// Version is set by main package.
var Version = "1.0.85"

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// mimeTypes maps file extensions to Content-Type values.
// Used instead of mime.TypeByExtension() because some systems (like fnOS)
// lack a mime database, causing .js files to be served as text/plain.
var mimeTypes = map[string]string{
	".html": "text/html; charset=utf-8",
	".js":   "text/javascript; charset=utf-8",
	".css":  "text/css; charset=utf-8",
	".json": "application/json; charset=utf-8",
	".png":  "image/png",
	".svg":  "image/svg+xml",
	".ico":  "image/x-icon",
	".woff": "font/woff",
	".woff2": "font/woff2",
	".map":  "application/json",
}

func handleStatic(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if path == "/" || path == "" {
		path = "/index.html"
	}

	// Try to serve from various possible locations
	uiDirs := []string{}
	if dest := os.Getenv("TRIM_APPDEST"); dest != "" {
		uiDirs = append(uiDirs, filepath.Join(dest, "ui"))
	}
	if dir := os.Getenv("UI_DIR"); dir != "" {
		uiDirs = append(uiDirs, dir)
	}
	uiDirs = append(uiDirs, "../ui", "./ui")

	serveFile := func(filePath string) {
		data, err := os.ReadFile(filePath)
		if err != nil {
			return
		}
		ext := strings.ToLower(filepath.Ext(filePath))
		if ct, ok := mimeTypes[ext]; ok {
			w.Header().Set("Content-Type", ct)
		} else {
			w.Header().Set("Content-Type", "application/octet-stream")
		}
		// 禁止浏览器缓存 HTML 页面，确保每次都加载最新版本
		// （因为 CGI 模式下无法做版本化的静态资源）
		if ext == ".html" {
			w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
			w.Header().Set("Pragma", "no-cache")
			w.Header().Set("Expires", "0")
		}
		w.WriteHeader(http.StatusOK)
		w.Write(data)
	}

	for _, uiDir := range uiDirs {
		filePath := filepath.Join(uiDir, path)
		if _, err := os.Stat(filePath); err == nil {
			serveFile(filePath)
			return
		}
	}

	// If not found and not an API route, serve index.html (SPA fallback)
	if !strings.HasPrefix(path, "/api/") {
		for _, uiDir := range uiDirs {
			indexPath := filepath.Join(uiDir, "index.html")
			if _, err := os.Stat(indexPath); err == nil {
				serveFile(indexPath)
				return
			}
		}
	}

	http.NotFound(w, r)
}

// ==================== Users ====================

func handleUsers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		listUsers(w, r)
	case http.MethodPost:
		createUser(w, r)
	case http.MethodOptions:
		w.WriteHeader(http.StatusOK)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func listUsers(w http.ResponseWriter, r *http.Request) {
	users, err := db.ListUsers()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Get peer info for online status
	interfaceName := getInterfaceName()
	peers, peerErr := wg.GetPeers(interfaceName)
	if peerErr != nil {
		peers = []wg.PeerInfo{}
	}
	peerMap := make(map[string]wg.PeerInfo)
	for _, p := range peers {
		peerMap[p.PublicKey] = p
	}

	// Hide private keys in list
	result := make([]map[string]interface{}, 0)
	for _, u := range users {
		item := map[string]interface{}{
			"id":                  u.ID,
			"username":            u.Username,
			"publicKey":           u.PublicKey,
			"allowedIPs":          u.AllowedIPs,
			"internalIP":          u.InternalIP,
			"dns":                 u.DNS,
			"mtu":                 u.MTU,
			"persistentKeepalive": u.PersistentKeepalive,
			"enabled":             u.Enabled,
			"createdAt":           u.CreatedAt,
			"updatedAt":           u.UpdatedAt,
		}

		// Add online status, traffic and speed from peer info
		if peer, ok := peerMap[u.PublicKey]; ok {
			item["online"] = peer.LatestHandshake > 0 || peer.TransferRx > 0 || peer.TransferTx > 0
			item["rxBytes"] = peer.TransferRx
			item["txBytes"] = peer.TransferTx
			item["endpoint"] = peer.Endpoint
			item["latestHandshake"] = peer.LatestHandshake
			if peer.LatestHandshake > 0 {
				item["onlineSince"] = db.FormatTime(peer.LatestHandshake * 1000)
			}
			// 实时带宽（从带宽历史记录取最新速度）
			if point, err := db.GetLatestBandwidth(u.ID); err == nil && point != nil {
				item["rxSpeed"] = point.RxSpeed
				item["txSpeed"] = point.TxSpeed
			} else {
				item["rxSpeed"] = float64(0)
				item["txSpeed"] = float64(0)
			}
		} else {
			item["online"] = false
			item["rxBytes"] = 0
			item["txBytes"] = 0
			item["rxSpeed"] = float64(0)
			item["txSpeed"] = float64(0)
		}

		result = append(result, item)
	}

	writeJSON(w, http.StatusOK, result)
}

func createUser(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Username            string `json:"username"`
		AllowedIPs          string `json:"allowedIPs"`
		InternalIP          string `json:"internalIP"`
		DNS                 string `json:"dns"`
		MTU                 int    `json:"mtu"`
		PersistentKeepalive int    `json:"persistentKeepalive"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	if input.Username == "" {
		writeError(w, http.StatusBadRequest, "username is required")
		return
	}
	if input.InternalIP == "" {
		writeError(w, http.StatusBadRequest, "internalIP is required")
		return
	}

	// Generate keys
	privateKey, publicKey, err := wg.GenerateKey()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate keys: "+err.Error())
		return
	}

	presharedKey, _ := wg.GeneratePresharedKey()

	if input.AllowedIPs == "" {
		input.AllowedIPs = input.InternalIP
	}
	if input.MTU <= 0 {
		input.MTU = 1420
	}
	if input.PersistentKeepalive <= 0 {
		input.PersistentKeepalive = 25
	}

	user := db.User{
		Username:            input.Username,
		PublicKey:           publicKey,
		PrivateKey:          privateKey,
		PresharedKey:        presharedKey,
		AllowedIPs:          input.AllowedIPs,
		InternalIP:          input.InternalIP,
		DNS:                 input.DNS,
		MTU:                 input.MTU,
		PersistentKeepalive: input.PersistentKeepalive,
		Enabled:             true,
	}

	id, err := db.CreateUser(user)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to create user: "+err.Error())
		return
	}

	user.ID = int(id)

	// Apply config to WireGuard
	if err := applyWGConfig(); err != nil {
		db.Log("WARN", "Failed to apply WireGuard config after user creation: "+err.Error())
	}

	db.Log("INFO", fmt.Sprintf("Created user: %s (IP: %s)", user.Username, user.InternalIP))

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":          user.ID,
		"username":    user.Username,
		"publicKey":   user.PublicKey,
		"privateKey":  user.PrivateKey,
		"internalIP":  user.InternalIP,
		"allowedIPs":  user.AllowedIPs,
		"presharedKey": user.PresharedKey,
	})
}

func handleUserByID(w http.ResponseWriter, r *http.Request) {
	// Extract user ID from path
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/users/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		writeError(w, http.StatusBadRequest, "user ID is required")
		return
	}

	var userID int
	fmt.Sscanf(parts[0], "%d", &userID)
	if userID == 0 {
		writeError(w, http.StatusBadRequest, "invalid user ID")
		return
	}

	// Check for sub-routes
	if len(parts) > 1 {
		switch parts[1] {
		case "stats":
			handleUserStats(w, r, userID)
			return
		case "history":
			handleUserHistory(w, r, userID)
			return
		case "traffic":
			handleUserTraffic(w, r, userID)
			return
		case "config":
			handleUserConfig(w, r, userID)
			return
		}
	}

	switch r.Method {
	case http.MethodGet:
		getUser(w, r, userID)
	case http.MethodPut:
		updateUser(w, r, userID)
	case http.MethodDelete:
		deleteUser(w, r, userID)
	case http.MethodOptions:
		w.WriteHeader(http.StatusOK)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func getUser(w http.ResponseWriter, r *http.Request, userID int) {
	user, err := db.GetUserByID(userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if user == nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}

	// Get peer info
	interfaceName := getInterfaceName()
	peers, peerErr := wg.GetPeers(interfaceName)
	if peerErr != nil {
		peers = []wg.PeerInfo{}
	}
	for _, p := range peers {
		if p.PublicKey == user.PublicKey {
			user.RxBytes = p.TransferRx
			user.TxBytes = p.TransferTx
			user.Online = p.LatestHandshake > 0
			user.ExternalIP = p.Endpoint
			if p.LatestHandshake > 0 {
				user.OnlineSince = db.FormatTime(p.LatestHandshake * 1000)
			}
			break
		}
	}

	writeJSON(w, http.StatusOK, user)
}

func updateUser(w http.ResponseWriter, r *http.Request, userID int) {
	var input struct {
		Username            string `json:"username"`
		PublicKey           string `json:"publicKey"`
		AllowedIPs          string `json:"allowedIPs"`
		InternalIP          string `json:"internalIP"`
		DNS                 string `json:"dns"`
		MTU                 int    `json:"mtu"`
		PersistentKeepalive int    `json:"persistentKeepalive"`
		Enabled             *bool  `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	existing, err := db.GetUserByID(userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if existing == nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}

	if input.Username != "" {
		existing.Username = input.Username
	}
	if input.PublicKey != "" {
		existing.PublicKey = input.PublicKey
	}
	if input.AllowedIPs != "" {
		existing.AllowedIPs = input.AllowedIPs
	}
	if input.InternalIP != "" {
		existing.InternalIP = input.InternalIP
	}
	if input.DNS != "" {
		existing.DNS = input.DNS
	}
	if input.MTU > 0 {
		existing.MTU = input.MTU
	}
	if input.PersistentKeepalive > 0 {
		existing.PersistentKeepalive = input.PersistentKeepalive
	}
	if input.Enabled != nil {
		existing.Enabled = *input.Enabled
	}

	if err := db.UpdateUser(*existing); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update user: "+err.Error())
		return
	}

	// Reapply WireGuard config
	if err := applyWGConfig(); err != nil {
		db.Log("WARN", "Failed to apply WireGuard config after user update: "+err.Error())
	}

	db.Log("INFO", fmt.Sprintf("Updated user: %s (ID: %d)", existing.Username, userID))

	writeJSON(w, http.StatusOK, map[string]string{"message": "user updated"})
}

func deleteUser(w http.ResponseWriter, r *http.Request, userID int) {
	user, err := db.GetUserByID(userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if user == nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}

	// 从数据库删除用户
	if err := db.DeleteUser(userID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete user: "+err.Error())
		return
	}

	db.Log("INFO", fmt.Sprintf("Deleted user: %s (ID: %d) - forced offline", user.Username, userID))

	// 通知守护进程重新应用配置（移除对等端）
	applyWGConfig()

	writeJSON(w, http.StatusOK, map[string]string{"message": "user deleted and forced offline"})
}

// ==================== User Stats ====================

func handleUserStats(w http.ResponseWriter, r *http.Request, userID int) {
	user, err := db.GetUserByID(userID)
	if err != nil || user == nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}

	interfaceName := getInterfaceName()
	peers, _ := wg.GetPeers(interfaceName)
	if peers == nil {
		peers = []wg.PeerInfo{}
	}

	stats := map[string]interface{}{
		"username":   user.Username,
		"internalIP": user.InternalIP,
		"online":     false,
		"rxBytes":    0,
		"txBytes":    0,
		"rxSpeed":    0,
		"txSpeed":    0,
		"endpoint":   "",
	}

	for _, p := range peers {
		if p.PublicKey == user.PublicKey {
			stats["online"] = p.LatestHandshake > 0 || p.TransferRx > 0 || p.TransferTx > 0
			stats["rxBytes"] = p.TransferRx
			stats["txBytes"] = p.TransferTx
			stats["endpoint"] = p.Endpoint
			if p.LatestHandshake > 0 {
				stats["onlineSince"] = db.FormatTime(p.LatestHandshake * 1000)
			}

			// Calculate speed from bandwidth history
			if point, err := db.GetLatestBandwidth(userID); err == nil && point != nil {
				stats["rxSpeed"] = point.RxSpeed
				stats["txSpeed"] = point.TxSpeed
			}

			// 本次会话流量（从活跃连接记录获取）
			stats["sessionRxBytes"], stats["sessionTxBytes"] = db.GetActiveSessionTraffic(userID, p.TransferRx, p.TransferTx)
			break
		}
	}

	writeJSON(w, http.StatusOK, stats)
}

func handleUserHistory(w http.ResponseWriter, r *http.Request, userID int) {
	page := 1
	pageSize := 20
	r.ParseForm()
	fmt.Sscanf(r.FormValue("page"), "%d", &page)
	fmt.Sscanf(r.FormValue("pageSize"), "%d", &pageSize)

	history, total, err := db.GetUserHistory(userID, page, pageSize)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":  history,
		"total": total,
		"page":  page,
	})
}

func handleUserTraffic(w http.ResponseWriter, r *http.Request, userID int) {
	rx, tx, err := db.GetUserTotalTraffic(userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	aggregate := r.URL.Query().Get("aggregate")
	maxPoints := 100

	var startTs, endTs int64
	since := r.URL.Query().Get("since")
	if since != "" {
		fmt.Sscanf(since, "%d", &startTs)
		if startTs > 0 {
			count, _ := db.CountBandwidthAfter(userID, startTs)
			maxPoints = int(count)
			if maxPoints > 100 {
				maxPoints = 100
			}
		}
	} else if s := r.URL.Query().Get("start"); s != "" {
		fmt.Sscanf(s, "%d", &startTs)
	}
	if e := r.URL.Query().Get("end"); e != "" {
		fmt.Sscanf(e, "%d", &endTs)
	}

	if aggregate == "" && maxPoints > 0 {
		aggregate = "max"
	}
	points, ptsErr := db.GetBandwidthHistoryAgg(userID, startTs, endTs, maxPoints, aggregate)
	if ptsErr != nil {
		writeError(w, http.StatusInternalServerError, ptsErr.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"totalRx": rx,
		"totalTx": tx,
		"chart":   points,
	})
}

// ==================== Global Stats ====================

func handleStats(w http.ResponseWriter, r *http.Request) {
	interfaceName := getInterfaceName()

	// Get total transfer
	rxBytes, txBytes, err := wg.GetInterfaceTransfer(interfaceName)
	if err != nil {
		rxBytes = 0
		txBytes = 0
	}

	// Get peers for online count
	peers, peerErr := wg.GetPeers(interfaceName)
	if peerErr != nil {
		peers = []wg.PeerInfo{}
	}
	onlineCount := 0
	for _, p := range peers {
		if p.LatestHandshake > 0 {
			onlineCount++
		}
	}

	// Get speed from latest bandwidth point
	var rxSpeed, txSpeed float64
	if point, err := db.GetLatestBandwidth(0); err == nil && point != nil {
		rxSpeed = point.RxSpeed
		txSpeed = point.TxSpeed
	}

	// Get external IP
	externalIP := getExternalIP()

	// Get WireGuard interface IP
	internalIP := ""
	if cfg, err := wg.LoadConfig(); err == nil {
		internalIP = cfg.Address
	}

	// Get service uptime
	uptime := getServiceUptime()

	// 总用户数从数据库取（而非 WireGuard 内核对等端数）
	allUsers, _ := db.ListUsers()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"rxBytes":    rxBytes,
		"txBytes":    txBytes,
		"rxSpeed":    rxSpeed,
		"txSpeed":    txSpeed,
		"onlineCount": onlineCount,
		"totalPeers":  len(allUsers),
		"externalIP": externalIP,
		"internalIP": internalIP,
		"uptime":     uptime,
	})
}

func handleStatsHistory(w http.ResponseWriter, r *http.Request) {
	userID := 0
	if uid := r.URL.Query().Get("userId"); uid != "" {
		fmt.Sscanf(uid, "%d", &userID)
	}

	aggregate := r.URL.Query().Get("aggregate")
	maxPoints := 100

	var startTs, endTs int64
	since := r.URL.Query().Get("since")
	if since != "" {
		fmt.Sscanf(since, "%d", &startTs)
		if startTs > 0 {
			count, _ := db.CountBandwidthAfter(userID, startTs)
			maxPoints = int(count)
			if maxPoints > 100 {
				maxPoints = 100
			}
		}
	} else if s := r.URL.Query().Get("start"); s != "" {
		fmt.Sscanf(s, "%d", &startTs)
	}
	if e := r.URL.Query().Get("end"); e != "" {
		fmt.Sscanf(e, "%d", &endTs)
	}

	if startTs == 0 {
		startTs = time.Now().Add(-1 * time.Hour).UnixMilli()
	}
	if endTs == 0 {
		endTs = time.Now().UnixMilli()
	}

	if aggregate == "" && maxPoints > 0 {
		aggregate = "max"
	}
	points, err := db.GetBandwidthHistoryAgg(userID, startTs, endTs, maxPoints, aggregate)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, points)
}

// ==================== Config ====================

func handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		getConfig(w, r)
	case http.MethodPut:
		updateConfig(w, r)
	case http.MethodOptions:
		w.WriteHeader(http.StatusOK)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func getConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := wg.LoadConfig()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	all, _ := db.GetAllConfig()
	historyDays := "7"
	if v, ok := all["history_retention_days"]; ok {
		historyDays = v
	}
	autoStart := "false"
	if v, ok := all["auto_start"]; ok {
		autoStart = v
	}
	flushInterval := "10"
	if v, ok := all["bandwidth_flush_interval"]; ok {
		flushInterval = v
	}

	currentIP := getExternalIP()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"wireguard":              cfg,
		"historyRetentionDays":   historyDays,
		"autoStart":              autoStart,
		"bandwidthFlushInterval": flushInterval,
		"detectedIP":             currentIP,
	})
}

func updateConfig(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Wireguard              *wg.WGConfig `json:"wireguard"`
		HistoryRetentionDays   string       `json:"historyRetentionDays"`
		AutoStart              string       `json:"autoStart"`
		BandwidthFlushInterval string       `json:"bandwidthFlushInterval"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	if input.Wireguard != nil {
		if err := wg.SaveConfig(*input.Wireguard); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to save config: "+err.Error())
			return
		}
	}

	if input.HistoryRetentionDays != "" {
		db.SetConfig("history_retention_days", input.HistoryRetentionDays)
	}
	if input.AutoStart != "" {
		db.SetConfig("auto_start", input.AutoStart)
		setAutoStart(input.AutoStart == "true")
	}
	if input.BandwidthFlushInterval != "" {
		db.SetConfig("bandwidth_flush_interval", input.BandwidthFlushInterval)
	}

	// Reapply WireGuard config
	if err := applyWGConfig(); err != nil {
		db.Log("WARN", "Failed to apply WireGuard config after update: "+err.Error())
	}

	db.Log("INFO", "Configuration updated")

	writeJSON(w, http.StatusOK, map[string]string{"message": "config updated"})
}

// ==================== Config Backup/Restore ====================

func handleConfigBackup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	all, _ := db.GetAllConfig()
	users, _ := db.ListUsers()

	backup := map[string]interface{}{
		"version":   Version,
		"timestamp": time.Now().Format(time.RFC3339),
		"config":    all,
		"users":     users,
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", "attachment; filename=wg-server-backup.json")
	json.NewEncoder(w).Encode(backup)

	db.Log("INFO", "Configuration backup exported")
}

func handleConfigRestore(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var backup struct {
		Version   string            `json:"version"`
		Timestamp string            `json:"timestamp"`
		Config    map[string]string `json:"config"`
		Users    []db.User         `json:"users"`
	}
	if err := json.NewDecoder(r.Body).Decode(&backup); err != nil {
		writeError(w, http.StatusBadRequest, "invalid backup file")
		return
	}

	// Restore config
	for k, v := range backup.Config {
		db.SetConfig(k, v)
	}

	// Note: User restoration is complex due to key generation. In practice,
	// the backup contains the full user data including keys.
	for _, u := range backup.Users {
		existing, _ := db.GetUserByUsername(u.Username)
		if existing != nil {
			u.ID = existing.ID
			db.UpdateUser(u)
		} else {
			db.CreateUser(u)
		}
	}

	// Reapply config
	applyWGConfig()

	db.Log("INFO", "Configuration backup restored")
	writeJSON(w, http.StatusOK, map[string]string{"message": "backup restored"})
}

func handleExportAll(w http.ResponseWriter, r *http.Request) {
	all, _ := db.GetAllConfig()
	users, _ := db.ListUsers()

	// 为每个用户获取带宽历史（最近 100 个点）和连接历史
	userDetails := make([]map[string]interface{}, 0)
	for _, u := range users {
		history, _, _ := db.GetUserHistory(u.ID, 1, 100)
		bandwidth, _ := db.GetBandwidthHistoryAgg(u.ID, 0, 0, 100, "max")
		detail := map[string]interface{}{
			"id":              u.ID,
			"username":        u.Username,
			"publicKey":       u.PublicKey,
			"allowedIPs":      u.AllowedIPs,
			"internalIP":      u.InternalIP,
			"dns":             u.DNS,
			"mtu":             u.MTU,
			"persistentKeepalive": u.PersistentKeepalive,
			"enabled":         u.Enabled,
			"createdAt":       u.CreatedAt,
			"updatedAt":       u.UpdatedAt,
			"connectionHistory": history,
			"bandwidthSamples":   bandwidth,
		}
		userDetails = append(userDetails, detail)
	}

	export := map[string]interface{}{
		"version":   Version,
		"timestamp": time.Now().Format(time.RFC3339),
		"config":    all,
		"users":     userDetails,
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", "attachment; filename=wg-server-full-export.json")
	json.NewEncoder(w).Encode(export)

	db.Log("INFO", "Full export downloaded")
}

// ==================== Service ====================

func handleService(w http.ResponseWriter, r *http.Request) {
	action := strings.TrimPrefix(r.URL.Path, "/api/service/")
	if action == "" {
		action = "status"
	}
	action = strings.Split(action, "/")[0]

	switch action {
	case "status":
		serviceStatus(w, r)
	case "start":
		serviceStart(w, r)
	case "stop":
		serviceStop(w, r)
	case "restart":
		serviceStop(w, r)
		serviceStart(w, r)
	default:
		writeError(w, http.StatusBadRequest, "unknown action: "+action)
	}
}

func serviceStatus(w http.ResponseWriter, r *http.Request) {
	interfaceName := getInterfaceName()
	wgRunning := wg.IsInterfaceUp(interfaceName)

	// Check if monitoring daemon is running
	monRunning := isMonitorRunning()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"wgRunning":    wgRunning,
		"monitorRunning": monRunning,
		"interfaceName": interfaceName,
	})
}

func serviceStart(w http.ResponseWriter, r *http.Request) {
	// 直接初始化 WireGuard 接口（daemon 有 root 权限）
	exec.Command("sysctl", "-w", "net.ipv4.ip_forward=1").Run()
	if err := wg.InitInterface(); err != nil {
		writeError(w, http.StatusInternalServerError, "init interface: "+err.Error())
		return
	}

	db.Log("INFO", "WireGuard service started")
	writeJSON(w, http.StatusOK, map[string]string{"message": "service started"})
}

func serviceStop(w http.ResponseWriter, r *http.Request) {
	// 直接关闭 WireGuard 接口（daemon 有 root 权限）
	interfaceName := getInterfaceName()
	exec.Command("ip", "link", "set", "dev", interfaceName, "down").Run()
	exec.Command("ip", "link", "delete", "dev", interfaceName).Run()

	db.Log("INFO", "WireGuard service stopped")
	writeJSON(w, http.StatusOK, map[string]string{"message": "service stopped"})
}

// ==================== System ====================

func handleSystem(w http.ResponseWriter, r *http.Request) {
	info := getSystemInfo()
	info["version"] = Version
	writeJSON(w, http.StatusOK, info)
}

func handleWGKernel(w http.ResponseWriter, r *http.Request) {
	loaded := wg.IsKernelModuleLoaded()
	kernelVersion := wg.GetKernelVersion()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"moduleLoaded":  loaded,
		"kernelVersion": kernelVersion,
	})
}

// ==================== Logs ====================

func handleLogs(w http.ResponseWriter, r *http.Request) {
	page := 1
	pageSize := 50
	r.ParseForm()
	fmt.Sscanf(r.FormValue("page"), "%d", &page)
	fmt.Sscanf(r.FormValue("pageSize"), "%d", &pageSize)
	level := r.FormValue("level")
	search := r.FormValue("search")

	logs, total, err := db.GetLogs(page, pageSize, level, search)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	size, _ := db.GetLogSize()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":  logs,
		"total": total,
		"page":  page,
		"size":  size,
	})
}

func handleLogsClean(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var input struct {
		Days int `json:"days"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil || input.Days <= 0 {
		input.Days = 7
	}

	if err := db.CleanLogsByDays(input.Days); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": fmt.Sprintf("cleaned logs older than %d days", input.Days)})
}

// ==================== IP Hint ====================

func handleIPHint(w http.ResponseWriter, r *http.Request) {
	cfg, err := wg.LoadConfig()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	ip, err := db.GetSmallestUnusedIP(cfg.Address)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"ip": ip})
}

// ==================== Helpers ====================

func getInterfaceName() string {
	if cfg, err := wg.LoadConfig(); err == nil {
		return cfg.InterfaceName
	}
	cfg := wg.DefaultConfig()
	return cfg.InterfaceName
}

func getExternalIP() string {
	// Try to get external IP from various sources
	data, err := os.ReadFile("/proc/1/cgroup")
	if err == nil && strings.Contains(string(data), "docker") {
		return "Container (NAT)"
	}

	// Try common methods
	cmd := execCommand("curl", "-s", "--connect-timeout", "3", "https://api.ip.sb/ip")
	if cmd != "" {
		return strings.TrimSpace(cmd)
	}
	cmd = execCommand("curl", "-s", "--connect-timeout", "3", "https://ipinfo.io/ip")
	if cmd != "" {
		return strings.TrimSpace(cmd)
	}

	return "Unknown"
}

func getServiceUptime() string {
	// Read system uptime
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return "Unknown"
	}
	var uptime float64
	fmt.Sscanf(string(data), "%f", &uptime)
	uptimeInt := int(uptime)
	days := uptimeInt / 86400
	hours := (uptimeInt / 3600) % 24
	minutes := (uptimeInt / 60) % 60
	return fmt.Sprintf("%dd %dh %dm", days, hours, minutes)
}

func getSystemInfo() map[string]interface{} {
	info := make(map[string]interface{})

	// CPU info
	if data, err := os.ReadFile("/proc/stat"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "cpu ") {
				fields := strings.Fields(line)
				if len(fields) >= 5 {
					user, _ := parseInt(fields[1])
					nice, _ := parseInt(fields[2])
					system, _ := parseInt(fields[3])
					idle, _ := parseInt(fields[4])
					total := user + nice + system + idle
					if total > 0 {
						info["cpuUsage"] = fmt.Sprintf("%.1f%%", float64(total-idle)/float64(total)*100)
					}
				}
				break
			}
		}
	}

	// Memory info
	if data, err := os.ReadFile("/proc/meminfo"); err == nil {
		var memTotal, memAvail int64
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "MemTotal:") {
				fmt.Sscanf(line, "MemTotal: %d kB", &memTotal)
			}
			if strings.HasPrefix(line, "MemAvailable:") {
				fmt.Sscanf(line, "MemAvailable: %d kB", &memAvail)
			}
		}
		if memTotal > 0 {
			usedPercent := float64(memTotal-memAvail) / float64(memTotal) * 100
			info["memory"] = map[string]interface{}{
				"total":     memTotal * 1024,
				"available": memAvail * 1024,
				"usedPercent": fmt.Sprintf("%.1f%%", usedPercent),
			}
		}
	}

	// Service uptime
	info["uptime"] = getServiceUptime()

	// Process info
	info["processMemory"] = getProcessMemory()

	// WireGuard 接口状态
	interfaceName := getInterfaceName()
	info["wgRunning"] = wg.IsInterfaceUp(interfaceName)
	info["monitorRunning"] = isMonitorRunning()

	return info
}

func getProcessMemory() string {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", os.Getpid()))
	if err != nil {
		return "Unknown"
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "VmRSS:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "VmRSS:"))
		}
	}
	return "Unknown"
}

func parseInt(s string) (int64, error) {
	var v int64
	_, err := fmt.Sscanf(s, "%d", &v)
	return v, err
}

func execCommand(name string, args ...string) string {
	cmd := exec.Command(name, args...)
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func applyWGConfig() error {
	// 所有 API 请求现在都经过 daemon Unix socket（root 权限），可以直接调用 wgctrl
	cfg, err := wg.LoadConfig()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	users, err := db.ListUsers()
	if err != nil {
		return fmt.Errorf("list users: %w", err)
	}
	return wg.ApplyConfig(*cfg, users)
}

func isMonitorRunning() bool {
	// 直接检查 daemon socket 是否可达（不走文件）
	dataDir := os.Getenv("TRIM_PKGVAR")
	if dataDir == "" {
		return false
	}
	sockPath := filepath.Join(dataDir, "daemon.sock")
	conn, err := net.DialTimeout("unix", sockPath, 100*time.Millisecond)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func setAutoStart(enabled bool) {
	serviceName := "wg-server.service"
	if enabled {
		exec.Command("systemctl", "enable", serviceName).Run()
	} else {
		exec.Command("systemctl", "disable", serviceName).Run()
	}
}

func handleUserConfig(w http.ResponseWriter, r *http.Request, userID int) {
	user, err := db.GetUserByID(userID)
	if err != nil || user == nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}

	cfg, err := wg.LoadConfig()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "load config: "+err.Error())
		return
	}

	// 获取服务端公网地址：优先使用配置的域名，其次自动检测
	serverDomain := cfg.ServerDomain
	if serverDomain == "" {
		serverDomain = getExternalIP()
	}
	if serverDomain == "" || serverDomain == "Unknown" {
		serverDomain = "YOUR_SERVER_IP"
	}
	serverEndpoint := fmt.Sprintf("%s:%d", serverDomain, cfg.ListenPort)

	// 准备 DNS
	dns := cfg.DNS
	if dns == "" {
		dns = "114.114.114.114, 8.8.8.8"
	}

	// 提取用户 IP（去掉 /32 后缀）
	clientIP := strings.TrimSuffix(user.InternalIP, "/32")
	if clientIP == "" {
		clientIP = user.InternalIP
	}

	// 计算 AllowedIPs：默认为配置的 VPN 网段（例如 192.168.5.0/24），而非全路由
	allowedIPs := computeAllowedIPs(cfg.Address)

	// 构建配置文件文本
	var config string
	pskLine := ""
	if user.PresharedKey != "" {
		pskLine = fmt.Sprintf("PresharedKey = %s\n", user.PresharedKey)
	}
	config = fmt.Sprintf(`[Interface]
PrivateKey = %s
Address = %s
DNS = %s
MTU = %d

[Peer]
PublicKey = %s
%sEndpoint = %s
AllowedIPs = %s
PersistentKeepalive = %d
`,
		user.PrivateKey,
		clientIP,
		dns,
		user.MTU,
		cfg.PublicKey,
		pskLine,
		serverEndpoint,
		allowedIPs,
		user.PersistentKeepalive,
	)

	// 支持 format 参数：conf / json
	format := r.URL.Query().Get("format")
	if format == "json" {
		// JSON 格式供前端展示
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"config":           config,
			"clientPrivateKey": user.PrivateKey,
			"clientAddress":    clientIP,
			"clientDNS":        dns,
			"clientMTU":        user.MTU,
			"serverPublicKey":  cfg.PublicKey,
			"serverEndpoint":   serverEndpoint,
			"persistentKeepalive": user.PersistentKeepalive,
			"presharedKey":     user.PresharedKey,
			"filename":         fmt.Sprintf("wg-client-%s.conf", user.Username),
		})
		return
	}

	// conf 格式直接下载文件
	filename := fmt.Sprintf("wg-client-%s.conf", user.Username)
	w.Header().Set("Content-Type", "application/wireguard-config")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(config))
}

// computeAllowedIPs 从服务端地址计算客户端 AllowedIPs
// 例如 "192.168.5.1/24" → "192.168.5.0/24"
func computeAllowedIPs(addr string) string {
	if addr == "" {
		return "192.168.5.0/24"
	}
	// 移除单个 IP/32 的情况
	addr = strings.TrimSuffix(addr, "/32")
	_, ipNet, err := net.ParseCIDR(addr)
	if err != nil {
		// 尝试只取 IP，自动推断 /24
		ip := net.ParseIP(addr)
		if ip == nil {
			return "192.168.5.0/24"
		}
		ip = ip.To4()
		if ip == nil {
			return addr + "/24"
		}
		return fmt.Sprintf("%d.%d.%d.0/24", ip[0], ip[1], ip[2])
	}
	return ipNet.String()
}


