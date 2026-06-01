package BrowserEnv

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	model "private_browser_client/Models/BrowserEnv"
)

const cdpBasicTestTimeout = 3 * time.Second

// TestBrowserEnvCDP 执行最小 CDP 连通性诊断。
//
// 设计来源：
// - 用户在 run / timezone / VNC 排障后，需要一个独立的基础程序判断 CDP 端口到底能不能用；
// - timezone probe 依赖 CDP，但它还混入 provider、代理模式和页面加载等待，排查时不够纯粹；
// - 因此这里只做 CDP 自身最小闭环：/json/version -> /json/new -> WebSocket -> Runtime.evaluate。
//
// 职责边界：
// - 端口只从 browser_envs 索引读取，不允许调用方传任意端口，避免这个诊断接口变成端口扫描工具；
// - 诊断步骤失败时返回 ok=false + stage/error，而不是直接抛 remoteError，方便前端展示失败位置；
// - 环境包不存在、未运行、未分配 CDP 端口属于前置业务错误，仍交给统一错误响应处理。
func (s *Service) TestBrowserEnvCDP(envID string) (*model.BrowserEnvCDPTestResponse, error) {
	index, err := getRuntimeIndex(envID)
	if err != nil {
		return nil, err
	}
	if index.CDPPort <= 0 {
		return nil, conflictError("环境包未分配 CDP 端口")
	}
	if index.Status != model.BrowserEnvStatusRunning {
		return nil, conflictError("环境包未运行，不能测试 CDP")
	}

	startedAt := time.Now()
	result := &model.BrowserEnvCDPTestResponse{
		EnvID:    index.EnvID,
		CDPPort:  index.CDPPort,
		Endpoint: publishedCDPHTTPURLForService(index.CDPPort, "/"),
		Stage:    "start",
		TestedAt: startedAt.Unix(),
	}
	defer func() {
		result.DurationMs = time.Since(startedAt).Milliseconds()
	}()

	version, err := fetchCDPVersion(index.CDPPort)
	if err != nil {
		return markCDPTestFailed(result, "http_version", err), nil
	}
	result.Browser = version.Browser
	result.ProtocolVersion = version.ProtocolVersion

	target, err := createCDPTarget(index.CDPPort)
	if err != nil {
		return markCDPTestFailed(result, "create_target", err), nil
	}
	result.WebSocketURL = target.WebSocketDebuggerURL
	defer closeCDPTarget(index.CDPPort, target.ID)

	dialer := websocket.Dialer{HandshakeTimeout: cdpBasicTestTimeout}
	conn, _, err := dialer.Dial(target.WebSocketDebuggerURL, nil)
	if err != nil {
		return markCDPTestFailed(result, "websocket", fmt.Errorf("cdp websocket dial failed: %w", err)), nil
	}
	defer conn.Close()

	client := &cdpClient{conn: conn}
	if err = client.call("Runtime.enable", nil, cdpBasicTestTimeout, nil); err != nil {
		return markCDPTestFailed(result, "runtime_enable", err), nil
	}
	runtimeResult, err := client.evaluateString(`"cdp-ok"`)
	if err != nil {
		return markCDPTestFailed(result, "runtime_evaluate", err), nil
	}
	result.OK = runtimeResult == "cdp-ok"
	result.Stage = "done"
	result.RuntimeResult = runtimeResult
	if !result.OK {
		result.Error = "Runtime.evaluate 返回值不符合预期"
	}
	return result, nil
}

type cdpVersionInfo struct {
	Browser         string `json:"Browser"`
	ProtocolVersion string `json:"Protocol-Version"`
}

func fetchCDPVersion(cdpPort int) (*cdpVersionInfo, error) {
	endpoint := publishedCDPHTTPURLForService(cdpPort, "/json/version")
	client := http.Client{Timeout: cdpBasicTestTimeout}
	resp, err := client.Get(endpoint)
	if err != nil {
		return nil, fmt.Errorf("cdp /json/version failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("cdp /json/version status: %s", resp.Status)
	}
	version := new(cdpVersionInfo)
	if err = json.NewDecoder(resp.Body).Decode(version); err != nil {
		return nil, fmt.Errorf("cdp /json/version decode failed: %w", err)
	}
	if strings.TrimSpace(version.Browser) == "" {
		return nil, fmt.Errorf("cdp /json/version browser is empty")
	}
	return version, nil
}

func markCDPTestFailed(result *model.BrowserEnvCDPTestResponse, stage string, err error) *model.BrowserEnvCDPTestResponse {
	result.OK = false
	result.Stage = stage
	if err != nil {
		result.Error = truncateRunError(err.Error())
	}
	return result
}
