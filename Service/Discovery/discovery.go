package Discovery

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"syscall"
	"time"

	"private_browser_client/Settings"
)

var (
	broadcasterMu sync.Mutex
	broadcaster   *Broadcaster
)

// BroadcastPayload 是新 Client 在独立内网中广播的最小发现报文。
//
// 设计来源：
// - 这次新项目虽然重建了业务模型，但 UDP 自动发现边界没有变；
// - Server 仍然需要先通过 UDP 找到本机 Edge 服务，再走 HTTP `/health` 和 `/device-info` 探测；
// - clientId 仍由 Node Server 分配，因此这里绝不能把 clientId/clientInstanceId 塞回广播里。
//
// 职责边界：
// - 只广播本机服务入口和非敏感能力摘要；
// - 不广播 slot/package/runtime relation 当前态，不广播 proxy、fingerprint、登录态或备份路径；
// - 这不是认证机制，只是独立内网里的服务发现报文。
type BroadcastPayload struct {
	DiscoveryMagic  string   `json:"discoveryMagic"`
	ProtocolVersion int      `json:"protocolVersion"`
	Service         string   `json:"service"`
	DiscoveryGroup  string   `json:"discoveryGroup"`
	ClientIP        string   `json:"clientIp"`
	BaseURL         string   `json:"baseUrl"`
	Hostname        string   `json:"hostname"`
	Mode            string   `json:"mode"`
	Version         string   `json:"version"`
	StartedAt       int64    `json:"startedAt"`
	LastHeartbeatAt int64    `json:"lastHeartbeatAt"`
	Capabilities    []string `json:"capabilities"`
}

// Broadcaster 管理 UDP discovery 广播循环。
//
// 当前新 Client 继续沿用 old 的设计原则：
// - 只发，不收；
// - 不维护节点列表；
// - 不做自动注册中心；
// - 不参与 clientId 分配。
type Broadcaster struct {
	config    *Settings.DiscoveryConfig
	startedAt int64
	stopCh    chan struct{}
	doneCh    chan struct{}
}

// StartBroadcaster 启动全局 discovery broadcaster。
//
// 基础设施层会在 HTTP 服务启动前拉起它，保证 Node Server 在扫描时能尽快看到本机 Client。
func StartBroadcaster() *Broadcaster {
	broadcasterMu.Lock()
	defer broadcasterMu.Unlock()
	if broadcaster != nil {
		return broadcaster
	}

	instance := NewBroadcaster(Settings.Conf.DiscoveryConfig)
	broadcaster = instance
	instance.Start()
	return instance
}

// StopBroadcaster 停止全局 discovery broadcaster。
//
// 服务退出时先停广播，避免 Edge API 已经下线但 UDP 还在误导 Server 继续发现本机。
func StopBroadcaster() {
	broadcasterMu.Lock()
	instance := broadcaster
	broadcaster = nil
	broadcasterMu.Unlock()
	if instance != nil {
		instance.Stop()
	}
}

func NewBroadcaster(config *Settings.DiscoveryConfig) *Broadcaster {
	return &Broadcaster{
		config:    config,
		startedAt: time.Now().Unix(),
		stopCh:    make(chan struct{}),
		doneCh:    make(chan struct{}),
	}
}

// Start 只启动周期广播，不创建 UDP 监听。
func (b *Broadcaster) Start() {
	if b == nil || b.config == nil || !b.config.Enabled {
		close(b.doneCh)
		return
	}
	go b.loop()
}

func (b *Broadcaster) Stop() {
	if b == nil {
		return
	}
	select {
	case <-b.doneCh:
		return
	default:
	}

	close(b.stopCh)
	select {
	case <-b.doneCh:
	case <-time.After(3 * time.Second):
		log.Printf("udp discovery broadcaster stop timeout\n")
	}
}

func (b *Broadcaster) loop() {
	defer close(b.doneCh)
	b.broadcastOnce()

	interval := time.Duration(b.config.IntervalSeconds) * time.Second
	if interval < 3*time.Second {
		interval = 5 * time.Second
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-b.stopCh:
			return
		case <-ticker.C:
			b.broadcastOnce()
		}
	}
}

func (b *Broadcaster) broadcastOnce() {
	payload := b.buildPayload()
	body, err := json.Marshal(payload)
	if err != nil {
		log.Printf("marshal udp discovery payload failed: %v\n", err)
		return
	}

	addr := fmt.Sprintf("%s:%d", strings.TrimSpace(b.config.BroadcastAddress), b.config.Port)
	udpAddr, err := net.ResolveUDPAddr("udp4", addr)
	if err != nil {
		log.Printf("resolve udp discovery address failed, addr=%s, err=%v\n", addr, err)
		return
	}

	conn, err := dialBroadcastUDP(udpAddr)
	if err != nil {
		log.Printf("dial udp discovery address failed, addr=%s, err=%v\n", addr, err)
		return
	}
	defer conn.Close()

	if err = conn.SetWriteBuffer(32 * 1024); err != nil {
		log.Printf("set udp discovery write buffer failed, addr=%s, err=%v\n", addr, err)
	}
	if _, err = conn.Write(body); err != nil {
		log.Printf("write udp discovery payload failed, addr=%s, err=%v\n", addr, err)
	}
}

