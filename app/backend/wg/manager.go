package wg

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"

	"golang.org/x/crypto/curve25519"

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
}

// DefaultConfig returns the default WireGuard configuration.
func DefaultConfig() WGConfig {
	return WGConfig{
		InterfaceName: "wg0",
		Address:       "192.168.5.1/24",
		ListenPort:    51820,
		MTU:           1420,
		PostUp:        "iptables -A FORWARD -i wg0 -j ACCEPT; iptables -t nat -A POSTROUTING -o eth0 -j MASQUERADE",
		PostDown:      "iptables -D FORWARD -i wg0 -j ACCEPT; iptables -t nat -D POSTROUTING -o eth0 -j MASQUERADE",
	}
}

// GenerateKey generates a WireGuard key pair using pure Go (no wg command needed).
func GenerateKey() (privateKey, publicKey string, err error) {
	// Generate 32 random bytes for the private key
	var priv [32]byte
	if _, err := rand.Read(priv[:]); err != nil {
		return "", "", fmt.Errorf("generate private key: %w", err)
	}

	// Clamp the private key per WireGuard spec:
	// - Clear lowest 3 bits of byte 0
	// - Set highest bit of byte 31
	// - Clear 2nd highest bit of byte 31
	priv[0] &= 248
	priv[31] &= 127
	priv[31] |= 64

	// Compute public key = priv * basepoint
	var pub [32]byte
	curve25519.ScalarBaseMult(&pub, &priv)

	// Encode both as base64
	privateKey = base64.StdEncoding.EncodeToString(priv[:])
	publicKey = base64.StdEncoding.EncodeToString(pub[:])

	return privateKey, publicKey, nil
}

// GeneratePresharedKey generates a preshared key (32 random bytes, base64 encoded).
func GeneratePresharedKey() (string, error) {
	var key [32]byte
	if _, err := rand.Read(key[:]); err != nil {
		return "", fmt.Errorf("generate preshared key: %w", err)
	}
	return base64.StdEncoding.EncodeToString(key[:]), nil
}

// LoadConfig loads WireGuard configuration from database.
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

	return &cfg, nil
}

// SaveConfig saves WireGuard configuration to database.
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
	}
	for k, v := range pairs {
		if err := db.SetConfig(k, v); err != nil {
			return err
		}
	}
	return nil
}

// WriteConfigFile writes the WireGuard config file and applies it.
func WriteConfigFile(cfg WGConfig, users []db.User) (string, error) {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("[Interface]\n"))
	sb.WriteString(fmt.Sprintf("PrivateKey = %s\n", cfg.PrivateKey))
	sb.WriteString(fmt.Sprintf("Address = %s\n", cfg.Address))
	sb.WriteString(fmt.Sprintf("ListenPort = %d\n", cfg.ListenPort))
	if cfg.MTU > 0 {
		sb.WriteString(fmt.Sprintf("MTU = %d\n", cfg.MTU))
	}
	if cfg.DNS != "" {
		sb.WriteString(fmt.Sprintf("DNS = %s\n", cfg.DNS))
	}
	if cfg.PostUp != "" {
		sb.WriteString(fmt.Sprintf("PostUp = %s\n", cfg.PostUp))
	}
	if cfg.PostDown != "" {
		sb.WriteString(fmt.Sprintf("PostDown = %s\n", cfg.PostDown))
	}

	for _, u := range users {
		if !u.Enabled {
			continue
		}
		sb.WriteString(fmt.Sprintf("\n[Peer]\n"))
		sb.WriteString(fmt.Sprintf("PublicKey = %s\n", u.PublicKey))
		if u.PresharedKey != "" {
			sb.WriteString(fmt.Sprintf("PresharedKey = %s\n", u.PresharedKey))
		}
		sb.WriteString(fmt.Sprintf("AllowedIPs = %s\n", u.AllowedIPs))
	}

	configContent := sb.String()

	// Write to temp file and apply
	cmd := exec.Command("wg", "setconf", cfg.InterfaceName, "/dev/stdin")
	cmd.Stdin = strings.NewReader(configContent)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return configContent, fmt.Errorf("apply wg config: %s: %w", string(output), err)
	}

	return configContent, nil
}

