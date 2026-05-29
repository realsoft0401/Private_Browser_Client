package Edge

// ArchAMD64 表示常见 x86_64 / AMD64 设备架构。
//
// 边缘服务后续选择镜像时必须使用归一化后的架构值，避免业务代码到处判断 x86_64、amd64 等原始写法。
const ArchAMD64 = "amd64"

// ArchARM64 表示 ARM64 / AArch64 设备架构。
//
// Apple Silicon、ARM 服务器、部分开发板都归一化到这个值。
const ArchARM64 = "arm64"

// ArchUnknown 表示 Docker API 未返回架构，或当前服务暂时无法识别。
//
// 架构未知时，后续拉取架构相关镜像应返回明确错误，不要盲目启动容器。
const ArchUnknown = "unknown"

// DeviceInfo 是边缘服务返回的本机设备能力信息。
//
// 设计来源：
// - `Private_Browser_Client` 已重新定位为边缘服务；
// - 这个模型只表达本机 Docker 2375 暴露的设备数据，不包含用户、节点归属或商业化设备编号；
// - 后续中心服务端如需保存这些数据，应由中心服务端拉取或接收上报后自行落库。
type DeviceInfo struct {
	DeviceIP            string `json:"deviceIp"`
	DockerAPIURL        string `json:"dockerApiUrl"`
	DeviceOS            string `json:"deviceOs"`
	DeviceArch          string `json:"deviceArch"`
	DeviceRawArch       string `json:"deviceRawArch"`
	CPUCores            int    `json:"cpuCores"`
	MemoryTotalBytes    int64  `json:"memoryTotalBytes"`
	DockerVersion       string `json:"dockerVersion"`
	DockerAPIVersion    string `json:"dockerApiVersion"`
	ComposeVersion      string `json:"composeVersion"`
	LastDockerStatus    string `json:"lastDockerStatus"`
	LastDockerMessage   string `json:"lastDockerMessage"`
	LastImagesCount     int64  `json:"lastImagesCount"`
	LastContainersCount int64  `json:"lastContainersCount"`
	CheckedAt           int64  `json:"checkedAt"`
}

// DockerStatus 是边缘服务返回的本机 Docker 状态摘要。
//
// 它只表达 Docker daemon 是否可用、镜像数量和容器数量，不承担镜像列表或容器列表职责。
// 后续需要完整列表时应新增 images / containers 接口，而不是让这个状态接口变重。
type DockerStatus struct {
	DockerAPIURL    string `json:"dockerApiUrl"`
	Status          string `json:"status"`
	Message         string `json:"message"`
	ImagesCount     int64  `json:"imagesCount"`
	ContainersCount int64  `json:"containersCount"`
	CheckedAt       int64  `json:"checkedAt"`
}

// DockerImage 是边缘服务返回给前端或中心端的本机镜像摘要。
//
// 设计边界：
// - 它来自 Docker Engine API `/images/json`；
// - 只保留镜像 ID、标签、摘要、创建时间、大小和关联容器数量；
// - 不直接透出 Docker 原始完整对象，避免后续前端依赖 Docker 内部字段导致接口难以演进。
type DockerImage struct {
	ID          string   `json:"id"`
	RepoTags    []string `json:"repoTags"`
	RepoDigests []string `json:"repoDigests"`
	Created     int64    `json:"created"`
	Size        int64    `json:"size"`
	VirtualSize int64    `json:"virtualSize"`
	SharedSize  int64    `json:"sharedSize"`
	Containers  int64    `json:"containers"`
}

// DockerContainer 是边缘服务返回给前端或中心端的项目相关容器摘要。
//
// 这个模型来自 Docker Engine API `/containers/json?all=true`。
// 当前接口只暴露 Private_Browser_Client 相关容器，不再把本机 Docker 的无关业务容器混进来。
// projectRole 用于区分边缘服务自身容器和浏览器环境容器，方便前端按类型展示和操作。
type DockerContainer struct {
	ID          string                `json:"id"`
	Names       []string              `json:"names"`
	Image       string                `json:"image"`
	ImageID     string                `json:"imageId"`
	Command     string                `json:"command"`
	Created     int64                 `json:"created"`
	Ports       []DockerContainerPort `json:"ports"`
	Labels      map[string]string     `json:"labels"`
	State       string                `json:"state"`
	Status      string                `json:"status"`
	ProjectRole string                `json:"projectRole"`
	EnvID       string                `json:"envId,omitempty"`
	UserID      string                `json:"userId,omitempty"`
	RPAType     string                `json:"rpaType,omitempty"`
}

