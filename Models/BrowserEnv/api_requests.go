package BrowserEnv

import "encoding/json"

// API 请求模型单独拆出，是因为它们服务 HTTP 协议边界，而不是环境包落盘事实。
// 后续如果 Edge API 演进，请优先在这一组文件里调整，而不是去碰 profile/binding/container 结构。

// CreateBrowserEnvRequest 是创建本地浏览器环境包的请求体。
//
// 这个接口只接收服务端下发的稳定配置，不接收 envId、端口、bindingId、containerId 等本机生成字段。
// 这样做是为了让边缘服务拥有本地事实来源，后续 run/import/cloud sync 都围绕环境包执行。
type CreateBrowserEnvRequest struct {
	UserID      string                    `json:"userId"`
	RPAType     string                    `json:"rpaType"`
	Name        string                    `json:"name"`
	Runtime     CreateRuntimeRequest      `json:"runtime"`
	Environment CreateEnvironmentRequest  `json:"environment"`
	Proxy       CreateProxyRequest        `json:"proxy"`
	Fingerprint *CreateFingerprintRequest `json:"fingerprint"`
	Metadata    CreateMetadataRequest     `json:"metadata"`
}

// CreateRuntimeRequest 是创建环境包时允许服务端传入的运行配置。
//
// 注意 enableVnc、ports、containerName 都不在这里；它们由边缘服务按本机规则生成。
type CreateRuntimeRequest struct {
	Image      string `json:"image"`
	StartupURL string `json:"startupUrl"`
	ShmSize    string `json:"shmSize"`
}

// CreateEnvironmentRequest 描述浏览器稳定环境参数。
//
// timezone/language/screen 会参与 binding identity 计算，修改它们应视为环境身份变化。
type CreateEnvironmentRequest struct {
	Timezone string              `json:"timezone"`
	Language string              `json:"language"`
	Screen   CreateScreenRequest `json:"screen"`
}

// CreateScreenRequest 描述浏览器屏幕参数。
type CreateScreenRequest struct {
	Width  int `json:"width"`
	Height int `json:"height"`
	Depth  int `json:"depth"`
}

// CreateProxyRequest 描述创建环境包时的代理配置。
//
// 设计来源：
// - 用户测试发现长 YAML 直接作为 JSON 字符串传输时容易被工具截断或转义错；
// - 当前项目是新开发，不需要兼容旧的明文 config 入参；
// - 因此创建环境包和后续 PATCH 代理配置统一使用 configBase64。
//
// 职责边界：
// - ConfigBase64 是 API 入参，表示代理 YAML 原文的 Base64；
// - Config 是 Service 解码后的内部值，只用于写入 proxy/clash.yaml，不参与 JSON 协议。
type CreateProxyRequest struct {
	Enabled      *bool  `json:"enabled"`
	Type         string `json:"type"`
	Mode         string `json:"mode"`
	ConfigBase64 string `json:"configBase64"`
	Config       string `json:"-"`
}

// CreateFingerprintRequest 预留创建时导入正式指纹备份的入口。
//
// 第一版允许不传；如果未来服务端已经有云端指纹备份，可以在这里带入，
// 边缘服务会把它落到 fingerprint/backup.json 与 runtime-config.json。
type CreateFingerprintRequest struct {
	Backup *CreateFingerprintBackupRequest `json:"backup"`
}

// CreateFingerprintBackupRequest 表示可选的正式指纹备份。
type CreateFingerprintBackupRequest struct {
	Available   bool                         `json:"available"`
	Fingerprint *RestorableFingerprintConfig `json:"fingerprint"`
	Raw         json.RawMessage              `json:"raw"`
}

// CreateMetadataRequest 保存创建环境包时的轻量说明。
//
// source 由边缘服务默认补 api；description 仅用于展示和排障，不参与 identityHash。
type CreateMetadataRequest struct {
	Source      string `json:"source"`
	Description string `json:"description"`
}

// ListBrowserEnvQuery 是查询本机环境包索引列表的内部参数。
//
// 设计来源：
// - 用户要求边缘服务能直接回答“本机管理了多少配置文件”；
// - 查询来源是 SQLite browser_envs 索引表，不再每次扫描环境包目录；
// - 默认排除 deleted，只有显式传 status=deleted 才看回收站数据。
type ListBrowserEnvQuery struct {
	UserID   string
	RPAType  string
	Status   string
	Page     int
	PageSize int
}

// RunBrowserEnvRequest 是启动环境包的请求体。
//
// run 的设计原则是“围绕 envId 恢复环境包”，不是让前端透传 Docker 参数；
// 第一版只允许 forceRecreate 这类生命周期开关，镜像、端口、挂载、指纹和代理都必须从环境包读取。
type RunBrowserEnvRequest struct {
	ForceRecreate bool `json:"forceRecreate"`
}

// StopBrowserEnvRequest 是停止环境包对应容器的请求体。
//
// 设计来源：
// - stop(envId) 是浏览器环境生命周期的正规入口，前端不应该直接记住 Docker containerId 再调 Edge 容器接口；
// - 当前只允许传 timeoutSeconds，原因是 Docker stop 需要一个浏览器写盘缓冲时间；
// - 不允许透传 force、signal 等危险参数，避免误伤 browser-data/profile 中的登录态文件。
type StopBrowserEnvRequest struct {
	TimeoutSeconds *int `json:"timeoutSeconds"`
}

// UpdateBrowserEnvProxyRequest 是修改环境包代理配置的请求体。
//
// 设计来源：
// - 用户明确要求“只要改的东西，就需要重新启动容器”；
// - 代理配置会进入 run 阶段的容器环境变量，但不参与 identityHash；
// - 镜像选择归 Server 或中心 ImagePolicy，不能再借代理接口顺手修改 runtime.image；
// - 因此这里使用 PATCH 语义允许局部字段；running 环境会进入后台重建队列，非 running 环境返回 restartRequired=true。
type UpdateBrowserEnvProxyRequest struct {
	Enabled *bool   `json:"enabled"`
	Type    *string `json:"type"`
	// Mode 是 Clash 顶层代理模式。
	//
	// 设计来源：
	// - 用户确认创建和修改代理时也应该能同时切换 rule/global/direct；
	// - mode 属于 proxy/clash.yaml 的配置事实，不能放进 run 临时参数；
	// - 如果和 configBase64 同时传入，后端会先解码 YAML，再写入顶层 mode。
	Mode *string `json:"mode"`
	// ConfigBase64 是代理 YAML 原文的 Base64 编码。
	//
	// 设计来源：
	// - 用户实测长 YAML 通过 JSON 字符串传输时容易被 Apifox/前端截断或转义出错；
	// - 浏览器镜像正式使用 MIHOMO_CONFIG_BASE64 传入代理配置，旧 CLASH_VERGE_CONFIG_BASE64 仅由 entrypoint 临时兼容；
	// - 因此修改接口也改为优先接收 configBase64，后端解码后再写入 proxy/clash.yaml。
	ConfigBase64 *string `json:"configBase64"`
}

// UpdateBrowserEnvProxyModeRequest 是切换 Clash 代理模式的请求体。
//
// 设计来源：
// - 用户确认规则模式/全局模式不应塞进 run 参数，而应作为代理配置修改接口；
// - mode 属于 proxy/clash.yaml 的事实，不属于容器生命周期参数；
// - 切换后需要走和代理配置修改相同的重建与 timezone 重新探测链路。
type UpdateBrowserEnvProxyModeRequest struct {
	Mode string `json:"mode"`
}
