package Edge

import (
	"bufio"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	model "private_browser_client/Models/Edge"
	DiscoveryService "private_browser_client/Service/Discovery"
	"private_browser_client/Settings"
)

type Service struct {
	httpClient *http.Client
}

func NewEdgeService() *Service {
	return &Service{
		httpClient: &http.Client{Timeout: 45 * time.Second},
	}
}

// GetDeviceInfo 返回当前 Client 的本机设备摘要。
//
// 设计来源：
//   - Node Server 绑定和发现阶段都要依赖 `/api/v1/edge/device-info` 把宿主机摘要落进中心库；
//   - 之前这里只返回骨架字段，导致 Server 里的 `cpu_cores / memory_total_mb / docker_version`
//     长期都是默认空值；
//   - 这次按正式设备摘要口径补齐，但仍坚持“只返回本机事实，不承载中心身份”。
//
// 不能退回的原则：
//   - 这里只能返回 Edge 本机事实；
//   - 不能再把 Node Server 分配的 `clientId` 塞回来，否则会破坏 project.md
//     明确要求的“Client 不生成、不保存、不要求请求携带 clientId”边界。
func (s *Service) GetDeviceInfo() (*model.DeviceInfo, error) {
	clientIP := DiscoveryService.CurrentAdvertiseHost()
	dockerVersion, _ := s.getDockerVersion()
	memoryTotalMB, _ := detectSystemMemoryMB()
	return &model.DeviceInfo{
		OS:            runtime.GOOS,
		DeviceArch:    runtime.GOARCH,
		CPUCores:      int64(runtime.NumCPU()),
		MemoryTotalMB: memoryTotalMB,
		DockerAPIURL:  resolveNodeVisibleDockerAPIURL(clientIP),
		DockerVersion: dockerVersion,
		DiscoveryMode: "independent-intranet",
	}, nil
}

// getDockerVersion 读取本机 Docker Engine 版本。
//
// 职责边界：
// - 这里只做设备摘要所需的只读探测；
// - Docker 不可达时不让整个 device-info 失败，而是把版本留空；
// - 真正的 Docker 可用性判断仍由 docker/status 和运行动作链路负责。
func (s *Service) getDockerVersion() (string, error) {
	dockerAPIURL := currentDockerAPIURL()
	if strings.TrimSpace(dockerAPIURL) == "" {
		return "", nil
	}
	response := new(model.DockerEngineVersionResponse)
	if err := s.fetchJSON(dockerAPIURL, http.MethodGet, "/version", nil, response); err != nil {
		return "", err
	}
	return strings.TrimSpace(response.Version), nil
}

// detectSystemMemoryMB 返回宿主机总内存，单位 MB。
//
// 设计来源：
// - Node 中心设备表需要保留最小硬件摘要，便于后续排障和节点比对；
// - 当前项目不额外引入系统探测第三方依赖，先用标准库覆盖 Linux 和 macOS；
// - 其余平台暂时返回 0，避免把 discovery/bind 主链路做成强依赖。
func detectSystemMemoryMB() (int64, error) {
	switch runtime.GOOS {
	case "linux":
		return detectLinuxMemoryMB()
	case "darwin":
		return detectDarwinMemoryMB()
	default:
		return 0, nil
	}
}

func detectLinuxMemoryMB() (int64, error) {
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "MemTotal:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			break
		}
		valueKB, parseErr := strconv.ParseInt(fields[1], 10, 64)
		if parseErr != nil {
			return 0, parseErr
		}
		return valueKB / 1024, nil
	}
	if err = scanner.Err(); err != nil {
		return 0, err
	}
	return 0, nil
}

func detectDarwinMemoryMB() (int64, error) {
	output, err := exec.Command("sysctl", "-n", "hw.memsize").Output()
	if err != nil {
		return 0, err
	}
	valueBytes, err := strconv.ParseInt(strings.TrimSpace(string(output)), 10, 64)
	if err != nil {
		return 0, err
	}
	return valueBytes / 1024 / 1024, nil
}

// resolveNodeVisibleDockerAPIURL 把本机 Docker API 地址翻译成 Node 可理解的入口。
//
// 设计来源：
// - Client 配置里常用 `127.0.0.1:2375` 访问本机 Docker；
// - 但 Node Server 通过 HTTP 探测 Client 时，如果把这个值原样记到中心，就会得到一个只对 Client 自己有效的地址；
// - 因此这里沿用 node-registration 的同一口径：若配置是 localhost/0.0.0.0，则改写成当前对外 advertise 的 clientIp。
func resolveNodeVisibleDockerAPIURL(clientIP string) string {
	raw := Settings.Conf.DockerConfig.APIURL
	if clientIP == "" {
		return raw
	}
	switch raw {
	case "http://127.0.0.1:2375", "http://localhost:2375", "http://0.0.0.0:2375":
		return "http://" + clientIP + ":2375"
	default:
		return raw
	}
}
