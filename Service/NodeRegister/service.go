package NodeRegister

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	Model "private_browser_client/Models/NodeRegister"
	DiscoveryService "private_browser_client/Service/Discovery"
	"private_browser_client/Settings"
)

const nodeResponseSuccessCode int64 = 1000

var errRemoteClientNotFound = errors.New("remote client not found")
var errAssignUnauthorized = errors.New("X-Edge-API-Key 无效")
var errAssignInvalidParams = errors.New("assign request invalid")

// Service 管理 Client -> Node Server 的中心注册能力。
//
// 职责边界：
// - 负责拼注册请求、查询远端是否已存在、接收 Node 分配的 clientId，并把结果返回给当前调用链；
// - 不负责 verify，不负责平台运行额度，不负责替 Node 判断节点是否可业务放行；
// - 如果 Node 已经存在同 baseUrl/clientIp 的节点，这里优先复用，而不是再次生成新 clientId；
// - project.md 已收紧边界：Client 不保存中心 clientId，所以这里不能再把返回结果落到本地 SQLite/JSON。
type Service struct {
	httpClient *http.Client
}

func NewService() *Service {
	return &Service{
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

// BuildStatusView 返回当前 Client 看到的中心注册状态。
//
// 设计来源：
// - 用户需要在本机接口里直接看到“现在这台 Client 对中心暴露了什么入口、Node 那边是否已存在登记结果”；
// - 同时用户又希望 Client 本地保留一份 JSON 留痕，因此这里同时返回“本地缓存”和“Node 实时结果”；
// - 如果 Node 暂时不可达，也必须把失败原因回出来，方便联调而不是静默显示未注册。
func (s *Service) BuildStatusView(ctx context.Context) Model.StatusView {
	configReady, configMessage := validateConfig()
	clientIP := DiscoveryService.CurrentAdvertiseHost()
	baseURL := DiscoveryService.CurrentAdvertiseBaseURL()
	dockerAPIURL := resolveNodeVisibleDockerAPIURL(clientIP)
	nodeName := resolveNodeName()
	cachedState, cacheErr := loadCachedState()
	view := Model.StatusView{
		Enabled:            Settings.Conf.NodeRegisterConfig != nil && Settings.Conf.NodeRegisterConfig.Enabled,
		ConfigReady:        configReady,
		ConfigMessage:      configMessage,
		NodeName:           nodeName,
		BaseURL:            baseURL,
		ClientIP:           clientIP,
		DockerAPIURL:       dockerAPIURL,
		ServerBaseURL:      strings.TrimSpace(Settings.Conf.NodeRegisterConfig.ServerBaseURL),
		MainAccountID:      strings.TrimSpace(Settings.Conf.NodeRegisterConfig.MainAccountID),
		Registered:         false,
		LookupStatus:       "not_checked",
		LookupMessage:      "尚未执行 Node 查询",
		CacheStatus:        "missing",
		CacheMessage:       "本地尚无 node-registration 缓存文件",
		CachedRegistration: cachedState,
	}
	if cacheErr != nil {
		view.CacheStatus = "cache_invalid"
		view.CacheMessage = cacheErr.Error()
	} else if cachedState != nil {
		view.CacheStatus = "cached"
		view.CacheMessage = "本地 JSON 已留存上次 Node 分配结果"
	}
	if !configReady {
		view.LookupStatus = "config_invalid"
		view.LookupMessage = configMessage
		return view
	}

	state, err := s.lookupRemoteState(ctx, nodeName, baseURL, clientIP, dockerAPIURL)
	if err == nil && state != nil {
		view.Registered = true
		view.LookupStatus = "found"
		view.LookupMessage = "Node 已存在当前 Client 的中心登记结果"
		view.Registration = state
		return view
	}
	if errors.Is(err, errRemoteClientNotFound) {
		view.LookupStatus = "not_found"
		view.LookupMessage = "Node 当前尚未登记这台 Client"
		return view
	}
	if err != nil {
		view.LookupStatus = "lookup_failed"
		view.LookupMessage = err.Error()
		return view
	}
	return view
}

type remoteNode struct {
	ClientID     string `json:"clientId"`
	AccountID    string `json:"accountId"`
	Name         string `json:"name"`
	BaseURL      string `json:"baseUrl"`
	ClientIP     string `json:"clientIp"`
	DockerAPIURL string `json:"dockerApiUrl"`
	CreatedAt    int64  `json:"createdAt"`
	UpdatedAt    int64  `json:"updatedAt"`
}

// AssignClientID 校验 Node 下发结果，并写入本地 node-registration.json。
//
// 这条链路是当前第一阶段正式主线的最后一段：
// - Node 已完成中心 bind
// - Client 只接收结果并本地留痕
// - 不把本地缓存升级成中心真相源
func (s *Service) AssignClientID(ctx context.Context, apiKey string, request Model.AssignRequest) (*Model.AssignResult, error) {
	if err := validateAssignAPIKey(apiKey); err != nil {
		return nil, err
	}
	if err := validateAssignRequest(request); err != nil {
		return nil, err
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	clientIP := DiscoveryService.CurrentAdvertiseHost()
	baseURL := DiscoveryService.CurrentAdvertiseBaseURL()
	dockerAPIURL := resolveNodeVisibleDockerAPIURL(clientIP)
	nodeName := resolveNodeName()
	assignedAt := request.AssignedAt
	if assignedAt <= 0 {
		assignedAt = time.Now().Unix()
	}
	state := &Model.RegistrationState{
		ClientID:          strings.TrimSpace(request.ClientID),
		MainAccountID:     strings.TrimSpace(request.AccountID),
		NodeServerBaseURL: strings.TrimRight(strings.TrimSpace(Settings.Conf.NodeRegisterConfig.ServerBaseURL), "/"),
		NodeName:          nodeName,
		BaseURL:           normalizeURL(baseURL),
		ClientIP:          strings.TrimSpace(clientIP),
		DockerAPIURL:      strings.TrimSpace(dockerAPIURL),
		Source:            firstNonEmpty(strings.TrimSpace(request.Source), "node-assign"),
		RegisteredAt:      assignedAt,
		UpdatedAt:         time.Now().Unix(),
	}

	previous, err := loadCachedState()
	if err != nil {
		return nil, err
	}
	if previous != nil && (previous.ClientID != state.ClientID || previous.MainAccountID != state.MainAccountID) {
		fmt.Printf(
			"node registration cache overwrite detected: oldClientId=%s newClientId=%s oldAccountId=%s newAccountId=%s source=%s updatedAt=%d\n",
			previous.ClientID, state.ClientID, previous.MainAccountID, state.MainAccountID, state.Source, state.UpdatedAt,
		)
	}
	if previous != nil && previous.RegisteredAt > 0 && previous.ClientID == state.ClientID {
		state.RegisteredAt = previous.RegisteredAt
	}
	if err = saveCachedState(state); err != nil {
		return nil, err
	}
	return &Model.AssignResult{
		Written:      true,
		CachePath:    nodeRegistrationCachePath(),
		Registration: state,
	}, nil
}

type nodeListEnvelope struct {
	Code    int64  `json:"code"`
	Message string `json:"message"`
	Data    struct {
		Items []remoteNode `json:"items"`
		Total int          `json:"total"`
	} `json:"data"`
}

func (s *Service) findExistingRemote(ctx context.Context, baseURL, clientIP string) (*remoteNode, error) {
	endpoint, err := buildNodeEdgeClientsListURL()
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build node list request failed: %w", err)
	}
	response, err := s.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("query node edge-clients failed: %w", err)
	}
	defer response.Body.Close()

	var envelope nodeListEnvelope
	if err = json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		return nil, fmt.Errorf("decode node edge-clients response failed: %w", err)
	}
	if envelope.Code != nodeResponseSuccessCode {
		return nil, fmt.Errorf("node edge-clients response not success: %s", envelope.Message)
	}
	normalizedBaseURL := normalizeURL(baseURL)
	clientIP = strings.TrimSpace(clientIP)
	for _, item := range envelope.Data.Items {
		if normalizedBaseURL != "" && normalizeURL(item.BaseURL) == normalizedBaseURL {
			return &item, nil
		}
		if clientIP != "" && strings.TrimSpace(item.ClientIP) == clientIP {
			return &item, nil
		}
	}
	return nil, errRemoteClientNotFound
}

