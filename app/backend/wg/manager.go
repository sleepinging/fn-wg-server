package wg

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/crypto/curve25519"
	"golang.zx2c4.com/wireguard/wgctrl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"

	"wg-server/db"
)

// WGConfig holds the WireGuard interface configuration.
type WGConfig struct {
	InterfaceName string `json:"interfaceName"`
	PrivateKey    string `json:"privateKey"`
	PublicKey     string `json:"publicKey"`
	Address       string `json:"address"`
	ListenPort    int    `json:"listenPort"`
	DNS           string `json:"dns"`
	MTU           int    `json:"mtu"`
	PostUp        string `json:"postUp"`
	PostDown      string `json:"postDown"`
	ServerDomain  string `json:"serverDomain"`
}

// DefaultConfig returns the default WireGuard configuration.
func DefaultConfig() WGConfig {
	wanIface := detectWANInterface()
	iface := detectDefaultInterfaceName()
	return WGConfig{
		InterfaceName: iface,
		Address:       "192.168.5.1/24",
		ListenPort:    51820,
		MTU:           1420,
		PostUp:        fmt.Sprintf("iptables -A FORWARD -i %s -j ACCEPT; iptables -t nat -A POSTROUTING -o %s -j MASQUERADE", iface, wanIface),
		PostDown:      fmt.Sprintf("iptables -D FORWARD -i %s -j ACCEPT; iptables -t nat -D POSTROUTING -o %s -j MASQUERADE", iface, wanIface),
	}
}

func detectWANInterface() string {
	data, err := os.ReadFile("/proc/net/route")
	if err != nil {
		return "eth0"
	}
	lines := strings.Split(string(data), "\n")
	for _, line := range lines[1:] {
		fields := strings.Fields(line)
		if len(fields) >= 3 && fields[1] == "00000000" {
			return fields[0]
		}
	}
	return "eth0"
}

func detectDefaultInterfaceName() string {
	// 找第一个可用的 wg 接口名
	for i := 0; i < 10; i++ {
		name := fmt.Sprintf("wg%d", i)
		if _, err := net.InterfaceByName(name); err != nil {
			return name
		}
	}
	return "wg0"
}

// ==================== 密钥生成 ====================

func GenerateKey() (privateKey, publicKey string, err error) {
	var priv [32]byte
	if _, err := rand.Read(priv[:]); err != nil {
		return "", "", fmt.Errorf("generate private key: %w", err)
	}
	priv[0] &= 248
	priv[31] &= 127
	priv[31] |= 64
	var pub [32]byte
	curve25519.ScalarBaseMult(&pub, &priv)
	privateKey = base64.StdEncoding.EncodeToString(priv[:])
	publicKey = base64.StdEncoding.EncodeToString(pub[:])
	return privateKey, publicKey, nil
}

func GeneratePresharedKey() (string, error) {
	var key [32]byte
	if _, err := rand.Read(key[:]); err != nil {
		return "", fmt.Errorf("generate preshared key: %w", err)
	}
	return base64.StdEncoding.EncodeToString(key[:]), nil
}

// ==================== 配置读写 ====================

func LoadConfig() (*WGConfig, error) {
	cfg := DefaultConfig()
	all, err := db.GetAllConfig()
	if err != nil {
		return nil, err
	}
	if v, ok := all["wg_private_key"]; ok {
		cfg.PrivateKey = v
	}
	if v, ok := all["wg_public_key"]; ok {
		cfg.PublicKey = v
	}

	// 如果数据库中没有密钥对，自动生成并保存
	if cfg.PrivateKey == "" || cfg.PublicKey == "" {
		priv, pub, err := GenerateKey()
		if err != nil {
			return nil, fmt.Errorf("auto-generate key: %w", err)
		}
		cfg.PrivateKey = priv
		cfg.PublicKey = pub
		// 保存到数据库
		db.SetConfig("wg_private_key", priv)
		db.SetConfig("wg_public_key", pub)
	}
	if v, ok := all["wg_address"]; ok {
		cfg.Address = v
	}
	if v, ok := all["wg_listen_port"]; ok {
		fmt.Sscanf(v, "%d", &cfg.ListenPort)
	}
	if v, ok := all["wg_dns"]; ok {
		cfg.DNS = v
	}
	if v, ok := all["wg_mtu"]; ok {
		fmt.Sscanf(v, "%d", &cfg.MTU)
	}
	if v, ok := all["wg_post_up"]; ok {
		cfg.PostUp = v
	}
	if v, ok := all["wg_post_down"]; ok {
		cfg.PostDown = v
	}
	if v, ok := all["interface_name"]; ok {
		cfg.InterfaceName = v
	}
	if v, ok := all["server_domain"]; ok {
		cfg.ServerDomain = v
	}
	return &cfg, nil
}