// InitInterface initializes the WireGuard interface.
func InitInterface() error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}

	// Check if interface already exists
	iface := cfg.InterfaceName
	_, err = net.InterfaceByName(iface)
	if err == nil {
		// Interface exists, just apply config
		users, err := db.ListUsers()
		if err != nil {
			return err
		}
		_, err = WriteConfigFile(*cfg, users)
		return err
	}

	// Create interface using ip link
	cmds := []string{
		fmt.Sprintf("ip link add dev %s type wireguard", iface),
		fmt.Sprintf("ip address add dev %s %s", iface, cfg.Address),
		fmt.Sprintf("ip link set dev %s up", iface),
		fmt.Sprintf("ip link set dev %s mtu %d", iface, cfg.MTU),
	}

	for _, c := range cmds {
		parts := strings.Split(c, " ")
		cmd := exec.Command(parts[0], parts[1:]...)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("%s: %s: %w", c, string(output), err)
		}
	}

	// Apply config
	users, err := db.ListUsers()
	if err != nil {
		return err
	}
	_, err = WriteConfigFile(*cfg, users)
	return err
}

// RemovePeer removes a peer from the WireGuard interface.
func RemovePeer(interfaceName, publicKey string) error {
	cmd := exec.Command("wg", "set", interfaceName, "peer", publicKey, "remove")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("remove peer: %s: %w", string(output), err)
	}
	return nil
}

// GetPeers returns the current peers from the WireGuard interface.
func GetPeers(interfaceName string) ([]PeerInfo, error) {
	cmd := exec.Command("wg", "show", interfaceName, "dump")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("wg show: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) < 2 {
		return nil, nil
	}

	// First line is interface info, rest are peers
	var peers []PeerInfo
	for _, line := range lines[1:] {
		parts := strings.Fields(line)
		if len(parts) < 5 {
			continue
		}
		// private_key public_key listen_port endpoint allowed_ips latest_handshake transfer_rx transfer_tx persistent_keepalive
		peer := PeerInfo{
			PublicKey:  parts[0],
			Endpoint:   parts[2],
			AllowedIPs: parts[3],
		}
		fmt.Sscanf(parts[5], "%d", &peer.LatestHandshake)
		fmt.Sscanf(parts[6], "%d", &peer.TransferRx)
		fmt.Sscanf(parts[7], "%d", &peer.TransferTx)

		peers = append(peers, peer)
	}
	return peers, nil
}

// PeerInfo holds WireGuard peer information.
type PeerInfo struct {
	PublicKey       string `json:"publicKey"`
	Endpoint        string `json:"endpoint"`
	AllowedIPs      string `json:"allowedIPs"`
	LatestHandshake int64  `json:"latestHandshake"`
	TransferRx      int64  `json:"transferRx"`
	TransferTx      int64  `json:"transferTx"`
}

// GetInterfaceTransfer gets total transfer for the interface.
func GetInterfaceTransfer(interfaceName string) (rxBytes, txBytes int64, err error) {
	cmd := exec.Command("wg", "show", interfaceName, "transfer")
	output, err := cmd.Output()
	if err != nil {
		return 0, 0, fmt.Errorf("wg show transfer: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) > 0 {
		parts := strings.Fields(lines[0])
		if len(parts) >= 3 {
			fmt.Sscanf(parts[1], "%d", &txBytes)
			fmt.Sscanf(parts[2], "%d", &rxBytes)
		}
	}
	return
}

// DecodeBase64 decodes a base64 string.
func DecodeBase64(s string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(s)
}

// IsKernelModuleLoaded checks if the WireGuard kernel module is loaded.
func IsKernelModuleLoaded() bool {
	data, err := os.ReadFile("/proc/modules")
	if err != nil {
		return false
	}
	return strings.Contains(string(data), "wireguard")
}

// GetKernelVersion returns the kernel version.
func GetKernelVersion() string {
	data, err := os.ReadFile("/proc/version")
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(data))
}

// IsInterfaceUp checks if the WireGuard interface is up.
func IsInterfaceUp(name string) bool {
	data, err := os.ReadFile("/sys/class/net/" + name + "/carrier")
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(data)) == "1"
}
