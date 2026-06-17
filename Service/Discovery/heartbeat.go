package Discovery

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"private_browser_client/Settings"
)

var (
	heartbeatMu     sync.Mutex
	heartbeatPusher *HeartbeatPusher
)

// HeartbeatPayload 是 Client 发给 Node Server 正式心跳接口的最小请求体。
//
// 它刻意和 UDP beacon 保持同一组 discovery 身份字段，
// 这样 Node Server 能用同一套协议口径理解“这个 Client 是谁、在哪里”。
type HeartbeatPayload struct {
	DiscoveryMagic  string `json:"discoveryMagic"`
	ProtocolVersion int    `json:"protocolVersion"`
	Service         string `json:"service"`
	DiscoveryGroup  string `json:"discoveryGroup"`
	BaseURL         string `json:"baseUrl,omitempty"`
	ClientIP        string `json:"clientIp,omitempty"`
	LastHeartbeatAt int64  `json:"lastHeartbeatAt,omitempty"`
}

// HeartbeatPusher 管理 Client -> Node Server 的正式心跳循环。
//
// 职责边界：
// - 只把 discovery 域最小摘要打给 Node Server；
// - 不负责 clientId 分配，不负责节点 verify，不负责业务状态回写；
// - 心跳失败只记日志，不阻塞 Client 本机 API。
type HeartbeatPusher struct {
	discoveryConfig *Settings.DiscoveryConfig
	heartbeatConfig *Settings.HeartbeatConfig
	httpClient      *http.Client
	stopCh          chan struct{}
	doneCh          chan struct{}
}

func StartHeartbeatPusher() *HeartbeatPusher {
	heartbeatMu.Lock()
	defer heartbeatMu.Unlock()
	if heartbeatPusher != nil {
		return heartbeatPusher
	}

	instance := NewHeartbeatPusher(Settings.Conf.DiscoveryConfig, Settings.Conf.HeartbeatConfig)
	heartbeatPusher = instance
	instance.Start()
	return instance
}

func StopHeartbeatPusher() {
	heartbeatMu.Lock()
	instance := heartbeatPusher
	heartbeatPusher = nil
	heartbeatMu.Unlock()
	if instance != nil {
		instance.Stop()
	}
}

func NewHeartbeatPusher(discoveryConfig *Settings.DiscoveryConfig, heartbeatConfig *Settings.HeartbeatConfig) *HeartbeatPusher {
	return &HeartbeatPusher{
		discoveryConfig: discoveryConfig,
		heartbeatConfig: heartbeatConfig,
		httpClient:      &http.Client{Timeout: 10 * time.Second},
		stopCh:          make(chan struct{}),
		doneCh:          make(chan struct{}),
	}
}

func (p *HeartbeatPusher) Start() {
	if p == nil || p.heartbeatConfig == nil || !p.heartbeatConfig.Enabled || strings.TrimSpace(p.heartbeatConfig.ServerBaseURL) == "" {
		close(p.doneCh)
		return
	}
	go p.loop()
}

func (p *HeartbeatPusher) Stop() {
	if p == nil {
		return
	}
	select {
	case <-p.doneCh:
		return
	default:
	}
	close(p.stopCh)
	select {
	case <-p.doneCh:
	case <-time.After(3 * time.Second):
		log.Printf("node heartbeat pusher stop timeout\n")
	}
}

func (p *HeartbeatPusher) loop() {
	defer close(p.doneCh)
	p.pushOnce()

	interval := time.Duration(p.heartbeatConfig.IntervalSeconds) * time.Second
	if interval < 5*time.Second {
		interval = 5 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-p.stopCh:
			return
		case <-ticker.C:
			p.pushOnce()
		}
	}
}

func (p *HeartbeatPusher) pushOnce() {
	payload := p.buildPayload()
	body, err := json.Marshal(payload)
	if err != nil {
		log.Printf("marshal node heartbeat payload failed: %v\n", err)
		return
	}

	endpoint := strings.TrimRight(p.heartbeatConfig.ServerBaseURL, "/") + "/api/v1/server/edge-clients/heartbeat"
	request, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		log.Printf("build node heartbeat request failed, endpoint=%s, err=%v\n", endpoint, err)
		return
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := p.httpClient.Do(request)
	if err != nil {
		log.Printf("post node heartbeat failed, endpoint=%s, err=%v\n", endpoint, err)
		return
	}
	defer response.Body.Close()
	if response.StatusCode >= 400 {
		log.Printf("node heartbeat status not ok, endpoint=%s, status=%s\n", endpoint, response.Status)
	}
}

func (p *HeartbeatPusher) buildPayload() HeartbeatPayload {
	clientIP := resolveAdvertiseHost(p.discoveryConfig)
	baseURL := resolveAdvertiseBaseURL(p.discoveryConfig, clientIP)
	return HeartbeatPayload{
		DiscoveryMagic:  strings.TrimSpace(p.discoveryConfig.Magic),
		ProtocolVersion: p.discoveryConfig.ProtocolVersion,
		Service:         Settings.Conf.Name,
		DiscoveryGroup:  strings.TrimSpace(p.discoveryConfig.Group),
		BaseURL:         baseURL,
		ClientIP:        clientIP,
		LastHeartbeatAt: time.Now().Unix(),
	}
}

func (p *HeartbeatPusher) DebugEndpoint() string {
	if p == nil || p.heartbeatConfig == nil {
		return ""
	}
	return fmt.Sprintf("%s/api/v1/server/edge-clients/heartbeat", strings.TrimRight(p.heartbeatConfig.ServerBaseURL, "/"))
}