// DockerContainerPort 表示容器端口映射摘要。
//
// Docker 原始字段使用 PrivatePort / PublicPort / Type，这里转换为前端更稳定的小驼峰命名。
type DockerContainerPort struct {
	IP          string `json:"ip"`
	PrivatePort int    `json:"privatePort"`
	PublicPort  int    `json:"publicPort"`
	Type        string `json:"type"`
}

// PullImageRequest 是拉取本机 Docker 镜像的请求体。
//
// image 支持完整镜像名，例如 `alpine:latest` 或 `registry.example.com/ns/browser:arm64`。
// tag 可选；如果 image 已经带 tag，Service 会自动拆分，避免调用方必须理解 Docker API 的 fromImage/tag 参数。
type PullImageRequest struct {
	Image string `json:"image"`
	Tag   string `json:"tag"`
}

// RemoveImageRequest 是删除本机 Docker 镜像的请求体。
//
// image 可以是镜像 ID、repo:tag 或 digest；force 只在明确需要强制删除时传 true。
// 这个动作会改变本机 Docker 状态，所以它只属于边缘服务本机管理能力，不涉及中心端数据库。
type RemoveImageRequest struct {
	Image   string `json:"image"`
	Force   bool   `json:"force"`
	NoPrune bool   `json:"noPrune"`
}

// ContainerActionRequest 是停止或重启容器时的可选请求参数。
//
// 设计来源：
// - Docker Engine API 的 stop / restart 支持 `t` 参数控制等待秒数；
// - 边缘服务需要给前端一个稳定的小驼峰字段，而不是暴露 Docker 原始查询参数；
// - start 动作不需要这个参数，stop / restart 不传时由 Service 使用保守默认值。
type ContainerActionRequest struct {
	TimeoutSeconds *int `json:"timeoutSeconds"`
}

// ContainerActionResult 是容器生命周期动作的统一响应。
//
// 它只表达本次边缘服务对本机 Docker 的执行结果，不写数据库，也不承诺容器业务状态。
// 如果 Docker 返回 304，status 会是 not-modified，用于表示容器本来就处在目标状态。
type ContainerActionResult struct {
	ContainerID string `json:"containerId"`
	Action      string `json:"action"`
	Status      string `json:"status"`
	Message     string `json:"message"`
	CheckedAt   int64  `json:"checkedAt"`
}

// DockerContainerCreateConfig 是边缘服务生成的 Docker create 请求体。
//
// 设计来源：
// - BrowserEnv.run 会把环境包转换为 Docker 参数，但底层 Docker API 仍由 Edge 服务统一调用；
// - 这里使用 Docker Engine API 的字段名大小写，避免在 Service 里拼 map[string]any 形成黑盒结构。
//
// 职责边界：
// - 只表达创建容器需要的最小字段；
// - 不承载环境包业务含义，不负责读取 profile/binding/proxy 文件；
// - 后续如果镜像需要更多 Linux 权限，优先扩展 HostConfig，而不是让前端透传任意 Docker 配置。
type DockerContainerCreateConfig struct {
	Image        string                         `json:"Image"`
	Env          []string                       `json:"Env,omitempty"`
	Labels       map[string]string              `json:"Labels,omitempty"`
	ExposedPorts map[string]struct{}            `json:"ExposedPorts,omitempty"`
	HostConfig   DockerContainerHostConfig      `json:"HostConfig"`
	Networking   *DockerContainerNetworkingSpec `json:"NetworkingConfig,omitempty"`
}

// DockerContainerHostConfig 描述 Docker 容器的宿主机侧配置。
type DockerContainerHostConfig struct {
	Binds         []string                       `json:"Binds,omitempty"`
	PortBindings  map[string][]DockerPortBinding `json:"PortBindings,omitempty"`
	RestartPolicy DockerContainerRestartPolicy   `json:"RestartPolicy,omitempty"`
	ShmSize       int64                          `json:"ShmSize,omitempty"`
	CapAdd        []string                       `json:"CapAdd,omitempty"`
	Devices       []DockerContainerDeviceMapping `json:"Devices,omitempty"`
	// SecurityOpt 保存 Docker 安全配置。
	//
	// 设计来源：
	// - Private_Browser_Control 旧 compose 容器使用 seccomp:unconfined；
	// - Chromium 在 Docker Desktop / 部分 Linux 节点中如果没有可用 sandbox，会直接退出并触发容器反复重启；
	// - Go run 生成 Docker create 参数时必须保留这个历史约束，不能只看镜像和端口。
	SecurityOpt []string `json:"SecurityOpt,omitempty"`
}

// DockerContainerRestartPolicy 描述容器重启策略。
type DockerContainerRestartPolicy struct {
	Name string `json:"Name"`
}

// DockerPortBinding 描述单个容器端口绑定到宿主机端口。
type DockerPortBinding struct {
	HostIP   string `json:"HostIp,omitempty"`
	HostPort string `json:"HostPort"`
}