func dialBroadcastUDP(addr *net.UDPAddr) (*net.UDPConn, error) {
	dialer := &net.Dialer{
		Control: func(network, address string, rawConn syscall.RawConn) error {
			var controlErr error
			if err := rawConn.Control(func(fd uintptr) {
				controlErr = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_BROADCAST, 1)
			}); err != nil {
				return err
			}
			return controlErr
		},
	}
	conn, err := dialer.Dial("udp4", addr.String())
	if err != nil {
		return nil, err
	}
	udpConn, ok := conn.(*net.UDPConn)
	if !ok {
		_ = conn.Close()
		return nil, fmt.Errorf("dialed conn is not UDP")
	}
	return udpConn, nil
}

func (b *Broadcaster) buildPayload() BroadcastPayload {
	clientIP := resolveAdvertiseHost(b.config)
	baseURL := resolveAdvertiseBaseURL(b.config, clientIP)
	hostname, _ := os.Hostname()
	now := time.Now().Unix()

	return BroadcastPayload{
		DiscoveryMagic:  strings.TrimSpace(b.config.Magic),
		ProtocolVersion: b.config.ProtocolVersion,
		Service:         Settings.Conf.Name,
		DiscoveryGroup:  strings.TrimSpace(b.config.Group),
		ClientIP:        clientIP,
		BaseURL:         baseURL,
		Hostname:        hostname,
		Mode:            Settings.Conf.Mode,
		Version:         Settings.Conf.Version,
		StartedAt:       b.startedAt,
		LastHeartbeatAt: now,
		Capabilities: []string{
			"edge-api",
			"docker-2375",
			"slot-runtime",
			"slot-vnc",
			"slot-cdp",
			"swagger",
		},
	}
}

func resolveAdvertiseBaseURL(config *Settings.DiscoveryConfig, clientIP string) string {
	if config != nil && strings.TrimSpace(config.AdvertiseBaseURL) != "" {
		return strings.TrimRight(strings.TrimSpace(config.AdvertiseBaseURL), "/")
	}
	port := 3300
	if Settings.Conf != nil && Settings.Conf.ServerConfig != nil && Settings.Conf.ServerConfig.Port > 0 {
		port = Settings.Conf.ServerConfig.Port
	}
	return fmt.Sprintf("http://%s:%d", clientIP, port)
}

func resolveAdvertiseHost(config *Settings.DiscoveryConfig) string {
	if config != nil && strings.TrimSpace(config.AdvertiseHost) != "" {
		return strings.TrimSpace(config.AdvertiseHost)
	}

	serverHost := ""
	if Settings.Conf != nil && Settings.Conf.ServerConfig != nil {
		serverHost = strings.TrimSpace(Settings.Conf.ServerConfig.Host)
	}
	if serverHost != "" && serverHost != "0.0.0.0" && serverHost != "::" && serverHost != "[::]" {
		return serverHost
	}
	if ip := firstPrivateIPv4(); ip != "" {
		return ip
	}
	return "127.0.0.1"
}

// CurrentAdvertiseHost 返回当前 Client 对外声明给 Node Server 的接入 IP。
//
// 这个封装存在的原因是：新补的 Node 注册、heartbeat、UDP discovery
// 都必须复用同一套“外部应该看到的地址”口径，避免注册用一个 IP、心跳又上报另一个 IP。
func CurrentAdvertiseHost() string {
	return resolveAdvertiseHost(Settings.Conf.DiscoveryConfig)
}

// CurrentAdvertiseBaseURL 返回当前 Client 对外声明给 Node Server 的 HTTP 基地址。
//
// 这里继续复用 discovery 的地址推导逻辑，保证：
// - UDP beacon 里看到的 baseUrl
// - HTTP heartbeat 里上报的 baseUrl
// - Node 注册时提交的 baseUrl
// 三者保持一致，避免 Node 因为地址不一致把同一台 Client 当成两条节点线索。
func CurrentAdvertiseBaseURL() string {
	clientIP := CurrentAdvertiseHost()
	return resolveAdvertiseBaseURL(Settings.Conf.DiscoveryConfig, clientIP)
}

func firstPrivateIPv4() string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return ""
	}

	fallback := ""
	for _, item := range interfaces {
		if item.Flags&net.FlagUp == 0 || item.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := item.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ip := extractIPv4(addr)
			if ip == nil {
				continue
			}
			if ip.IsPrivate() {
				return ip.String()
			}
			if fallback == "" {
				fallback = ip.String()
			}
		}
	}
	return fallback
}

func extractIPv4(addr net.Addr) net.IP {
	switch value := addr.(type) {
	case *net.IPNet:
		return value.IP.To4()
	case *net.IPAddr:
		return value.IP.To4()
	default:
		return nil
	}
}
