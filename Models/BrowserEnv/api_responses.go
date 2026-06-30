package BrowserEnv

// CreateBrowserEnvResponse 是创建环境包成功后的同步结果。
type CreateBrowserEnvResponse struct {
	EnvID        string            `json:"envId"`
	UserID       string            `json:"userId"`
	RPAType      string            `json:"rpaType"`
	EnvSequence  int               `json:"envSequence"`
	Ports        BrowserEnvPorts   `json:"ports"`
	EnvPath      string            `json:"envPath"`
	Files        map[string]string `json:"files"`
	IdentityHash string            `json:"identityHash"`
	CreatedAt    int64             `json:"createdAt"`
}

// UpdateBrowserEnvProxyResponse 是代理修改后的同步结果。
//
// 当前先返回最关键的收口字段，便于前端确认：
// - 环境是谁
// - 是否进入后台重启队列
// - runtimeProtection/proxyRuntime 是否已回到 pending
type UpdateBrowserEnvProxyResponse struct {
	EnvID                   string `json:"envId"`
	RestartQueued           bool   `json:"restartQueued"`
	RuntimeProtectionStatus string `json:"runtimeProtectionStatus"`
	ProxyRuntimeStatus      string `json:"proxyRuntimeStatus"`
}

// TaskAcceptedResponse 是正式 browser-env 长链路接口统一返回的接单结果。
type TaskAcceptedResponse struct {
	TaskID       string `json:"taskId"`
	TaskType     string `json:"taskType"`
	ResourceType string `json:"resourceType"`
	ResourceID   string `json:"resourceId"`
	EventsURL    string `json:"eventsUrl"`
}

// StopResponse 是正式 browser-env stop 接口的同步结果。
type StopResponse struct {
	EnvID           string `json:"envId"`
	Status          string `json:"status"`
	ContainerStatus string `json:"containerStatus"`
	StoppedAt       int64  `json:"stoppedAt"`
}

// DeleteBrowserEnvImageResponse 是正式 browser-env `/del` 接口的同步结果。
//
// 设计来源：
// - `/del` 只表达“本机运行镜像处理结果”，不是环境包生命周期 task；
// - 调用方最关心的是删的是哪张镜像、Docker 是否真的删掉、如果没删掉原因是什么；
// - 因此这里返回镜像删除摘要，而不把浏览器环境状态字段重新包装成第二套假状态。
type DeleteBrowserEnvImageResult struct {
	Image    string `json:"image"`
	Deleted  string `json:"deleted,omitempty"`
	Untagged string `json:"untagged,omitempty"`
}

type DeleteBrowserEnvImageResponse struct {
	EnvID          string                        `json:"envId"`
	Image          string                        `json:"image"`
	ImageRemoved   bool                          `json:"imageRemoved"`
	Results        []DeleteBrowserEnvImageResult `json:"results"`
	WarningMessage string                        `json:"warningMessage"`
	DeletedAt      int64                         `json:"deletedAt"`
}

// ListBrowserEnvResponse 是环境包列表接口响应。
//
// 列表只返回轻量摘要和统计，不返回代理明文、指纹 raw 或 browser-data 内容；
// 详情页需要更深信息时，再单独调用 detail 接口。
type ListBrowserEnvResponse struct {
	Total     int64              `json:"total"`
	Page      int                `json:"page"`
	PageSize  int                `json:"pageSize"`
	ByStatus  map[string]int64   `json:"byStatus"`
	ByRPAType map[string]int64   `json:"byRpaType"`
	Items     []*BrowserEnvIndex `json:"items"`
}