func buildNodeEdgeClientsListURL() (string, error) {
	serverBaseURL := strings.TrimRight(Settings.Conf.NodeRegisterConfig.ServerBaseURL, "/")
	rawURL := serverBaseURL + "/api/v1/edge-clients"
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("build node edge-clients url failed: %w", err)
	}
	query := parsed.Query()
	query.Set("accountId", strings.TrimSpace(Settings.Conf.NodeRegisterConfig.MainAccountID))
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func validateConfig() (bool, string) {
	config := Settings.Conf.NodeRegisterConfig
	if config == nil || !config.Enabled {
		return false, "node_register 未启用；如需让 Client 查询中心登记状态并接收 assign，请先在 Settings/config-docker.yaml 打开 node_register.enabled"
	}
	if strings.TrimSpace(config.ServerBaseURL) == "" {
		return false, "node_register.server_base_url 不能为空"
	}
	if strings.TrimSpace(config.MainAccountID) == "" {
		return false, "node_register.main_account_id 不能为空"
	}
	return true, "ready"
}

func resolveNodeName() string {
	config := Settings.Conf.NodeRegisterConfig
	if config != nil && strings.TrimSpace(config.NodeName) != "" {
		return strings.TrimSpace(config.NodeName)
	}
	hostname, err := os.Hostname()
	if err == nil && strings.TrimSpace(hostname) != "" {
		return strings.TrimSpace(hostname)
	}
	return "private-browser-client"
}

func resolveNodeVisibleDockerAPIURL(clientIP string) string {
	raw := strings.TrimSpace(Settings.Conf.DockerConfig.APIURL)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	host := parsed.Hostname()
	if host == "" {
		return raw
	}
	if host != "127.0.0.1" && host != "localhost" && host != "0.0.0.0" {
		return raw
	}
	if strings.TrimSpace(clientIP) == "" {
		return raw
	}
	port := parsed.Port()
	if port == "" {
		port = "2375"
	}
	return fmt.Sprintf("%s://%s:%s%s", parsed.Scheme, strings.TrimSpace(clientIP), port, parsed.EscapedPath())
}

