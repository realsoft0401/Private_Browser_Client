package Discovery

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"private_browser_client/Settings"
)

var (
	broadcasterMu sync.Mutex
	broadcaster   *Broadcaster
)

// BroadcastPayload 是 Client 在内网 UDP 中广播的最小服务发现报文。
//
// 设计来源：
// - 用户希望 Server 自动扫描内网加入 Client，也支持手动加入；
// - 用户明确不再使用 clientInstanceId，Client IP 是连接位置，clientId 由中心服务发放；
// - UDP 报文必须带 discoveryMagic / protocolVersion / discoveryGroup，Server 只接收符合协议的 Client 广播。
//
// 职责边界：
// - 这里只广播服务位置和基础能力，帮助 Server 找到 Client；
// - 不广播用户、环境包、代理、指纹、登录态、宿主机环境变量或 Docker 详情；
// - 这不是鉴权机制，Client V1 仍依赖独立内网、防火墙和上游 Server 访问控制。
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

// Broadcaster 管理 UDP discovery 后台广播循环。
//
// 这个后台任务只负责“把自己是谁、在哪里”广播出去，不负责接收 UDP、
// 不维护 Edge Client 列表，也不自动向 Server 注册。Server 收到广播后仍需要按 clientId/IP 规则确认身份。
type Broadcaster struct {
	config    *Settings.DiscoveryConfig
	startedAt int64
	stopCh    chan struct{}
	doneCh    chan struct{}
}

// StartBroadcaster 启动全局 UDP discovery 广播器。
//
// 基础设施层在 HTTP 服务启动前调用它；如果配置关闭，则不启动 goroutine。
// 广播失败只写日志，不阻塞 Client 主服务，因为手动添加 Client 仍然可用。
func StartBroadcaster() *Broadcaster {
	broadcasterMu.Lock()
	defer broadcasterMu.Unlock()
	if broadcaster != nil {
		return broadcaster
	}
	b := NewBroadcaster(Settings.Conf.DiscoveryConfig)
	broadcaster = b
	b.Start()
	return b
}

// StopBroadcaster 停止全局 UDP discovery 广播器。
//
// 退出时先停广播再关进程，避免服务已经不可用但仍继续发出可发现信号。
func StopBroadcaster() {
	broadcasterMu.Lock()
	b := broadcaster
	broadcaster = nil
	broadcasterMu.Unlock()
	if b != nil {
		b.Stop()
	}
}

// NewBroadcaster 根据配置创建广播器。
//
// Settings 层已经做过默认值归一化；这里保留简单结构，便于后续把广播状态加入 /health。
func NewBroadcaster(config *Settings.DiscoveryConfig) *Broadcaster {
	return &Broadcaster{
		config:    config,
		startedAt: time.Now().Unix(),
		stopCh:    make(chan struct{}),
		doneCh:    make(chan struct{}),
	}
}

// Start 启动周期广播。
//
// Start 不创建监听端口，不接收任何 UDP 数据；Client 仍然是单节点边缘服务，不变成节点注册中心。
func (b *Broadcaster) Start() {
	if b == nil || b.config == nil || !b.config.Enabled {
		close(b.doneCh)
		return
	}
	go b.loop()
}

// Stop 停止周期广播。
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

// loop 执行立即广播和周期广播。
//
// 立即广播是为了让 Server 启动扫描时不必等满一个周期；后续每隔 interval 再发一次心跳。
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

// broadcastOnce 发送一帧 discovery JSON。
//
// UDP 广播在不同操作系统和网段上可能因为路由、防火墙或容器网络限制失败；
// 这里明确记录错误，但不影响生命周期 API，因为 Server 仍可通过手动 IP 添加 Client。
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

	conn, err := net.DialUDP("udp4", nil, udpAddr)
	if err != nil {
		log.Printf("dial udp discovery address failed, addr=%s, err=%v\n", addr, err)
		return
	}
	defer conn.Close()
	if _, err = conn.Write(body); err != nil {
		log.Printf("write udp discovery payload failed, addr=%s, err=%v\n", addr, err)
	}
}

// buildPayload 生成不含敏感信息的发现报文。
//
// Client IP 只是 Server 连接到本 Client 的网络位置，不是身份主键；
// 如果 IP 后续变化，Server 应标记 IP 不一致并等待人工更新，而不是自动覆盖 clientId 身份。
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
			"browser-env-lifecycle",
			"webvnc",
			"swagger",
			"sse-task-progress",
		},
	}
}

// resolveAdvertiseBaseURL 计算 Server 可以访问的 Client API 根地址。
//
// 如果部署环境有反向代理或固定管理地址，应显式配置 advertise_base_url；
// 否则按当前 Client IP 和本机监听端口生成内网 HTTP 地址。
func resolveAdvertiseBaseURL(config *Settings.DiscoveryConfig, clientIP string) string {
	if config != nil && strings.TrimSpace(config.AdvertiseBaseURL) != "" {
		return strings.TrimRight(strings.TrimSpace(config.AdvertiseBaseURL), "/")
	}
	port := Settings.Conf.ServerConfig.Port
	if port <= 0 {
		port = 3300
	}
	return fmt.Sprintf("http://%s:%d", clientIP, port)
}

// resolveAdvertiseHost 选择 UDP 报文里的 Client IP。
//
// 优先级是显式 advertise_host > server.host 中的固定 IP > 本机非 loopback 私网 IPv4。
// 这样既支持容器/多网卡部署手工指定，也能在普通内网机器上开箱即用。
func resolveAdvertiseHost(config *Settings.DiscoveryConfig) string {
	if config != nil && strings.TrimSpace(config.AdvertiseHost) != "" {
		return strings.TrimSpace(config.AdvertiseHost)
	}
	serverHost := strings.TrimSpace(Settings.Conf.ServerConfig.Host)
	if serverHost != "" && serverHost != "0.0.0.0" && serverHost != "::" && serverHost != "[::]" {
		return serverHost
	}
	if ip := firstPrivateIPv4(); ip != "" {
		return ip
	}
	return "127.0.0.1"
}

// firstPrivateIPv4 扫描本机网卡并返回第一个可用私网 IPv4。
//
// Discovery 只面向独立内网，优先私网地址可以避免把 loopback 或 Docker bridge 地址误发给 Server。
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

// extractIPv4 从 net.Addr 中提取 IPv4 地址。
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
