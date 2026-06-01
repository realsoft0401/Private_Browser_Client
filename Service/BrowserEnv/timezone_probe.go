package BrowserEnv

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	browserEnvDao "private_browser_client/Dao/BrowserEnv"
	model "private_browser_client/Models/BrowserEnv"
	edgeService "private_browser_client/Service/Edge"
)

var ianaTimezoneRe = regexp.MustCompile(`^[A-Za-z_]+/[A-Za-z0-9_+\-.]+(?:/[A-Za-z0-9_+\-.]+)?$`)

const (
	timezoneProbeMaxDuration   = 30 * time.Second
	timezoneProbeStartupWait   = 5 * time.Second
	timezoneProbeAttemptLimit  = 1
	timezoneProbeAttemptWait   = 2 * time.Second
	timezoneProxyReadyAttempts = 3
	timezoneProxyReadyWait     = 2 * time.Second
	timezoneCDPPageLoadWait    = 10 * time.Second
	timezoneCDPCommandTimeout  = 3 * time.Second
	timezoneCDPBodyReadWait    = 4 * time.Second
	timezoneCurlMaxTime        = 6 * time.Second
)

type timezoneProbeProvider struct {
	Name string
	URL  string
}

type timezoneProbeResult struct {
	Provider string
	URL      string
	ExitIP   string
	Country  string
	Region   string
	Timezone string
	Attempts []model.TimezoneProbeAttempt
}

type timezoneProbeApplyResult struct {
	Result                 *timezoneProbeResult
	TimezoneChanged        bool
	NeedsContainerRecreate bool
	ProbeFailed            bool
	Error                  string
}

type timezoneProbeError struct {
	Message  string
	Attempts []model.TimezoneProbeAttempt
}

func (e *timezoneProbeError) Error() string {
	return e.Message
}

var timezoneProbeProviders = []timezoneProbeProvider{
	{Name: "ipwho.is", URL: "https://ipwho.is"},
	{Name: "ip-api.com", URL: "http://ip-api.com/json"},
	{Name: "ipapi.co", URL: "https://ipapi.co/json/"},
}

// applyContainerTimezoneProbe 在浏览器容器内确认代理出口 timezone 并回写环境身份。
//
// 设计来源：
// - 用户确认 timezone 不能由 Go 边缘服务宿主机请求 IP 服务决定；
// - 只有浏览器容器内的 Clash/TUN/DNS/代理链路才代表账号环境真实出口；
// - timezone 又参与 identityHash，因此 run 和 running 状态代理重建后必须以容器内探测结果为准。
//
// 职责边界：
// - 只在容器已经启动后执行；
// - 代理启用时先等待 Clash/TUN 初始化，再请求固定三个 provider，避免刚启动时直连出口污染 timezone；
// - 成功后回写 profile、binding、proxy-runtime 和 manifest；
// - 失败或超时只记录 attempts，不把 run/PATCH proxy 整体拖到无响应；调用方可从响应和详情看到 timezoneStatus。
func applyContainerTimezoneProbe(pkg *runPackage, containerID string) (*timezoneProbeApplyResult, error) {
	deadline := time.Now().Add(timezoneProbeMaxDuration)
	if err := waitForContainerProxyReady(pkg, containerID, deadline); err != nil {
		return recordTimezoneProbeFailure(pkg, err.Error(), nil)
	}
	result, err := probeTimezoneInContainer(pkg, containerID, deadline)
	if err != nil {
		attempts := []model.TimezoneProbeAttempt(nil)
		if probeErr, ok := err.(*timezoneProbeError); ok {
			attempts = probeErr.Attempts
		}
		return recordTimezoneProbeFailure(pkg, err.Error(), attempts)
	}
	timezoneChanged, err := writeTimezoneProbeSuccess(pkg, result)
	if err != nil {
		_ = updateRunErrorWithRuntime(pkg, err.Error(), containerID)
		return nil, err
	}
	return &timezoneProbeApplyResult{
		Result:                 result,
		TimezoneChanged:        timezoneChanged,
		NeedsContainerRecreate: timezoneChanged,
	}, nil
}

