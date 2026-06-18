package BrowserEnv

// CreateBrowserEnvRequest 是正式 browser-env 创建接口的请求体。
//
// 这条接口只接收中心或上层业务下发的稳定配置。
// envId、bindingId、端口、路径和本机序号全部由 Client 在本地生成并落盘。
type CreateBrowserEnvRequest struct {
	UserID      string                      `json:"userId"`
	RPAType     string                      `json:"rpaType"`
	Name        string                      `json:"name"`
	Runtime     CreateBrowserEnvRuntime     `json:"runtime"`
	Environment CreateBrowserEnvEnvironment `json:"environment"`
	Proxy       CreateBrowserEnvProxy       `json:"proxy"`
}

type CreateBrowserEnvRuntime struct {
	Image      string `json:"image"`
	StartupURL string `json:"startupUrl"`
	ShmSize    string `json:"shmSize"`
}

type CreateBrowserEnvEnvironment struct {
	Timezone string                 `json:"timezone"`
	Language string                 `json:"language"`
	Screen   CreateBrowserEnvScreen `json:"screen"`
}

type CreateBrowserEnvScreen struct {
	Width  int `json:"width"`
	Height int `json:"height"`
	Depth  int `json:"depth"`
}

// CreateBrowserEnvProxy 只允许上层传入代理摘要和 YAML Base64。
//
// Config 是服务层解码后的内部值，不进入正式 HTTP 协议。
type CreateBrowserEnvProxy struct {
	Enabled      *bool  `json:"enabled"`
	Type         string `json:"type"`
	ConfigBase64 string `json:"configBase64"`
	Config       string `json:"-"`
}

// UpdateBrowserEnvProxyRequest 是正式代理修改接口的请求体。
//
// 当前按已经收口的协议保留四个字段：
// - enabled
// - type
// - mode
// - configBase64
// 其中 mode 只改 Clash 顶层 mode。
type UpdateBrowserEnvProxyRequest struct {
	Enabled      *bool   `json:"enabled"`
	Type         *string `json:"type"`
	Mode         *string `json:"mode"`
	ConfigBase64 *string `json:"configBase64"`
}

// RunRequest 是正式 browser-env run 接口的请求体。
//
// 这次先严格按文档收口，只接受 slotId 和 forceRecreate；
// image、proxy、fingerprint 等运行关键配置仍然只能来自环境包资产本身。
type RunRequest struct {
	SlotID        string `json:"slotId"`
	ForceRecreate bool   `json:"forceRecreate"`
}

// StopRequest 是正式 browser-env stop 接口的请求体。
//
// 当前 stop 仍然是短链路同步动作，因此这里只保留 Docker stop 等待秒数。
type StopRequest struct {
	TimeoutSeconds int `json:"timeoutSeconds"`
}

// ListBrowserEnvQuery 是查询本机环境包索引列表的内部参数。
//
// 这组参数只服务 SQLite 列表和统计，不直接参与任何环境包资产写入；
// page/pageSize 在 Service 层统一清洗，避免前端和 Node Server 各自实现一套分页口径。
type ListBrowserEnvQuery struct {
	UserID   string
	RPAType  string
	Status   string
	Page     int
	PageSize int
}
