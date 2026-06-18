package Edge

// DockerStatus 是当前 Client 对外暴露的本机 Docker 健康摘要。
//
// 设计来源：
// - 这条接口只服务“本机 Docker 能不能用”的快速判断；
// - 它不替代镜像列表、容器列表，也不承载平台调度或商业授权语义；
// - 当前前端和 Node 排障都需要一条立即返回的只读入口。
//
// 职责边界：
// - 只表达本机 Docker API 当前是否可访问、镜像数、容器数；
// - 不返回镜像明细，不返回容器明细；
// - 不写 SQLite，不创建 task。
type DockerStatus struct {
	DockerAPIURL    string `json:"dockerApiUrl"`
	Status          string `json:"status"`
	Message         string `json:"message"`
	ImagesCount     int64  `json:"imagesCount"`
	ContainersCount int64  `json:"containersCount"`
	CheckedAt       int64  `json:"checkedAt"`
}

// DockerImage 是 `GET /api/v1/edge/docker/images` 返回的镜像摘要。
//
// 它只保留当前阶段联调真正需要的字段，避免把 Docker 原始响应整个暴露给前端后难以演进。
type DockerImage struct {
	ID          string   `json:"id"`
	RepoTags    []string `json:"repoTags"`
	RepoDigests []string `json:"repoDigests"`
	Created     int64    `json:"created"`
	Size        int64    `json:"size"`
}

// DockerContainer 是 `GET /api/v1/edge/docker/containers` 返回的项目相关容器摘要。
//
// 当前只保留本项目关心的最小字段：
// - 容器是谁
// - 当前状态是什么
// - 属于哪个 projectRole
// - 能否识别出 slotId / envId
type DockerContainer struct {
	ID          string   `json:"id"`
	Names       []string `json:"names"`
	Image       string   `json:"image"`
	State       string   `json:"state"`
	Status      string   `json:"status"`
	ProjectRole string   `json:"projectRole"`
	SlotID      string   `json:"slotId,omitempty"`
	EnvID       string   `json:"envId,omitempty"`
}

// DockerEngineInfoResponse 对应 Docker `/info` 的最小读取模型。
//
// 这里只保留当前新 Client 真正消费的字段，避免模型变成巨大黑盒。
type DockerEngineInfoResponse struct {
	Images     int64 `json:"Images"`
	Containers int64 `json:"Containers"`
}

// DockerEngineImageResponse 对应 Docker `/images/json` 的最小读取模型。
type DockerEngineImageResponse struct {
	ID          string   `json:"Id"`
	RepoTags    []string `json:"RepoTags"`
	RepoDigests []string `json:"RepoDigests"`
	Created     int64    `json:"Created"`
	Size        int64    `json:"Size"`
}

// DockerEngineContainerResponse 对应 Docker `/containers/json?all=true` 的最小读取模型。
type DockerEngineContainerResponse struct {
	ID     string            `json:"Id"`
	Names  []string          `json:"Names"`
	Image  string            `json:"Image"`
	Labels map[string]string `json:"Labels"`
	State  string            `json:"State"`
	Status string            `json:"Status"`
}