// recordTimezoneProbeFailure 把 provider/CDP/curl 失败收敛为可返回状态。
//
// timezone 是运行保护信号，不应该因为外部 provider 慢或不可达导致 HTTP 请求无返回；
// 因此这里只写 proxy-runtime/binding 的 failed 状态，不把 browser_envs.status 改成 error。
func recordTimezoneProbeFailure(pkg *runPackage, message string, attempts []model.TimezoneProbeAttempt) (*timezoneProbeApplyResult, error) {
	if err := writeTimezoneProbeFailed(pkg, message, attempts); err != nil {
		return nil, err
	}
	return &timezoneProbeApplyResult{
		ProbeFailed: true,
		Error:       truncateRunError(message),
	}, nil
}

// waitForContainerProxyReady 给 Clash/TUN 启动留出明确窗口。
//
// 用户实测发现容器刚启动时 provider 请求可能先走直连出口，得到 Asia/Shanghai 这类错误 timezone。
// 因此代理启用时不能立刻采样；先等待容器内出现 clash/mihomo 进程，再额外等待一小段时间让路由和 DNS 接管。
func waitForContainerProxyReady(pkg *runPackage, containerID string, deadline time.Time) error {
	if pkg == nil || !pkg.Profile.Proxy.Enabled {
		return nil
	}
	edge := edgeService.NewEdgeService()
	lastErr := ""
	for i := 0; i < timezoneProxyReadyAttempts; i++ {
		if !hasTimezoneProbeBudget(deadline, timezoneProxyReadyWait) {
			return fmt.Errorf("timezone probe timeout: waiting for clash/tun ready")
		}
		if i > 0 {
			time.Sleep(minDuration(timezoneProxyReadyWait, time.Until(deadline)))
		}
		if err := execProxyReadyCheck(edge, containerID); err == nil {
			if !hasTimezoneProbeBudget(deadline, timezoneProbeStartupWait) {
				return fmt.Errorf("timezone probe timeout: no startup wait budget")
			}
			time.Sleep(minDuration(timezoneProbeStartupWait, time.Until(deadline)))
			return nil
		} else {
			lastErr = err.Error()
		}
	}
	return fmt.Errorf("timezone probe failed: clash/tun not ready: %s", truncateRunError(lastErr))
}

