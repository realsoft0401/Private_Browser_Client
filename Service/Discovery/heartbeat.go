package Discovery

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	Model "private_browser_client/Models/NodeRegister"
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

// TriggerHeartbeatNow 允许业务链路在关键时刻主动触发一次即时 heartbeat。
//
// 设计来源：
// - 当前正式规则已经收口成“bind 成功 -> Client 写入 node-registration.json -> Client 开始主动 heartbeat”；
// - 如果只依赖定时 ticker，Node 侧数据库要等一个周期才会看到最新活性，这会让绑定刚完成时的管理视图显得滞后；
// - 因此这里提供一个受控即时触发入口，专门给 assign 成功后的链路使用。
//
// 职责边界：
// - 这里只负责“如果心跳器已经启动，就立刻补打一发 heartbeat”；
// - 不负责启动新的心跳器，不绕过本地 node-registration.json 校验，不改变定时心跳策略。
func TriggerHeartbeatNow() {
	heartbeatMu.Lock()
	instance := heartbeatPusher
	heartbeatMu.Unlock()
	if instance == nil {
		return
	}
	instance.pushOnce()
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
	if p == nil || p.heartbeatConfig == nil || !p.heartbeatConfig.Enabled {
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
	registration, err := loadCachedRegistration()
	if err != nil {
		log.Printf("load node registration for heartbeat failed: %v\n", err)
		return
	}
	if registration == nil || strings.TrimSpace(registration.NodeServerBaseURL) == "" {
		// Node 还没有完成 bind 并写回本地 JSON 时，Client 不应主动向任何地址打心跳。
		return
	}

	payload := p.buildPayload()
	body, err := json.Marshal(payload)
	if err != nil {
		log.Printf("marshal node heartbeat payload failed: %v\n", err)
		return
	}

	// 这里显式固定为 `/api/v1/edge-clients/heartbeat`。
	//
	// 设计来源：
	// - 这一轮联调已经暴露出一个真实问题：Client 仍在请求历史路径
	//   `/api/v1/server/edge-clients/heartbeat`，而 Node 当前正式路由已经收口到
	//   `/api/v1/edge-clients/heartbeat`；
	// - 结果就是 3300 明明在线、3400 却一直“发现不到”，因为心跳根本没打到真实入口；
	// - 这条路径必须和 Node 当前路由保持强一致，后续如果 Server 改路由，Client、OpenAPI、
	//   配置说明和联调文档都必须一起改，不能再只改一边。
	endpoint := strings.TrimRight(registration.NodeServerBaseURL, "/") + "/api/v1/edge-clients/heartbeat"
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
	registration, err := loadCachedRegistration()
	if err != nil || registration == nil || strings.TrimSpace(registration.NodeServerBaseURL) == "" {
		return ""
	}
	return fmt.Sprintf("%s/api/v1/edge-clients/heartbeat", strings.TrimRight(registration.NodeServerBaseURL, "/"))
}

func loadCachedRegistration() (*Model.RegistrationState, error) {
	path := filepath.Join(Settings.Conf.ProjectRoot, "data", "node-registration.json")
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read node registration cache failed: %w", err)
	}
	var state Model.RegistrationState
	if err = json.Unmarshal(body, &state); err != nil {
		return nil, fmt.Errorf("decode node registration cache failed: %w", err)
	}
	if strings.TrimSpace(state.ClientID) == "" {
		return nil, nil
	}
	return &state, nil
}
