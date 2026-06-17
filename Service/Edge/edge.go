package Edge

import (
	"net/http"
	"runtime"
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

// GetDeviceInfo 返回当前新 Client 的最小本机设备信息。
//
// 当前阶段先保持最小事实模型，让 `/api/v1/edge/device-info` 有稳定返回。
// 后续真正接入 Docker 探测后，再把 Docker 版本、镜像数、容器数等事实补到这里。
//
// 不能退回的原则：
//   - 这里只能返回 Edge 本机事实；
//   - 不能再把 Node Server 分配的 `clientId` 塞回来，否则会破坏 project.md
//     明确要求的“Client 不生成、不保存、不要求请求携带 clientId”边界。
func (s *Service) GetDeviceInfo() (*model.DeviceInfo, error) {
	clientIP := DiscoveryService.CurrentAdvertiseHost()
	return &model.DeviceInfo{
		OS:            runtime.GOOS,
		DeviceArch:    runtime.GOARCH,
		DockerAPIURL:  resolveNodeVisibleDockerAPIURL(clientIP),
		DiscoveryMode: "independent-intranet",
	}, nil
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