// probeTimezoneInContainer 在容器真实网络链路内逐个请求 provider。
//
// 设计来源：
// - 用户确认不同 Clash mode 的探测入口不能混用；
// - rule 模式需要由浏览器页面真实发起请求，才能让域名规则、进程行为和浏览器链路参与判断；
// - global/direct 模式规则分流不再是重点，应使用容器内 curl 走 Clash mixed-port 直接确认出口。
//
// 职责边界：
// - 不让宿主机代发请求；
// - rule 模式只走 CDP，不再失败后自动 curl 兜底，避免得到和浏览器规则链路不一致的 timezone；
// - global/direct 模式只走 curl/wget，并显式使用 mixed-port 进入 Clash。
func probeTimezoneInContainer(pkg *runPackage, containerID string, deadline time.Time) (*timezoneProbeResult, error) {
	attempts := make([]model.TimezoneProbeAttempt, 0, len(timezoneProbeProviders)*timezoneProbeAttemptLimit)
	edge := edgeService.NewEdgeService()
	transport := selectTimezoneProbeTransport(pkg)
	for attempt := 0; attempt < timezoneProbeAttemptLimit; attempt++ {
		if attempt > 0 {
			if !hasTimezoneProbeBudget(deadline, timezoneProbeAttemptWait) {
				break
			}
			time.Sleep(minDuration(timezoneProbeAttemptWait, time.Until(deadline)))
		}
		for _, provider := range timezoneProbeProviders {
			if !hasTimezoneProbeBudget(deadline, minTimezoneProviderBudget(transport)) {
				attempts = append(attempts, model.TimezoneProbeAttempt{
					Provider: provider.Name,
					URL:      provider.URL,
					OK:       false,
					Error:    "timezone probe timeout",
				})
				return nil, &timezoneProbeError{
					Message:  "timezone probe timeout",
					Attempts: attempts,
				}
			}
			output, err := execProbeProviderByTransport(edge, pkg, containerID, provider.URL, transport, deadline)
			if err != nil {
				attempts = append(attempts, model.TimezoneProbeAttempt{
					Provider: provider.Name,
					URL:      provider.URL,
					OK:       false,
					Error:    truncateRunError(err.Error()),
				})
				continue
			}
			parsed, err := parseTimezoneProviderResponse(provider, output)
			if err != nil {
				attempts = append(attempts, model.TimezoneProbeAttempt{
					Provider: provider.Name,
					URL:      provider.URL,
					OK:       false,
					Error:    truncateRunError(err.Error()),
				})
				continue
			}
			attempts = append(attempts, model.TimezoneProbeAttempt{
				Provider: provider.Name,
				URL:      provider.URL,
				OK:       true,
			})
			parsed.Attempts = attempts
			return parsed, nil
		}
	}
	return nil, &timezoneProbeError{
		Message:  "timezone probe failed: all providers failed",
		Attempts: attempts,
	}
}

func minTimezoneProviderBudget(transport timezoneProbeTransport) time.Duration {
	if transport == timezoneProbeTransportCDP {
		return timezoneCDPPageLoadWait + timezoneCDPCommandTimeout + time.Second
	}
	return 3 * time.Second
}

type timezoneProbeTransport string

const (
	timezoneProbeTransportCDP  timezoneProbeTransport = "cdp"
	timezoneProbeTransportCurl timezoneProbeTransport = "curl"
)

// selectTimezoneProbeTransport 根据 Clash mode 选择 timezone 探测入口。
//
// rule 模式默认使用 CDP，因为只有浏览器页面请求才能触发真实浏览器规则链路；
// global/direct 使用 curl 走 mixed-port，避免 CDP 页面行为影响出口判断。
// 如果启用了代理但 YAML 没有显式 mode，按 Clash 常见默认 rule 处理。
func selectTimezoneProbeTransport(pkg *runPackage) timezoneProbeTransport {
	if pkg == nil || !pkg.Profile.Proxy.Enabled {
		return timezoneProbeTransportCurl
	}
	switch extractClashMode(pkg.ProxyConfig) {
	case "global", "direct":
		return timezoneProbeTransportCurl
	case "rule", "":
		return timezoneProbeTransportCDP
	default:
		return timezoneProbeTransportCDP
	}
}

func execProbeProviderByTransport(edge *edgeService.Service, pkg *runPackage, containerID string, providerURL string, transport timezoneProbeTransport, deadline time.Time) (string, error) {
	if transport == timezoneProbeTransportCDP {
		return execProbeProviderViaBrowser(pkg, providerURL, deadline)
	}
	return execProbeProvider(edge, containerID, providerURL, detectClashMixedPort(pkg), deadline)
}

func execProxyReadyCheck(edge *edgeService.Service, containerID string) error {
	script := `if command -v pgrep >/dev/null 2>&1; then
  pgrep -af 'clash|mihomo|clash-verge' >/dev/null
elif command -v ps >/dev/null 2>&1; then
  ps aux 2>/dev/null | grep -E 'clash|mihomo|clash-verge' | grep -v grep >/dev/null
else
  exit 0
fi`
	result, err := edge.ExecDockerContainer(containerID, []string{"sh", "-lc", script})
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		output := strings.TrimSpace(result.Output)
		if output == "" {
			output = fmt.Sprintf("exit code %d", result.ExitCode)
		}
		return fmt.Errorf("%s", output)
	}
	return nil
}