func SaveConfig(cfg WGConfig) error {
	pairs := map[string]string{
		"wg_private_key": cfg.PrivateKey,
		"wg_public_key":  cfg.PublicKey,
		"wg_address":     cfg.Address,
		"wg_listen_port": fmt.Sprintf("%d", cfg.ListenPort),
		"wg_dns":         cfg.DNS,
		"wg_mtu":         fmt.Sprintf("%d", cfg.MTU),
		"wg_post_up":     cfg.PostUp,
		"wg_post_down":   cfg.PostDown,
		"interface_name": cfg.InterfaceName,
		"server_domain":  cfg.ServerDomain,
	}
	for k, v := range pairs {
		if err := db.SetConfig(k, v); err != nil {
			return err
		}
	}
	return nil
}

// ==================== WireGuard 控制（使用 wgctrl 库，不依赖系统 wg 命令）====================

// newClient 创建 wgctrl 客户端
func newClient() (*wgctrl.Client, error) {
	return wgctrl.New()
}

// ApplyConfig 通过 wgctrl 库将配置应用到 WireGuard 接口
func ApplyConfig(cfg WGConfig, users []db.User) error {
	client, err := newClient()
	if err != nil {
		return fmt.Errorf("wgctrl new: %w", err)
	}
	defer client.Close()

	var peers []wgtypes.PeerConfig
	for _, u := range users {
		if !u.Enabled {
			continue
		}
		pubKey, _ := wgtypes.ParseKey(u.PublicKey)
		peerCfg := wgtypes.PeerConfig{
			PublicKey: pubKey,
		}
		if u.PresharedKey != "" {
			psk, _ := wgtypes.ParseKey(u.PresharedKey)
			peerCfg.PresharedKey = &psk
		}
		if u.AllowedIPs != "" {
			for _, cidr := range strings.Split(u.AllowedIPs, ",") {
				cidr = strings.TrimSpace(cidr)
				_, ipNet, err := net.ParseCIDR(cidr)
				if err == nil {
					peerCfg.AllowedIPs = append(peerCfg.AllowedIPs, *ipNet)
				}
			}
		}
		peers = append(peers, peerCfg)
	}

	privKey, _ := wgtypes.ParseKey(cfg.PrivateKey)
	listenPort := cfg.ListenPort
	config := wgtypes.Config{
		PrivateKey: &privKey,
		ListenPort: &listenPort,
		Peers:      peers,
		ReplacePeers: true,
	}

	return client.ConfigureDevice(cfg.InterfaceName, config)
}

// InitInterface 初始化 WireGuard 接口
func InitInterface() error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}

	iface := cfg.InterfaceName
	_, err = net.InterfaceByName(iface)
	if err == nil {
		// 接口已存在，直接应用配置
		users, err := db.ListUsers()
		if err != nil {
			return err
		}
		return ApplyConfig(*cfg, users)
	}

	// 创建接口
	cmds := []string{
		fmt.Sprintf("ip link add dev %s type wireguard", iface),
		fmt.Sprintf("ip address add dev %s %s", iface, cfg.Address),
		fmt.Sprintf("ip link set dev %s up", iface),
	}
	if cfg.MTU > 0 {
		cmds = append(cmds, fmt.Sprintf("ip link set dev %s mtu %d", iface, cfg.MTU))
	}

	for _, c := range cmds {
		parts := strings.Split(c, " ")
		cmd := exec.Command(parts[0], parts[1:]...)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("%s: %s: %w", c, string(output), err)
		}
	}

	// 应用 WireGuard 配置
	users, err := db.ListUsers()
	if err != nil {
		return err
	}
	return ApplyConfig(*cfg, users)
}

// RemovePeer 从 WireGuard 接口移除对等端
func RemovePeer(interfaceName, publicKey string) error {
	client, err := newClient()
	if err != nil {
		return fmt.Errorf("wgctrl new: %w", err)
	}
	defer client.Close()

	pubKey, err := wgtypes.ParseKey(publicKey)
	if err != nil {
		return fmt.Errorf("parse key: %w", err)
	}

	config := wgtypes.Config{
		Peers: []wgtypes.PeerConfig{{
			PublicKey: pubKey,
			Remove:    true,
		}},
	}
	return client.ConfigureDevice(interfaceName, config)
}

// GetPeers 获取当前 WireGuard 接口的所有对等端信息
// CGI 进程无权限，从守护进程写入的缓存文件读取
func GetPeers(interfaceName string) ([]PeerInfo, error) {
	return GetPeersFromCache(interfaceName)
}

// GetPeersFromWgctl 通过 wgctrl 库直接读取（需要 root/CAP_NET_ADMIN）
func GetPeersFromWgctl(interfaceName string) ([]PeerInfo, error) {
	client, err := newClient()
	if err != nil {
		return nil, fmt.Errorf("wgctrl new: %w", err)
	}
	defer client.Close()

	device, err := client.Device(interfaceName)
	if err != nil {
		return nil, fmt.Errorf("get device: %w", err)
	}

	var peers []PeerInfo
	for _, p := range device.Peers {
		var allowedIPs []string
		for _, aip := range p.AllowedIPs {
			allowedIPs = append(allowedIPs, aip.String())
		}
		peers = append(peers, PeerInfo{
			PublicKey:       p.PublicKey.String(),
			Endpoint:        p.Endpoint.String(),
			AllowedIPs:      strings.Join(allowedIPs, ","),
			LatestHandshake: p.LastHandshakeTime.Unix(),
			TransferRx:      p.ReceiveBytes,
			TransferTx:      p.TransmitBytes,
			PersistentKeepalive: int(p.PersistentKeepaliveInterval.Seconds()),
		})
	}
	return peers, nil
}

