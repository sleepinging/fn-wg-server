package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"wg-server/api"
	"wg-server/db"
	"wg-server/daemon"
	"wg-server/wg"
)

const Version = "1.0.38"

func init() {
	// 统一使用 Asia/Shanghai 时区
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err == nil {
		time.Local = loc
	}
}

func main() {
	// Determine data directory
	dataDir := os.Getenv("TRIM_PKGVAR")
	if dataDir == "" {
		home, _ := os.UserHomeDir()
		dataDir = filepath.Join(home, ".wg-server")
	}

	// Set version for API module
	api.Version = Version

	// Initialize database
	if err := db.Init(dataDir); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to init db: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	// Check for daemon mode
	if len(os.Args) > 1 && os.Args[1] == "daemon" {
		runDaemon(dataDir)
		return
	}

	// Check for command mode
	if len(os.Args) > 1 {
		handleCommand(os.Args[1])
		return
	}

	// Set peers cache directory (shared between daemon and CGI)
	wg.SetPeersCacheDir(dataDir)

	// Check if running as CGI
	if os.Getenv("GATEWAY_INTERFACE") != "" {
		// CGI mode: manually handle the request
		handleCGI()
		return
	}

	// Direct HTTP server mode (for development/testing)
	port := "8080"
	if p := os.Getenv("PORT"); p != "" {
		port = p
	}
	router := api.NewRouter()

	fmt.Printf("wg-server v%s starting on :%s\n", Version, port)
	http.ListenAndServe(":"+port, router)
}

// handleCGI processes a CGI request manually.
func handleCGI() {
	method := os.Getenv("REQUEST_METHOD")
	if method == "" {
		method = "GET"
	}

	// The URL path: use PATH_INFO, fall back to SCRIPT_NAME
	pathInfo := os.Getenv("PATH_INFO")
	scriptName := os.Getenv("SCRIPT_NAME")
	queryString := os.Getenv("QUERY_STRING")

	// trim_http_cgi 把完整路径放进 PATH_INFO（例如 /cgi/.../index.cgi/api/stats/history）
	// 需要提取 index.cgi 后面的部分作为实际 URL 路径
	urlPath := pathInfo
	if idx := strings.Index(urlPath, "/index.cgi/"); idx >= 0 {
		urlPath = urlPath[idx+len("/index.cgi"):] // 保留 /api/stats/history 格式
	} else if idx := strings.Index(urlPath, "/index.cgi"); idx >= 0 {
		urlPath = urlPath[idx+len("/index.cgi"):]
		if urlPath == "" {
			urlPath = "/"
		}
	} else if urlPath == "" {
		// If empty, try SCRIPT_NAME
		urlPath = scriptName
		if urlPath == "" {
			urlPath = "/"
		}
	}

	// Build the full URL
	fullURL := urlPath
	if queryString != "" {
		fullURL = urlPath + "?" + queryString
	}

	// Read body for POST/PUT requests
	var bodyReader io.Reader
	contentLength := os.Getenv("CONTENT_LENGTH")
	if contentLength != "" && contentLength != "0" {
		bodyReader = os.Stdin
	}

	// Create a minimal HTTP request
	req, err := http.NewRequest(method, fullURL, bodyReader)
	if err != nil {
		writeCGIError(500, "Failed to create request: "+err.Error())
		return
	}

	// Set remote address
	req.RemoteAddr = os.Getenv("REMOTE_ADDR")
	if req.RemoteAddr == "" {
		req.RemoteAddr = "127.0.0.1"
	}

	// Set content type from environment
	if ct := os.Getenv("CONTENT_TYPE"); ct != "" {
		req.Header.Set("Content-Type", ct)
	}

	// Create a ResponseRecorder to capture the response
	w := &cgiResponseWriter{
		header: make(http.Header),
	}

	// Handle the request
	router := api.NewRouter()
	router.ServeHTTP(w, req)

	// Write CGI response to stdout
	w.flush()
}

// cgiResponseWriter implements http.ResponseWriter for CGI output.
type cgiResponseWriter struct {
	header     http.Header
	statusCode int
	body       strings.Builder
	wroteHeader bool
}

func (w *cgiResponseWriter) Header() http.Header {
	return w.header
}

func (w *cgiResponseWriter) WriteHeader(statusCode int) {
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true
	w.statusCode = statusCode
}

func (w *cgiResponseWriter) Write(data []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.body.Write(data)
}

func (w *cgiResponseWriter) flush() {
	// Write status
	statusText := http.StatusText(w.statusCode)
	if statusText == "" {
		statusText = "Unknown"
	}
	fmt.Printf("Status: %d %s\r\n", w.statusCode, statusText)

	// Write headers
	for key, values := range w.header {
		for _, value := range values {
			fmt.Printf("%s: %s\r\n", key, value)
		}
	}

	// Empty line to separate headers from body
	fmt.Print("\r\n")

	// Write body
	fmt.Print(w.body.String())
}

func writeCGIError(status int, msg string) {
	statusText := http.StatusText(status)
	fmt.Printf("Status: %d %s\r\n", status, statusText)
	fmt.Printf("Content-Type: application/json\r\n")
	fmt.Print("\r\n")
	fmt.Printf(`{"error":"%s"}`, msg)
}

func runDaemon(dataDir string) {
	interfaceName := loadInterfaceName()

	mon := daemon.NewMonitor(interfaceName, dataDir)
	mon.Start()

	// Wait for signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	<-sigCh

	mon.Stop()
}

func handleCommand(cmd string) {
	switch cmd {
	case "version":
		fmt.Println(Version)
	case "init-wg":
		// Initialize WireGuard interface
		if err := wg.InitInterface(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to init WireGuard: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("WireGuard initialized")
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", cmd)
		fmt.Println("Available commands: daemon, version, init-wg")
		os.Exit(1)
	}
}

// GetAppDest returns the application destination directory.
func GetAppDest() string {
	dest := os.Getenv("TRIM_APPDEST")
	if dest == "" {
		// Try to detect from executable path
		exe, _ := os.Executable()
		dest = filepath.Dir(filepath.Dir(exe))
	}
	return dest
}

// isWGRunning checks if the WireGuard interface is up.
func isWGRunning() bool {
	interfaceName := loadInterfaceName()
	data, err := os.ReadFile("/sys/class/net/" + interfaceName + "/carrier")
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(data)) == "1"
}

func loadInterfaceName() string {
	if cfg, err := db.GetAllConfig(); err == nil {
		if name, ok := cfg["interface_name"]; ok && name != "" {
			return name
		}
	}
	return wg.DefaultConfig().InterfaceName
}