func execProbeProviderViaBrowser(pkg *runPackage, providerURL string, deadline time.Time) (string, error) {
	if pkg == nil || pkg.Profile.Ports.CDP <= 0 {
		return "", fmt.Errorf("cdp port is empty")
	}
	if !hasTimezoneProbeBudget(deadline, timezoneCDPPageLoadWait+timezoneCDPCommandTimeout) {
		return "", fmt.Errorf("timezone probe timeout before cdp provider")
	}
	target, err := createCDPTarget(pkg.Profile.Ports.CDP)
	if err != nil {
		return "", err
	}
	defer closeCDPTarget(pkg.Profile.Ports.CDP, target.ID)

	dialer := websocket.Dialer{HandshakeTimeout: minDuration(timezoneCDPCommandTimeout, time.Until(deadline))}
	conn, _, err := dialer.Dial(target.WebSocketDebuggerURL, nil)
	if err != nil {
		return "", fmt.Errorf("cdp websocket dial failed: %w", err)
	}
	defer conn.Close()

	client := &cdpClient{conn: conn}
	if err = client.call("Page.enable", nil, minDuration(timezoneCDPCommandTimeout, time.Until(deadline)), nil); err != nil {
		return "", err
	}
	if err = client.call("Runtime.enable", nil, minDuration(timezoneCDPCommandTimeout, time.Until(deadline)), nil); err != nil {
		return "", err
	}
	if err = client.call("Page.navigate", map[string]any{"url": providerURL}, minDuration(timezoneCDPCommandTimeout, time.Until(deadline)), nil); err != nil {
		return "", err
	}
	if !hasTimezoneProbeBudget(deadline, timezoneCDPPageLoadWait) {
		return "", fmt.Errorf("timezone probe timeout before cdp page wait")
	}
	time.Sleep(timezoneCDPPageLoadWait)
	if !hasTimezoneProbeBudget(deadline, time.Second) {
		return "", fmt.Errorf("timezone probe timeout before reading cdp body")
	}
	return client.readDocumentBody(minDuration(timezoneCDPBodyReadWait, time.Until(deadline)))
}

type cdpTarget struct {
	ID                   string `json:"id"`
	WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
}

func createCDPTarget(cdpPort int) (*cdpTarget, error) {
	endpoint := publishedCDPHTTPURLForService(cdpPort, "/json/new?"+url.QueryEscape("about:blank"))
	req, err := http.NewRequest(http.MethodPut, endpoint, nil)
	if err != nil {
		return nil, err
	}
	client := http.Client{Timeout: timezoneCDPCommandTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cdp create target failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("cdp create target status: %s", resp.Status)
	}
	target := new(cdpTarget)
	if err = json.NewDecoder(resp.Body).Decode(target); err != nil {
		return nil, fmt.Errorf("cdp create target decode failed: %w", err)
	}
	if strings.TrimSpace(target.WebSocketDebuggerURL) == "" {
		return nil, fmt.Errorf("cdp websocket url is empty")
	}
	target.WebSocketDebuggerURL = rewriteCDPWebSocketURLForService(target.WebSocketDebuggerURL, cdpPort)
	return target, nil
}

func closeCDPTarget(cdpPort int, targetID string) {
	targetID = strings.TrimSpace(targetID)
	if targetID == "" {
		return
	}
	req, err := http.NewRequest(http.MethodGet, publishedCDPHTTPURLForService(cdpPort, "/json/close/"+url.PathEscape(targetID)), nil)
	if err != nil {
		return
	}
	resp, err := http.DefaultClient.Do(req)
	if err == nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
}

type cdpClient struct {
	conn   *websocket.Conn
	nextID int
}

