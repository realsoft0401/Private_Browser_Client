package Edge

// DeviceInfo 是当前新 Client 对外暴露的最小设备信息模型。
//
// 当前阶段先保留最小本机事实：
// - 操作系统
// - 架构
// - Docker API 地址
// - 发现模式
//
// 后续真正接入 Docker 探测后，再在这个模型里补更多本机能力摘要。
//
// 不能退回的边界：
//   - 这里是 Edge 本机事实接口，不承载 Node Server 的中心身份；
//   - `clientId` 由 Node Server 维护，不能再从这个本机事实模型里回传，
//     否则会把 Client 重新做成“自己持有中心身份”的服务。
type DeviceInfo struct {
	OS            string `json:"os"`
	DeviceArch    string `json:"arch"`
	DockerAPIURL  string `json:"dockerApiUrl"`
	DiscoveryMode string `json:"discoveryMode"`
}