// SetPeersCacheDir 设置对等端缓存目录（由 daemon 在启动时调用）
var peersCacheDir string

func SetPeersCacheDir(dir string) {
	peersCacheDir = dir
}

// SavePeersToCache 将对等端信息写入缓存文件（由守护进程调用）
func SavePeersToCache(peers []PeerInfo) error {
	if peersCacheDir == "" {
		return fmt.Errorf("peers cache dir not set")
	}
	data, err := json.Marshal(peers)
	if err != nil {
		return err
	}
	cacheFile := filepath.Join(peersCacheDir, "peers.cache")
	return os.WriteFile(cacheFile, data, 0644)
}

// GetPeersFromCache 从缓存文件读取对等端信息（由 CGI 调用）
func GetPeersFromCache(interfaceName string) ([]PeerInfo, error) {
	if peersCacheDir == "" {
		return []PeerInfo{}, nil
	}
	cacheFile := filepath.Join(peersCacheDir, "peers.cache")
	data, err := os.ReadFile(cacheFile)
	if err != nil {
		return []PeerInfo{}, nil
	}
	var peers []PeerInfo
	if err := json.Unmarshal(data, &peers); err != nil {
		return []PeerInfo{}, nil
	}
	return peers, nil
}

// getPeersFallback 降级方案：通过 /proc/net/wireguard 读取
func getPeersFallback(interfaceName string) ([]PeerInfo, error) {
	data, err := os.ReadFile("/proc/net/wireguard")
	if err != nil {
		return nil, err
	}
	return parseWireguardProc(string(data), interfaceName)
}

func parseWireguardProc(data, iface string) ([]PeerInfo, error) {
	var peers []PeerInfo
	lines := strings.Split(data, "\n")
	inTarget := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, iface+":") {
			inTarget = true
			continue
		}
		if inTarget {
			if line == "" {
				break
			}
			// 解析对等端行
			// 格式: public_key=xxx endpoint=xxx allowed_ips=xxx latest_handshake=xxx transfer=xxx persistent_keepalive=xxx
			peer := PeerInfo{}
			parts := strings.Fields(line)
			for _, part := range parts {
				kv := strings.SplitN(part, "=", 2)
				if len(kv) != 2 {
					continue
				}
				switch kv[0] {
				case "public_key":
					peer.PublicKey = kv[1]
				case "endpoint":
					peer.Endpoint = kv[1]
				case "allowed_ips":
					peer.AllowedIPs = kv[1]
				case "latest_handshake":
					fmt.Sscanf(kv[1], "%d", &peer.LatestHandshake)
				case "transfer":
					fmt.Sscanf(kv[1], "%d", &peer.TransferRx)
				}
			}
			if peer.PublicKey != "" {
				peers = append(peers, peer)
			}
		}
	}
	return peers, nil
}

// PeerInfo 对等端信息
type PeerInfo struct {
	PublicKey           string `json:"publicKey"`
	Endpoint            string `json:"endpoint"`
	AllowedIPs          string `json:"allowedIPs"`
	LatestHandshake     int64  `json:"latestHandshake"`
	TransferRx          int64  `json:"transferRx"`
	TransferTx          int64  `json:"transferTx"`
	PersistentKeepalive int    `json:"persistentKeepalive"`
}

// GetInterfaceTransfer 获取接口总流量（通过 sysfs，不依赖 wg 命令）
func GetInterfaceTransfer(interfaceName string) (rxBytes, txBytes int64, err error) {
	rx, errRx := readSysfsFile(fmt.Sprintf("/sys/class/net/%s/statistics/rx_bytes", interfaceName))
	tx, errTx := readSysfsFile(fmt.Sprintf("/sys/class/net/%s/statistics/tx_bytes", interfaceName))
	if errRx != nil || errTx != nil {
		return 0, 0, fmt.Errorf("read sysfs stats: rx=%v tx=%v", errRx, errTx)
	}
	return rx, tx, nil
}

func readSysfsFile(path string) (int64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
}

// IsKernelModuleLoaded 检查 WireGuard 内核模块是否加载
func IsKernelModuleLoaded() bool {
	data, err := os.ReadFile("/proc/modules")
	if err != nil {
		return false
	}
	return strings.Contains(string(data), "wireguard")
}

// GetKernelVersion 返回内核版本
func GetKernelVersion() string {
	data, err := os.ReadFile("/proc/version")
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(data))
}

// IsInterfaceUp 检查 WireGuard 接口是否启用
func IsInterfaceUp(name string) bool {
	data, err := os.ReadFile("/sys/class/net/" + name + "/carrier")
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(data)) == "1"
}
