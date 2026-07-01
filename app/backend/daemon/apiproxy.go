package daemon

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// socketPath 返回 Unix socket 路径
func socketPath(dataDir string) string {
	return filepath.Join(dataDir, "daemon.sock")
}

// StartAPIServer 在守护进程内启动本地 HTTP API 服务（Unix socket）
// 所有 DB 操作仅由此服务处理，CGI 通过这个 socket 转发请求
// handler 由 main 传入（避免 daemon import api 导致循环依赖）
func StartAPIServer(dataDir string, handler http.Handler) {
	sockPath := socketPath(dataDir)

	os.Remove(sockPath)

	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		log.Printf("Failed to start API server: %v", err)
		return
	}

	os.Chmod(sockPath, 0666)

	server := &http.Server{
		Handler:      handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	go func() {
		log.Printf("Internal API server started on %s", sockPath)
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Printf("API server error: %v", err)
		}
	}()
}

// StopAPIServer 清理 socket 文件
func StopAPIServer(dataDir string) {
	os.Remove(socketPath(dataDir))
}

// ProxyToDaemon 将 CGI 请求转发到守护进程的 Unix socket API 服务
// 连接失败时重试最多 5 次（给守护进程启动时间）
func ProxyToDaemon(dataDir, method, urlPath, queryString string, body io.Reader, contentType string) (int, http.Header, []byte, error) {
	sockPath := socketPath(dataDir)

	// 缓冲请求体（重试时需要多次读取）
	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = io.ReadAll(body)
		if err != nil {
			return 500, nil, nil, fmt.Errorf("read body: %w", err)
		}
	}

	// 必须使用完整的 http:// URL（含 scheme/host），否则自定义 Dial 不工作
	fullURL := "http://localhost" + urlPath
	if queryString != "" {
		fullURL += "?" + queryString
	}

	transport := &http.Transport{
		Dial: func(_, _ string) (net.Conn, error) {
			return net.DialTimeout("unix", sockPath, 5*time.Second)
		},
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   25 * time.Second,
	}

	var lastErr error
	for i := 0; i < 5; i++ {
		// 每次重试创建新 request（body 只能读一次）
		req, err := http.NewRequest(method, fullURL, bytes.NewReader(bodyBytes))
		if err != nil {
			return 500, nil, nil, fmt.Errorf("create request: %w", err)
		}
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}

		resp, err := client.Do(req)
		if err == nil {
			defer resp.Body.Close()
			respBody, readErr := io.ReadAll(resp.Body)
			if readErr != nil {
				return 502, nil, nil, fmt.Errorf("read response: %w", readErr)
			}
			return resp.StatusCode, resp.Header, respBody, nil
		}
		lastErr = err
		time.Sleep(200 * time.Millisecond)
	}
	return 502, nil, nil, fmt.Errorf("daemon request (after 5 retries): %w", lastErr)
}
