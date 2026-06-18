package Edge

// ContainerActionRequest 是 stop/restart 这类容器动作的可选参数。
//
// 设计来源：
// - TikTok 这类真实浏览器会在 stop 前的几秒内继续把 Cookies、IndexedDB、Local Storage 刷盘；
// - 如果新 Client 只会“立刻删容器”，登录态很容易还没落盘就被打断；
// - 因此这里补回 old 已验证过的 timeoutSeconds，让上层能明确要求 Docker 做优雅停止。
//
// 职责边界：
// - 这里只表达 Docker stop/restart 的等待秒数；
// - 不承担 BrowserEnv 的业务状态，也不暴露任意 Docker 查询参数。
type ContainerActionRequest struct {
	TimeoutSeconds *int `json:"timeoutSeconds"`
}

// DockerContainerCreateConfig 是当前 Client 发送给 Docker Engine 的受控 create 请求体。
//
// 设计来源：
//   - slot 初始化阶段最早只需要镜像、端口和基本环境变量；
//   - 但正式 browser-env run 已经明确要把 browser-data/profile 挂进容器，
//     还可能按代理配置追加 TUN 相关 HostConfig；
//   - 因此这里不能继续只保留“占位容器最小字段”，否则 run 永远无法真正消费环境包资产。
//
// 职责边界：
// - 这里只暴露当前 Client 已经正式接入的受控 Docker 字段；
// - 不允许把前端或上层请求原样透传成任意 Docker HostConfig；
// - 后续若新增运行能力，应继续在受控字段里逐项补，不回退成黑盒透传。
type DockerContainerCreateConfig struct {
	Image        string                    `json:"Image"`
	Cmd          []string                  `json:"Cmd,omitempty"`
	Env          []string                  `json:"Env,omitempty"`
	Labels       map[string]string         `json:"Labels,omitempty"`
	ExposedPorts map[string]struct{}       `json:"ExposedPorts,omitempty"`
	HostConfig   DockerContainerHostConfig `json:"HostConfig,omitempty"`
}

type DockerContainerHostConfig struct {
	Binds         []string                       `json:"Binds,omitempty"`
	RestartPolicy DockerContainerRestartPolicy   `json:"RestartPolicy,omitempty"`
	PortBindings  map[string][]DockerPortBinding `json:"PortBindings,omitempty"`
	ShmSize       int64                          `json:"ShmSize,omitempty"`
	SecurityOpt   []string                       `json:"SecurityOpt,omitempty"`
	CapAdd        []string                       `json:"CapAdd,omitempty"`
	Devices       []DockerContainerDeviceMapping `json:"Devices,omitempty"`
}

type DockerContainerRestartPolicy struct {
	Name string `json:"Name"`
}

type DockerPortBinding struct {
	HostIP   string `json:"HostIp,omitempty"`
	HostPort string `json:"HostPort"`
}

// DockerContainerDeviceMapping 描述 Docker HostConfig.Devices 的单条设备映射。
//
// 当前只用于受控追加 `/dev/net/tun` 这类浏览器运行必须的宿主机设备，
// 不能演变成任意设备透传能力。
type DockerContainerDeviceMapping struct {
	PathOnHost        string `json:"PathOnHost"`
	PathInContainer   string `json:"PathInContainer"`
	CgroupPermissions string `json:"CgroupPermissions,omitempty"`
}

type DockerContainerCreateResult struct {
	ID       string   `json:"Id"`
	Warnings []string `json:"Warnings,omitempty"`
}

type ContainerActionResult struct {
	ContainerID string `json:"containerId"`
	Action      string `json:"action"`
	Status      string `json:"status"`
	Message     string `json:"message"`
	CheckedAt   int64  `json:"checkedAt"`
}

type DockerPullEvent struct {
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

// PullImageRequest 是 `POST /api/v1/edge/docker/pull-image` 的请求体。
//
// 当前先只收一条完整 image 引用，不再额外拆 tag，避免前端和 Node 联调时出现两套镜像表达方式。
type PullImageRequest struct {
	Image string `json:"image"`
}

// RemoveImageRequest 是 `POST /api/v1/edge/docker/remove-image` 的请求体。
//
// 这条接口只作用于本机 Docker 镜像，不删除环境包目录，也不修复业务引用关系。
type RemoveImageRequest struct {
	Image   string `json:"image"`
	Force   bool   `json:"force"`
	NoPrune bool   `json:"noPrune"`
}

// DockerImageRemoveResult 对应 Docker remove image 返回的一条结果摘要。
type DockerImageRemoveResult struct {
	Deleted  string `json:"Deleted,omitempty"`
	Untagged string `json:"Untagged,omitempty"`
}
