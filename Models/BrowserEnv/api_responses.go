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