type cdpResponse struct {
	ID     int             `json:"id"`
	Error  *cdpError       `json:"error,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
}

type cdpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (c *cdpClient) call(method string, params any, timeout time.Duration, out any) error {
	c.nextID++
	id := c.nextID
	payload := map[string]any{
		"id":     id,
		"method": method,
	}
	if params != nil {
		payload["params"] = params
	}
	if err := c.conn.WriteJSON(payload); err != nil {
		return fmt.Errorf("cdp %s write failed: %w", method, err)
	}
	deadline := time.Now().Add(timeout)
	for {
		if err := c.conn.SetReadDeadline(deadline); err != nil {
			return err
		}
		var resp cdpResponse
		if err := c.conn.ReadJSON(&resp); err != nil {
			return fmt.Errorf("cdp %s read failed: %w", method, err)
		}
		if resp.ID != id {
			continue
		}
		if resp.Error != nil {
			return fmt.Errorf("cdp %s failed: %s", method, resp.Error.Message)
		}
		if out != nil && len(resp.Result) > 0 {
			if err := json.Unmarshal(resp.Result, out); err != nil {
				return fmt.Errorf("cdp %s result decode failed: %w", method, err)
			}
		}
		return nil
	}
}

func (c *cdpClient) readDocumentBody(timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		body, err := c.evaluateString(`document.body ? document.body.innerText : ""`)
		if err == nil && strings.TrimSpace(body) != "" {
			return strings.TrimSpace(body), nil
		}
		lastErr = err
		time.Sleep(500 * time.Millisecond)
	}
	if lastErr != nil {
		return "", lastErr
	}
	return "", fmt.Errorf("browser provider body is empty")
}

func (c *cdpClient) evaluateString(expression string) (string, error) {
	var result struct {
		Result struct {
			Value string `json:"value"`
		} `json:"result"`
	}
	err := c.call("Runtime.evaluate", map[string]any{
		"expression":    expression,
		"returnByValue": true,
		"awaitPromise":  true,
	}, timezoneCDPCommandTimeout, &result)
	if err != nil {
		return "", err
	}
	return result.Result.Value, nil
}

func execProbeProvider(edge *edgeService.Service, containerID string, url string, proxyPort string, deadline time.Time) (string, error) {
	curlProxy := ""
	wgetProxyEnv := ""
	maxTimeSeconds := timezoneCurlTimeoutSeconds(deadline)
	connectTimeoutSeconds := maxTimeSeconds
	if connectTimeoutSeconds > 3 {
		connectTimeoutSeconds = 3
	}
	if proxyPort != "" {
		curlProxy = " -x " + shellQuote("http://127.0.0.1:"+proxyPort)
		wgetProxyEnv = "http_proxy=" + shellQuote("http://127.0.0.1:"+proxyPort) + " https_proxy=" + shellQuote("http://127.0.0.1:"+proxyPort) + " "
	}
	script := fmt.Sprintf(`if command -v curl >/dev/null 2>&1; then
  curl -fsSL --connect-timeout %d --max-time %d%s %s
elif command -v wget >/dev/null 2>&1; then
  %swget -qO- --timeout=%d %s
else
  echo "no curl or wget in container" >&2
  exit 127
fi`, connectTimeoutSeconds, maxTimeSeconds, curlProxy, shellQuote(url), wgetProxyEnv, maxTimeSeconds, shellQuote(url))
	result, err := edge.ExecDockerContainer(containerID, []string{"sh", "-lc", script})
	if err != nil {
		return "", err
	}
	output := strings.TrimSpace(result.Output)
	if result.ExitCode != 0 {
		if output == "" {
			output = fmt.Sprintf("exit code %d", result.ExitCode)
		}
		return "", fmt.Errorf("%s", output)
	}
	if output == "" {
		return "", fmt.Errorf("empty response")
	}
	return output, nil
}

func timezoneCurlTimeoutSeconds(deadline time.Time) int {
	remaining := minDuration(timezoneCurlMaxTime, time.Until(deadline))
	seconds := int(remaining / time.Second)
	if seconds < 2 {
		return 2
	}
	return seconds
}

func hasTimezoneProbeBudget(deadline time.Time, need time.Duration) bool {
	return time.Until(deadline) >= need
}

func minDuration(a time.Duration, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

func detectClashMixedPort(pkg *runPackage) string {
	if pkg == nil || !pkg.Profile.Proxy.Enabled {
		return ""
	}
	match := regexp.MustCompile(`(?m)^\s*mixed-port\s*:\s*([0-9]+)\s*$`).FindStringSubmatch(pkg.ProxyConfig)
	if len(match) == 2 {
		return strings.TrimSpace(match[1])
	}
	return ""
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

func parseTimezoneProviderResponse(provider timezoneProbeProvider, raw string) (*timezoneProbeResult, error) {
	switch provider.Name {
	case "ipwho.is":
		var payload struct {
			Success  bool   `json:"success"`
			IP       string `json:"ip"`
			Country  string `json:"country_code"`
			Region   string `json:"region"`
			Timezone struct {
				ID string `json:"id"`
			} `json:"timezone"`
		}
		if err := json.Unmarshal([]byte(raw), &payload); err != nil {
			return nil, fmt.Errorf("invalid json: %w", err)
		}
		if !payload.Success {
			return nil, fmt.Errorf("provider returned success=false")
		}
		return buildTimezoneProbeResult(provider, payload.IP, payload.Country, payload.Region, payload.Timezone.ID)
	case "ip-api.com":
		var payload struct {
			Status   string `json:"status"`
			Query    string `json:"query"`
			Country  string `json:"countryCode"`
			Region   string `json:"regionName"`
			Timezone string `json:"timezone"`
		}
		if err := json.Unmarshal([]byte(raw), &payload); err != nil {
			return nil, fmt.Errorf("invalid json: %w", err)
		}
		if payload.Status != "success" {
			return nil, fmt.Errorf("provider status is not success")
		}
		return buildTimezoneProbeResult(provider, payload.Query, payload.Country, payload.Region, payload.Timezone)
	case "ipapi.co":
		var payload struct {
			IP       string `json:"ip"`
			Country  string `json:"country_code"`
			Region   string `json:"region"`
			Timezone string `json:"timezone"`
		}
		if err := json.Unmarshal([]byte(raw), &payload); err != nil {
			return nil, fmt.Errorf("invalid json: %w", err)
		}
		return buildTimezoneProbeResult(provider, payload.IP, payload.Country, payload.Region, payload.Timezone)
	default:
		return nil, fmt.Errorf("unsupported provider: %s", provider.Name)
	}
}

func buildTimezoneProbeResult(provider timezoneProbeProvider, exitIP string, country string, region string, timezone string) (*timezoneProbeResult, error) {
	timezone = strings.TrimSpace(timezone)
	if timezone == "" {
		return nil, fmt.Errorf("timezone is empty")
	}
	if !ianaTimezoneRe.MatchString(timezone) {
		return nil, fmt.Errorf("timezone is not IANA format: %s", timezone)
	}
	return &timezoneProbeResult{
		Provider: provider.Name,
		URL:      provider.URL,
		ExitIP:   strings.TrimSpace(exitIP),
		Country:  strings.TrimSpace(country),
		Region:   strings.TrimSpace(region),
		Timezone: timezone,
	}, nil
}

func writeTimezoneProbeSuccess(pkg *runPackage, result *timezoneProbeResult) (bool, error) {
	now := time.Now().Unix()
	oldTimezone := strings.TrimSpace(pkg.Profile.Environment.Timezone)
	timezoneChanged := oldTimezone != result.Timezone

	pkg.Profile.Environment.Timezone = result.Timezone
	pkg.Profile.Metadata.UpdatedAt = now
	proxyHash := buildTextHash(pkg.ProxyConfig)
	identity := buildBindingIdentityFromProfile(pkg.Manifest.UserID, pkg.Profile, pkg.Manifest.Paths, proxyHash)
	identityHash, err := buildJSONHash(identity)
	if err != nil {
		return false, fmt.Errorf("计算 timezone identityHash 失败: %w", err)
	}
	if identityHash != pkg.Binding.IdentityHash {
		pkg.Binding.Version++
	}
	pkg.Binding.Identity = identity
	pkg.Binding.IdentityHash = identityHash
	pkg.Binding.ConfigHash = identityHash
	pkg.Binding.RuntimeProtection.TimezoneStatus = "verified"
	pkg.Binding.RuntimeProtection.LastCheckedAt = &now
	if timezoneChanged {
		pkg.Binding.RuntimeProtection.RuntimeDrift = boolPtr(true)
	}
	pkg.Binding.UpdatedAt = now
	pkg.Manifest.UpdatedAt = now

	source := result.Provider
	runtime := model.ProxyRuntimeFile{
		CheckedAt: &now,
		ExitIP:    optionalString(result.ExitIP),
		Region:    optionalString(result.Region),
		Country:   optionalString(result.Country),
		Timezone:  optionalString(result.Timezone),
		Source:    &source,
		Status:    "verified",
		Attempts:  result.Attempts,
		Drift:     timezoneChanged,
	}
	if err = writePackageJSON(pkg.AbsoluteEnvPath, pkg.Manifest.Paths.Profile, pkg.Profile); err != nil {
		return false, err
	}
	if err = writePackageJSON(pkg.AbsoluteEnvPath, pkg.Manifest.Paths.Binding, pkg.Binding); err != nil {
		return false, err
	}
	if err = writePackageJSON(pkg.AbsoluteEnvPath, pkg.Manifest.Paths.ProxyRuntime, runtime); err != nil {
		return false, err
	}
	return timezoneChanged, writeJSONFile(filepath.Join(pkg.AbsoluteEnvPath, "manifest.json"), pkg.Manifest)
}

func writeTimezoneProbeFailed(pkg *runPackage, message string, attempts []model.TimezoneProbeAttempt) error {
	if pkg == nil {
		return nil
	}
	now := time.Now().Unix()
	source := "container-probe"
	runtime := model.ProxyRuntimeFile{
		CheckedAt: &now,
		Source:    &source,
		Status:    "failed",
		Attempts:  attempts,
		Drift:     false,
	}
	pkg.Binding.RuntimeProtection.TimezoneStatus = "failed"
	pkg.Binding.RuntimeProtection.LastCheckedAt = &now
	pkg.Binding.UpdatedAt = now
	_ = writePackageJSON(pkg.AbsoluteEnvPath, pkg.Manifest.Paths.ProxyRuntime, runtime)
	return writePackageJSON(pkg.AbsoluteEnvPath, pkg.Manifest.Paths.Binding, pkg.Binding)
}

func updateRunErrorWithRuntime(pkg *runPackage, message string, containerID string) error {
	if pkg == nil || pkg.Index == nil {
		return nil
	}
	now := time.Now().Unix()
	lastError := truncateRunError(message)
	containerName := pkg.Container.ContainerName
	return browserEnvDao.NewRuntimeModelHandler().UpdateBrowserEnvRuntime(context.Background(), &model.BrowserEnvRuntimeUpdate{
		EnvID:           pkg.Index.EnvID,
		Status:          model.BrowserEnvStatusError,
		ContainerID:     optionalString(containerID),
		ContainerName:   optionalString(containerName),
		ContainerStatus: model.BrowserEnvStatusRunning,
		MonitorStatus:   model.BrowserEnvMonitorStatusUnknown,
		LastError:       &lastError,
		UpdatedAt:       now,
		LastStartedAt:   &now,
		LastStoppedAt:   pkg.Index.LastStoppedAt,
		LastCheckedAt:   &now,
	})
}

func boolPtr(value bool) *bool {
	return &value
}