func buildStateFromRemote(node *remoteNode, nodeName, baseURL, clientIP, dockerAPIURL, source string) *Model.RegistrationState {
	if node == nil {
		return nil
	}
	registeredAt := node.CreatedAt
	if registeredAt <= 0 {
		registeredAt = time.Now().Unix()
	}
	updatedAt := node.UpdatedAt
	if updatedAt <= 0 {
		updatedAt = time.Now().Unix()
	}
	return &Model.RegistrationState{
		ClientID:          strings.TrimSpace(node.ClientID),
		MainAccountID:     firstNonEmpty(strings.TrimSpace(node.AccountID), strings.TrimSpace(Settings.Conf.NodeRegisterConfig.MainAccountID)),
		NodeServerBaseURL: strings.TrimSpace(Settings.Conf.NodeRegisterConfig.ServerBaseURL),
		NodeName:          firstNonEmpty(strings.TrimSpace(node.Name), nodeName),
		BaseURL:           firstNonEmpty(normalizeURL(node.BaseURL), normalizeURL(baseURL)),
		ClientIP:          firstNonEmpty(strings.TrimSpace(node.ClientIP), strings.TrimSpace(clientIP)),
		DockerAPIURL:      firstNonEmpty(strings.TrimSpace(node.DockerAPIURL), strings.TrimSpace(dockerAPIURL)),
		Source:            strings.TrimSpace(source),
		RegisteredAt:      registeredAt,
		UpdatedAt:         updatedAt,
	}
}

// lookupRemoteState 把“Node 是否已经认识这台 Client”收敛成单独函数。
//
// 这样拆开的原因：
//   - 状态接口和 assign 后回读都需要同一套 baseUrl/clientIp 去重规则；
//   - 这里故意只查 Node 远端事实，不把本地 JSON 缓存当成真相源，
//     避免未来有人把缓存结果误当成正式中心身份。
func (s *Service) lookupRemoteState(ctx context.Context, nodeName, baseURL, clientIP, dockerAPIURL string) (*Model.RegistrationState, error) {
	node, err := s.findExistingRemote(ctx, baseURL, clientIP)
	if err != nil {
		return nil, err
	}
	return buildStateFromRemote(node, nodeName, baseURL, clientIP, dockerAPIURL, "remote-list"), nil
}

// loadCachedState 读取本地 node-registration JSON 留痕。
//
// 设计边界：
// - 这里只读取“上次 Node 分配结果”的缓存副本；
// - 它服务于重启后展示、人工排障和辅助比对；
// - 它不是正式真相源，不能代替实时查 Node。
func loadCachedState() (*Model.RegistrationState, error) {
	path := nodeRegistrationCachePath()
	body, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
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

// saveCachedState 把 Node 返回的中心登记结果写入本地 JSON。
//
// 为什么保留这层：
// - 用户明确要求不要落数据库，而要单独留一个 JSON 文件；
// - 这样既能保留重启后的留痕，又不会把中心身份塞进本机 SQLite 索引；
// - 后续如果 Node 口径变化，这个文件也只能作为缓存，不能升级成真相源。
func saveCachedState(state *Model.RegistrationState) error {
	if state == nil || strings.TrimSpace(state.ClientID) == "" {
		return nil
	}
	path := nodeRegistrationCachePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create node registration cache dir failed: %w", err)
	}
	body, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal node registration cache failed: %w", err)
	}
	if err = os.WriteFile(path, body, 0o644); err != nil {
		return fmt.Errorf("write node registration cache failed: %w", err)
	}
	return nil
}

func validateAssignAPIKey(apiKey string) error {
	expected := ""
	if Settings.Conf.NodeRegisterConfig != nil {
		expected = strings.TrimSpace(Settings.Conf.NodeRegisterConfig.EdgeAPIKey)
	}
	if expected == "" || strings.TrimSpace(apiKey) != expected {
		return errAssignUnauthorized
	}
	return nil
}

func validateAssignRequest(request Model.AssignRequest) error {
	if strings.TrimSpace(request.ClientID) == "" {
		return fmt.Errorf("%w: clientId 不能为空", errAssignInvalidParams)
	}
	if strings.TrimSpace(request.AccountID) == "" {
		return fmt.Errorf("%w: accountId 不能为空", errAssignInvalidParams)
	}
	return nil
}

func isAssignUnauthorized(err error) bool {
	return errors.Is(err, errAssignUnauthorized)
}

func isAssignInvalidParams(err error) bool {
	return errors.Is(err, errAssignInvalidParams)
}

func nodeRegistrationCachePath() string {
	return filepath.Join(Settings.Conf.ProjectRoot, "data", "node-registration.json")
}

func normalizeURL(raw string) string {
	return strings.TrimRight(strings.TrimSpace(raw), "/")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