// DockerContainerDeviceMapping 描述宿主机设备映射。
type DockerContainerDeviceMapping struct {
	PathOnHost        string `json:"PathOnHost"`
	PathInContainer   string `json:"PathInContainer"`
	CgroupPermissions string `json:"CgroupPermissions"`
}

// DockerContainerNetworkingSpec 预留网络配置。
//
// 第一版不主动设置复杂网络；保留结构是为了后续如果要接自定义 bridge，
// 可以继续由边缘服务生成受控配置，而不是让前端透传 Docker 原始对象。
type DockerContainerNetworkingSpec struct {
	EndpointsConfig map[string]any `json:"EndpointsConfig,omitempty"`
}

// DockerContainerCreateResult 是 Docker create 成功后的摘要。
type DockerContainerCreateResult struct {
	ID       string   `json:"Id"`
	Warnings []string `json:"Warnings,omitempty"`
}

// DockerPullEvent 是 Docker `/images/create` 流式响应的一条事件。
//
// Docker 拉取镜像会返回多行 JSON，这里保留常用状态字段，方便前端展示拉取进度和错误原因。
type DockerPullEvent struct {
	Status         string              `json:"status"`
	ID             string              `json:"id,omitempty"`
	Progress       string              `json:"progress,omitempty"`
	ProgressDetail *DockerProgressInfo `json:"progressDetail,omitempty"`
	Error          string              `json:"error,omitempty"`
}

// DockerProgressInfo 表示 Docker 拉取镜像的进度详情。
type DockerProgressInfo struct {
	Current int64 `json:"current"`
	Total   int64 `json:"total"`
}

// DockerImageRemoveResult 是 Docker 删除镜像后的结果摘要。
//
// Docker 可能返回 Untagged 或 Deleted，两者都保留，便于排查只是解除标签还是删除了实际层。
type DockerImageRemoveResult struct {
	Untagged string `json:"untagged,omitempty"`
	Deleted  string `json:"deleted,omitempty"`
}

// DockerEngineInfoResponse 是 Docker Engine API `/info` 的部分响应模型。
//
// 这里只保留边缘服务当前需要的字段，避免把 Docker 原始大对象直接暴露给前端或中心服务端。
type DockerEngineInfoResponse struct {
	Architecture    string `json:"Architecture"`
	OSType          string `json:"OSType"`
	OperatingSystem string `json:"OperatingSystem"`
	NCPU            int    `json:"NCPU"`
	MemTotal        int64  `json:"MemTotal"`
	ServerVersion   string `json:"ServerVersion"`
	Images          int64  `json:"Images"`
	Containers      int64  `json:"Containers"`
}

// DockerEngineVersionResponse 是 Docker Engine API `/version` 的部分响应模型。
//
// `/info` 更偏运行态，`/version` 更偏 Docker 版本和 API 元数据，两者组合后才能给中心端稳定判断依据。
type DockerEngineVersionResponse struct {
	Version    string `json:"Version"`
	APIVersion string `json:"ApiVersion"`
	OS         string `json:"Os"`
	Arch       string `json:"Arch"`
}

// DockerEngineImageResponse 是 Docker Engine API `/images/json` 的部分响应模型。
//
// Docker 返回字段首字母大写，这个内部模型只用于解码，外部响应统一转换为 DockerImage。
type DockerEngineImageResponse struct {
	ID          string   `json:"Id"`
	RepoTags    []string `json:"RepoTags"`
	RepoDigests []string `json:"RepoDigests"`
	Created     int64    `json:"Created"`
	Size        int64    `json:"Size"`
	VirtualSize int64    `json:"VirtualSize"`
	SharedSize  int64    `json:"SharedSize"`
	Containers  int64    `json:"Containers"`
}

// DockerEngineContainerResponse 是 Docker Engine API `/containers/json` 的部分响应模型。
//
// 它只用于内部解码，避免把 Docker 原始字段名直接扩散到边缘服务对外 API。
type DockerEngineContainerResponse struct {
	ID      string                              `json:"Id"`
	Names   []string                            `json:"Names"`
	Image   string                              `json:"Image"`
	ImageID string                              `json:"ImageID"`
	Command string                              `json:"Command"`
	Created int64                               `json:"Created"`
	Ports   []DockerEngineContainerPortResponse `json:"Ports"`
	Labels  map[string]string                   `json:"Labels"`
	State   string                              `json:"State"`
	Status  string                              `json:"Status"`
}

// DockerEngineContainerPortResponse 是 Docker 原始端口映射解码模型。
type DockerEngineContainerPortResponse struct {
	IP          string `json:"IP"`
	PrivatePort int    `json:"PrivatePort"`
	PublicPort  int    `json:"PublicPort"`
	Type        string `json:"Type"`
}
