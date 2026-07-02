package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
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

func init() {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err == nil {
		time.Local = loc
	}
}

const Version = "1.0.81"



func main() {
	// Determine data directory
	dataDir := os.Getenv("TRIM_PKGVAR")
	if dataDir == "" {
		home, _ := os.UserHomeDir()
		dataDir = filepath.Join(home, ".wg-server")
	}

	// Set version for API module
	api.Version = Version

	// Check for daemon mode
	if len(os.Args) > 1 && os.Args[1] == "daemon" {
		// 守护进程模式：初始化 DB，启动本地 API 服务
		if err := db.Init(dataDir); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to init db: %v\n", err)
			os.Exit(1)
		}
		defer db.Close()
		runDaemon(dataDir)
		return
	}

	// Check for command mode
	if len(os.Args) > 1 {
		handleCommand(os.Args[1])
		return
	}

	// Check if running as CGI
	if os.Getenv("GATEWAY_INTERFACE") != "" {
		// CGI 模式：不初始化 DB，直接转发请求给守护进程
		handleCGI()
		return
	}

	// Direct HTTP server mode (for development/testing)
	// 开发模式才初始化 DB
	if err := db.Init(dataDir); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to init db: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()
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

	pathInfo := os.Getenv("PATH_INFO")
	scriptName := os.Getenv("SCRIPT_NAME")
	queryString := os.Getenv("QUERY_STRING")

	// 从 PATH_INFO 提取 /api/... 路径
	urlPath := pathInfo
	if idx := strings.Index(urlPath, "/index.cgi/"); idx >= 0 {
		urlPath = urlPath[idx+len("/index.cgi"):]
	} else if idx := strings.Index(urlPath, "/index.cgi"); idx >= 0 {
		urlPath = urlPath[idx+len("/index.cgi"):]
		if urlPath == "" {
			urlPath = "/"
		}
	} else if urlPath == "" {
		urlPath = scriptName
		if urlPath == "" {
			urlPath = "/"
		}
	}

	// 根路径 / 返回前端 HTML 页面
	if urlPath == "/" && method == "GET" {
		serveUIHTML()
		return
	}

	// 读取请求体
	var bodyReader io.Reader
	contentLength := os.Getenv("CONTENT_LENGTH")
	if contentLength != "" && contentLength != "0" {
		bodyReader = os.Stdin
	}

	contentType := os.Getenv("CONTENT_TYPE")

	// 转发到守护进程的本地 API 服务
	dataDir := os.Getenv("TRIM_PKGVAR")
	if dataDir == "" {
		home, _ := os.UserHomeDir()
		dataDir = filepath.Join(home, ".wg-server")
	}

	statusCode, respHeaders, respBody, err := daemon.ProxyToDaemon(dataDir, method, urlPath, queryString, bodyReader, contentType)
	if err != nil {
		// 守护进程未运行，尝试自动启动
		log.Printf("ProxyToDaemon error: %v (url=%s) - trying to start daemon", err, urlPath)
		startDaemon(dataDir)
		// 等待 socket 就绪
		time.Sleep(2 * time.Second)
		// 重试一次
		statusCode, respHeaders, respBody, err = daemon.ProxyToDaemon(dataDir, method, urlPath, queryString, bodyReader, contentType)
		if err != nil {
			log.Printf("ProxyToDaemon retry failed: %v", err)
			writeCGIError(502, "daemon unavailable")
			return
		}
	}

	// 写状态行
	statusText := http.StatusText(statusCode)
	if statusText == "" {
		statusText = "Unknown"
	}
	fmt.Printf("Status: %d %s\r\n", statusCode, statusText)

	// 转发响应头
	for key, values := range respHeaders {
		for _, value := range values {
			fmt.Printf("%s: %s\r\n", key, value)
		}
	}

	// 空行分隔
	fmt.Print("\r\n")

	// 写响应体
	os.Stdout.Write(respBody)
}

func serveUIHTML() {
	// 查找 UI HTML 文件
	uiDir := os.Getenv("UI_DIR")
	if uiDir == "" {
		// 从可执行路径推断
		exe, _ := os.Executable()
		uiDir = filepath.Join(filepath.Dir(filepath.Dir(exe)), "ui")
	}
	htmlPath := filepath.Join(uiDir, "index.html")

	data, err := os.ReadFile(htmlPath)
	if err != nil {
		log.Printf("Failed to read UI HTML: %v", err)
		writeCGIError(500, "UI not found")
		return
	}

	fmt.Print("Status: 200 OK\r\n")
	fmt.Print("Content-Type: text/html; charset=utf-8\r\n")
	fmt.Print("Cache-Control: no-cache\r\n")
	fmt.Print("\r\n")
	os.Stdout.Write(data)
}

func writeCGIError(status int, msg string) {
	statusText := http.StatusText(status)
	fmt.Printf("Status: %d %s\r\n", status, statusText)
	fmt.Printf("Content-Type: application/json\r\n")
	fmt.Print("\r\n")
	fmt.Printf(`{"error":"%s"}`, msg)
}

func runDaemon(dataDir string) {
	// 文件锁互斥，防止同时运行多个守护进程
	lockFile, err := tryLock(dataDir)
	if err != nil {
		log.Printf("Cannot start daemon: %v", err)
		os.Exit(1)
	}
	defer lockFile.Close()

	interfaceName := loadInterfaceName()

	// 启动内部 API 服务（Unix socket），处理所有 DB 操作
	daemon.StartAPIServer(dataDir, api.NewRouter())

	mon := daemon.NewMonitor(interfaceName, dataDir)
	mon.Start()

	// Wait for signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	<-sigCh

	mon.Stop()
	daemon.StopAPIServer(dataDir)
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

// startDaemon 启动守护进程（从 CGI 调用时自动启动）
// 使用 Setpgid 脱离 CGI 进程组，防止 CGI 退出时守护进程被杀死
func startDaemon(dataDir string) {
	// 先检查守护进程是否已在运行
	if _, err := net.DialTimeout("unix", filepath.Join(dataDir, "daemon.sock"), 100*time.Millisecond); err == nil {
		return // 已在运行
	}

	exe, err := os.Executable()
	if err != nil {
		log.Printf("startDaemon: cannot get executable: %v", err)
		return
	}

	// 日志文件
	logFile := filepath.Join(dataDir, "info.log")
	f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("startDaemon: cannot open log: %v", err)
		return
	}

	cmd := exec.Command(exe, "daemon")
	cmd.Env = append(os.Environ(), "TRIM_PKGVAR="+dataDir)
	cmd.Stdout = f
	cmd.Stderr = f
	// 新建进程组，脱离 CGI 进程
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		f.Close()
		log.Printf("startDaemon: failed to start: %v", err)
		return
	}
	f.Close()

	log.Printf("startDaemon: daemon started (PID: %d)", cmd.Process.Pid)
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

// tryLock 获取文件锁，防止多个守护进程同时运行。
// Linux 上使用 flock，Windows 上降级为空操作。
func tryLock(dataDir string) (*os.File, error) {
	lockPath := filepath.Join(dataDir, "daemon.lock")
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, err
	}
	err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("daemon already running (pid in %s)", lockPath)
	}
	// 写入 PID
	f.Truncate(0)
	f.WriteString(fmt.Sprintf("%d", os.Getpid()))
	return f, nil
}

func loadInterfaceName() string {
	if cfg, err := db.GetAllConfig(); err == nil {
		if name, ok := cfg["interface_name"]; ok && name != "" {
			return name
		}
	}
	return wg.DefaultConfig().InterfaceName
}


