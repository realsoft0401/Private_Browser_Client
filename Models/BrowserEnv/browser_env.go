package BrowserEnv

import "encoding/json"

const (
	// SchemaVersion 是环境包文件格式版本。
	//
	// 设计来源：
	// - 环境包后续要支持云存储、跨设备导入和长期迁移；
	// - schemaVersion 写进 manifest 后，未来字段升级时可以做兼容判断；
	// - 第一版固定为 1，不允许调用方传入覆盖。
	SchemaVersion = 1

	// DefaultContainerUserDataDir 是容器内 Chromium 用户数据目录。
	//
	// 登录态真实载体会挂载到这个路径；后续不能随便改成临时目录，
	// 否则 Cookies、LocalStorage、IndexedDB 等登录态数据无法复用。
	DefaultContainerUserDataDir = "/data/profile"
	DefaultStartupURL           = "about:blank"
	DefaultShmSize              = "1g"
	DefaultScreenDepth          = 24
)

const (
	// BrowserEnvStatusCreated 表示环境包已建立，但还没有真正启动容器。
	BrowserEnvStatusCreated = "created"
	// BrowserEnvStatusRunning 表示环境包已进入运行态。
	BrowserEnvStatusRunning = "running"
	// BrowserEnvStatusStopped 表示最近一次容器已经停止。
	BrowserEnvStatusStopped = "stopped"
	// BrowserEnvStatusDeleted 表示逻辑删除，不应再作为正常列表展示。
	BrowserEnvStatusDeleted = "deleted"
	// BrowserEnvStatusArchived 表示已归档，可保留文件但不参与活跃列表。
	BrowserEnvStatusArchived = "archived"
	// BrowserEnvStatusError 表示创建或运行过程发生异常。
	BrowserEnvStatusError = "error"

	// BrowserEnvContainerStatusUnknown 表示数据库里还没有容器事实快照。
	BrowserEnvContainerStatusUnknown = "unknown"
	// BrowserEnvMonitorStatusUnknown 表示监控尚未上报。
	BrowserEnvMonitorStatusUnknown = "unknown"
)

// SupportedRPATypes 是第一版允许的 RPA 类型。
//
// 这里保留 tk/fb/ins 等短码，是为了让 envId、云存储 key 和目录结构保持短而稳定。
// 如果未来新增平台，应先在这里扩展枚举，再同步更新 OpenAPI 和项目文档。
var SupportedRPATypes = map[string]struct{}{
	"tk":    {},
	"fb":    {},
	"ins":   {},
	"yt":    {},
	"x":     {},
	"other": {},
}

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

// CreateProxyRequest 描述环境包代理配置。
//
// proxy.config 会单独写入 proxy/clash.yaml，profile 里只保存 configPath，
// 避免大段 YAML 混入 profile 导致后续 diff、迁移和 hash 处理变复杂。
type CreateProxyRequest struct {
	Enabled *bool  `json:"enabled"`
	Type    string `json:"type"`
	Config  string `json:"config"`
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

// CreateBrowserEnvResponse 是创建环境包成功后的响应。
type CreateBrowserEnvResponse struct {
	EnvID        string            `json:"envId"`
	UserID       string            `json:"userId"`
	RPAType      string            `json:"rpaType"`
	EnvSequence  int               `json:"envSequence"`
	Ports        BrowserEnvPorts   `json:"ports"`
	EnvPath      string            `json:"envPath"`
	Files        map[string]string `json:"files"`
	IdentityHash string            `json:"identityHash"`
	ConfigHash   string            `json:"configHash"`
	CreatedAt    int64             `json:"createdAt"`
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

// ListBrowserEnvResponse 是环境包列表接口响应。
//
// total/byStatus/byRpaType 都来自数据库索引，items 只返回摘要字段；
// 这样前端能快速渲染列表和统计，但不会意外拿到代理明文、指纹原文或浏览器登录态数据。
type ListBrowserEnvResponse struct {
	Total     int64              `json:"total"`
	Page      int                `json:"page"`
	PageSize  int                `json:"pageSize"`
	ByStatus  map[string]int64   `json:"byStatus"`
	ByRPAType map[string]int64   `json:"byRpaType"`
	Items     []*BrowserEnvIndex `json:"items"`
}

// RunBrowserEnvRequest 是启动环境包的请求体。
//
// run 的设计原则是“围绕 envId 恢复环境包”，不是让前端透传 Docker 参数；
// 第一版只允许 forceRecreate 这类生命周期开关，镜像、端口、挂载、指纹和代理都必须从环境包读取。
type RunBrowserEnvRequest struct {
	ForceRecreate bool `json:"forceRecreate"`
}

// RunBrowserEnvResponse 是环境包启动后的运行摘要。
//
// 这些字段全部属于运行态，不参与 identityHash；
// 后续中心服务端可用它记录边缘执行结果，前端可用 cdpUrl/vncUrl 连接本机浏览器实例。
type RunBrowserEnvResponse struct {
	EnvID         string          `json:"envId"`
	ContainerID   string          `json:"containerId"`
	ContainerName string          `json:"containerName"`
	Image         string          `json:"image"`
	Status        string          `json:"status"`
	Ports         BrowserEnvPorts `json:"ports"`
	CDPURL        string          `json:"cdpUrl"`
	VNCURL        string          `json:"vncUrl"`
	DockerAPIURL  string          `json:"dockerApiUrl"`
	DeviceArch    string          `json:"deviceArch"`
	StartedAt     int64           `json:"startedAt"`
	Reused        bool            `json:"reused"`
}

// BrowserEnvVNCInfoResponse 是浏览器版 VNC 连接信息。
//
// 设计来源：
// - Mac 原生 VNC 客户端会弹密码框，用户明确要求改成浏览器内访问；
// - 浏览器不能直接连 vnc:// TCP，因此边缘服务提供 WebSocket 代理地址，前端 noVNC 连接 wsUrl；
// - vncUrl 仍保留给排障或原生客户端使用，但推荐前端使用 webVncUrl。
type BrowserEnvVNCInfoResponse struct {
	EnvID     string `json:"envId"`
	VNCPort   int    `json:"vncPort"`
	VNCURL    string `json:"vncUrl"`
	WSURL     string `json:"wsUrl"`
	WebVNCURL string `json:"webVncUrl"`
}

// BrowserEnvDetailResponse 是环境包详情接口响应。
//
// 设计来源：
// - 用户要求前端能进入单个环境包查看完整配置关系；
// - 详情必须围绕 SQLite 索引和环境包文件共同组成，不能只看列表摘要；
// - 代理配置和指纹原文可能很大且敏感，详情只返回摘要、hash 和路径，不返回 clash.yaml 明文或 fingerprint raw。
//
// 职责边界：
// - 负责给前端展示 manifest/profile/binding/container 的结构化事实；
// - 负责暴露运行中 VNC 入口；
// - 不负责修改环境包，也不做 Docker 实时探测。
type BrowserEnvDetailResponse struct {
	Index       *BrowserEnvIndex               `json:"index"`
	Manifest    ManifestFile                   `json:"manifest"`
	Profile     ProfileFile                    `json:"profile"`
	Binding     BrowserEnvBindingDetail        `json:"binding"`
	Container   ContainerFile                  `json:"container"`
	Proxy       BrowserEnvProxyDetail          `json:"proxy"`
	Fingerprint BrowserEnvFingerprintDetail    `json:"fingerprint"`
	Consistency BrowserEnvConsistencyCheck     `json:"consistency"`
	Files       map[string]string              `json:"files"`
	VNC         *BrowserEnvDetailVNCConnection `json:"vnc,omitempty"`
}

// BrowserEnvBindingDetail 是详情接口里可展示的 binding 摘要。
//
// 它保留 identityHash、storage、sessionState 这类排障必需字段，
// 但不额外展开 browser-data 目录内容，登录态载体不能从详情接口泄漏。
type BrowserEnvBindingDetail struct {
	ID                string             `json:"id"`
	Version           int                `json:"version"`
	Locked            bool               `json:"locked"`
	IdentityHash      string             `json:"identityHash"`
	ConfigHash        string             `json:"configHash"`
	Identity          BindingIdentity    `json:"identity"`
	Storage           BindingStorage     `json:"storage"`
	SessionState      BindingSession     `json:"sessionState"`
	Fingerprint       BindingFingerprint `json:"fingerprint"`
	RuntimeProtection RuntimeProtection  `json:"runtimeProtection"`
	CreatedAt         int64              `json:"createdAt"`
	UpdatedAt         int64              `json:"updatedAt"`
}

// BrowserEnvProxyDetail 是代理详情摘要。
//
// configHash 用于判断代理配置是否变化；configSizeBytes 便于前端展示文件是否为空。
// 这里明确不返回代理配置正文，后续如果要编辑代理，应走专门的代理修改接口。
type BrowserEnvProxyDetail struct {
	Enabled         bool             `json:"enabled"`
	Type            string           `json:"type"`
	ConfigPath      string           `json:"configPath"`
	ConfigHash      string           `json:"configHash"`
	ConfigSizeBytes int              `json:"configSizeBytes"`
	Runtime         ProxyRuntimeFile `json:"runtime"`
}

// BrowserEnvFingerprintDetail 是指纹详情摘要。
//
// snapshot 和 backup 都只返回状态和可恢复字段，不返回 raw，
// 避免把浏览器检测站或人工确认结果里的完整原文塞进普通详情接口。
type BrowserEnvFingerprintDetail struct {
	Snapshot      BrowserEnvFingerprintSnapshotDetail `json:"snapshot"`
	Backup        BrowserEnvFingerprintBackupDetail   `json:"backup"`
	RuntimeConfig BrowserEnvFingerprintRuntimeDetail  `json:"runtimeConfig"`
}

// BrowserEnvFingerprintSnapshotDetail 是最近一次指纹检测摘要。
type BrowserEnvFingerprintSnapshotDetail struct {
	OK        bool             `json:"ok"`
	Source    string           `json:"source"`
	TestedAt  *int64           `json:"testedAt"`
	TargetURL string           `json:"targetUrl"`
	PageURL   string           `json:"pageUrl"`
	Title     string           `json:"title"`
	Score     FingerprintScore `json:"score"`
}

// BrowserEnvFingerprintBackupDetail 是正式指纹备份摘要。
type BrowserEnvFingerprintBackupDetail struct {
	Available          bool                         `json:"available"`
	SavedAt            *int64                       `json:"savedAt"`
	SourceSnapshotPath string                       `json:"sourceSnapshotPath"`
	HasFingerprint     bool                         `json:"hasFingerprint"`
	Fingerprint        *RestorableFingerprintConfig `json:"fingerprint,omitempty"`
}

// BrowserEnvFingerprintRuntimeDetail 是可注入容器的指纹配置摘要。
type BrowserEnvFingerprintRuntimeDetail struct {
	Available   bool                         `json:"available"`
	Fingerprint *RestorableFingerprintConfig `json:"fingerprint,omitempty"`
}

// BrowserEnvConsistencyCheck 表示详情读取时做的轻量一致性检查。
//
// 这不是运行健康检查，不会访问 Docker；它只验证数据库索引和环境包文件是否还能互相对上。
type BrowserEnvConsistencyCheck struct {
	ManifestMatchesIndex bool     `json:"manifestMatchesIndex"`
	IdentityHashMatches  bool     `json:"identityHashMatches"`
	ProxyConfigExists    bool     `json:"proxyConfigExists"`
	BrowserDataExists    bool     `json:"browserDataExists"`
	Errors               []string `json:"errors"`
}

// BrowserEnvDetailVNCConnection 是详情接口中运行态 VNC 连接入口。
//
// 只有环境包 status=running 且有 vncPort 时才返回，和列表接口的规则保持一致。
type BrowserEnvDetailVNCConnection struct {
	VNCURL    string `json:"vncUrl"`
	VNCWSURL  string `json:"vncWsUrl"`
	WebVNCURL string `json:"webVncUrl"`
}

// BrowserEnvRuntimeUpdate 是 browser_envs 运行态字段更新模型。
//
// 它只更新列表和状态需要的摘要字段，不把 Docker create 的完整配置或环境变量写进数据库；
// 完整运行事实仍以 container.json / manifest.lastRuntime 和 Docker daemon 为准。
type BrowserEnvRuntimeUpdate struct {
	EnvID           string
	Status          string
	ContainerID     *string
	ContainerName   *string
	ContainerStatus string
	MonitorStatus   string
	LastError       *string
	UpdatedAt       int64
	LastStartedAt   *int64
	LastStoppedAt   *int64
	LastCheckedAt   *int64
}

// BrowserEnvIndex 是 browser_envs 表的索引型元数据模型。
//
// 设计来源：
// - 用户要求边缘服务能查询“本机管理了多少配置文件”，并且支持假删除和后续状态监控；
// - 所以这里保存的是环境包索引、状态快照和少量运行元信息，不保存 profile 原文、代理原文和登录态数据；
// - 真正的大字段都留在文件系统里，数据库只做可查询索引层。
//
// 职责边界：
// - 负责快速查询、过滤、统计和状态判断；
// - 不负责承载浏览器登录态，也不替代文件系统中的环境包事实；
// - 后续如果有中心服务上报，也应优先更新这张索引表，再由前端或 API 聚合展示。
type BrowserEnvIndex struct {
	// EnvID 是环境包唯一编号，也是文件夹目录名和数据库主键。
	EnvID string `json:"envId"`
	// UserID 是环境包归属用户，只用于本机按用户分组查询。
	UserID string `json:"userId"`
	// RPAType 表示环境类型，例如 tk / fb / ins。
	RPAType string `json:"rpaType"`
	// Name 是展示名称，不参与身份计算。
	Name string `json:"name"`
	// EnvSequence 是本机递增序号，是端口规则的来源。
	EnvSequence int `json:"envSequence"`
	// CDPPort 是当前环境包分配的 CDP 端口。
	CDPPort int `json:"cdpPort"`
	// VNCPort 是当前环境包分配的 VNC 端口。
	VNCPort int `json:"vncPort"`
	// VNCURL 是运行中环境包的原生 VNC 地址。
	//
	// 只有 status=running 时才返回；非运行态返回空，避免前端误认为可以连接。
	VNCURL string `json:"vncUrl,omitempty"`
	// VNCWSURL 是运行中环境包的 noVNC WebSocket 代理地址。
	//
	// 前端浏览器应优先把这个地址交给 noVNC，而不是直接使用 vnc://。
	VNCWSURL string `json:"vncWsUrl,omitempty"`
	// WebVNCURL 是运行中环境包的独立浏览器 VNC 页面地址。
	//
	// 这是给 Mac/浏览器用户直接打开的入口，避免原生 VNC 客户端弹密码框。
	WebVNCURL string `json:"webVncUrl,omitempty"`
	// EnvPath 是环境包相对路径，只存索引，不存绝对路径。
	EnvPath string `json:"envPath"`
	// Status 是环境包生命周期状态。
	Status string `json:"status"`
	// ContainerID 保存最近一次运行容器 ID，未运行时为空。
	ContainerID *string `json:"containerId,omitempty"`
	// ContainerName 保存最近一次运行容器名，便于排障。
	ContainerName *string `json:"containerName,omitempty"`
	// ContainerStatus 保存最近一次本机容器状态快照。
	ContainerStatus string `json:"containerStatus"`
	// MonitorStatus 保存本机监控检查结果。
	MonitorStatus string `json:"monitorStatus"`
	// LastError 保存最近一次可读错误，方便前端展示和排障。
	LastError *string `json:"lastError,omitempty"`
	// FingerprintRestored 表示是否已经把指纹注入运行态容器。
	FingerprintRestored bool `json:"fingerprintRestored"`
	// HasBrowserData 表示 browser-data/profile 目录是否已建立。
	HasBrowserData bool `json:"hasBrowserData"`
	// CreatedAt 是记录创建时间。
	CreatedAt int64 `json:"createdAt"`
	// UpdatedAt 是记录最近更新时间。
	UpdatedAt int64 `json:"updatedAt"`
	// DeletedAt 非空时表示假删除。
	DeletedAt *int64 `json:"deletedAt,omitempty"`
	// LastStartedAt 保存最近一次启动时间。
	LastStartedAt *int64 `json:"lastStartedAt,omitempty"`
	// LastStoppedAt 保存最近一次停止时间。
	LastStoppedAt *int64 `json:"lastStoppedAt,omitempty"`
	// LastCheckedAt 保存最近一次检查时间。
	LastCheckedAt *int64 `json:"lastCheckedAt,omitempty"`
}

// BrowserEnvPorts 是本机运行端口。
//
// 它不参与 identityHash；导入到其他设备时如果冲突，可以按本机规则重新分配。
type BrowserEnvPorts struct {
	CDP int `json:"cdp"`
	VNC int `json:"vnc"`
}

// RestorableFingerprintConfig 是可注入容器的指纹恢复配置。
//
// 这里只保存 navigator/screen 这类适合恢复的字段，不保存 Cookie、LocalStorage、
// IndexedDB 等登录态数据；登录态始终由 browser-data/profile 承载。
type RestorableFingerprintConfig struct {
	UserAgent           string         `json:"userAgent,omitempty"`
	Platform            string         `json:"platform,omitempty"`
	Language            string         `json:"language,omitempty"`
	Languages           []string       `json:"languages,omitempty"`
	DeviceMemory        int            `json:"deviceMemory,omitempty"`
	HardwareConcurrency int            `json:"hardwareConcurrency,omitempty"`
	ColorDepth          int            `json:"colorDepth,omitempty"`
	Screen              *ScreenSize    `json:"screen,omitempty"`
	AvailableScreen     *ScreenSize    `json:"availableScreen,omitempty"`
	MaxTouchPoints      *int           `json:"maxTouchPoints,omitempty"`
	Extra               map[string]any `json:"extra,omitempty"`
}

// ScreenSize 是指纹恢复配置中的屏幕尺寸。
type ScreenSize struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

// ManifestFile 是 manifest.json 的落盘结构。
type ManifestFile struct {
	SchemaVersion int                 `json:"schemaVersion"`
	EnvID         string              `json:"envId"`
	UserID        string              `json:"userId"`
	RPAType       string              `json:"rpaType"`
	SnowflakeID   string              `json:"snowflakeId"`
	EnvSequence   int                 `json:"envSequence"`
	Paths         ManifestPaths       `json:"paths"`
	LastRuntime   ManifestLastRuntime `json:"lastRuntime"`
	CreatedAt     int64               `json:"createdAt"`
	UpdatedAt     int64               `json:"updatedAt"`
}

// ManifestPaths 统一记录环境包内相对路径。
type ManifestPaths struct {
	Profile                  string `json:"profile"`
	Binding                  string `json:"binding"`
	Container                string `json:"container"`
	BrowserData              string `json:"browserData"`
	FingerprintSnapshot      string `json:"fingerprintSnapshot"`
	FingerprintBackup        string `json:"fingerprintBackup"`
	FingerprintRuntimeConfig string `json:"fingerprintRuntimeConfig"`
	ProxyConfig              string `json:"proxyConfig"`
	ProxyRuntime             string `json:"proxyRuntime"`
	Logs                     string `json:"logs"`
}

// ManifestLastRuntime 记录环境包最近一次运行位置。
//
// 这些字段只用于审计和排障，不参与 identityHash。
type ManifestLastRuntime struct {
	EdgeNodeID    *string `json:"edgeNodeId"`
	DeviceArch    *string `json:"deviceArch"`
	DockerAPIURL  *string `json:"dockerApiUrl"`
	ContainerID   *string `json:"containerId"`
	ContainerName *string `json:"containerName"`
	LastStartedAt *int64  `json:"lastStartedAt"`
	LastStoppedAt *int64  `json:"lastStoppedAt"`
}

// ProfileFile 是 profile.json 的落盘结构。
type ProfileFile struct {
	EnvID       string          `json:"envId"`
	EnvSequence int             `json:"envSequence"`
	Name        string          `json:"name"`
	RPAType     string          `json:"rpaType"`
	Runtime     ProfileRuntime  `json:"runtime"`
	Environment ProfileEnv      `json:"environment"`
	Ports       BrowserEnvPorts `json:"ports"`
	Proxy       ProfileProxy    `json:"proxy"`
	Metadata    ProfileMetadata `json:"metadata"`
}

// ProfileRuntime 保存浏览器容器启动所需的稳定运行配置。
type ProfileRuntime struct {
	Image                string `json:"image"`
	ContainerUserDataDir string `json:"containerUserDataDir"`
	StartupURL           string `json:"startupUrl"`
	EnableVNC            bool   `json:"enableVnc"`
	ShmSize              string `json:"shmSize"`
}

// ProfileEnv 保存参与环境身份的浏览器环境配置。
type ProfileEnv struct {
	Timezone string        `json:"timezone"`
	Language string        `json:"language"`
	Screen   ProfileScreen `json:"screen"`
}

// ProfileScreen 保存浏览器屏幕配置。
type ProfileScreen struct {
	Width  int `json:"width"`
	Height int `json:"height"`
	Depth  int `json:"depth"`
}

// ProfileProxy 只保存代理入口信息和配置路径。
type ProfileProxy struct {
	Enabled    bool   `json:"enabled"`
	Type       string `json:"type"`
	ConfigPath string `json:"configPath"`
}

// ProfileMetadata 保存展示和排障信息。
type ProfileMetadata struct {
	Source      string `json:"source"`
	Description string `json:"description"`
	CreatedAt   int64  `json:"createdAt"`
	UpdatedAt   int64  `json:"updatedAt"`
}

// BindingFile 是 binding.json 的落盘结构。
type BindingFile struct {
	ID                string             `json:"id"`
	Version           int                `json:"version"`
	Locked            bool               `json:"locked"`
	IdentityHash      string             `json:"identityHash"`
	ConfigHash        string             `json:"configHash"`
	Identity          BindingIdentity    `json:"identity"`
	Storage           BindingStorage     `json:"storage"`
	SessionState      BindingSession     `json:"sessionState"`
	Fingerprint       BindingFingerprint `json:"fingerprint"`
	RuntimeProtection RuntimeProtection  `json:"runtimeProtection"`
	CreatedAt         int64              `json:"createdAt"`
	UpdatedAt         int64              `json:"updatedAt"`
}

// BindingIdentity 是 identityHash 的来源结构。
type BindingIdentity struct {
	UserID          string                `json:"userId"`
	RPAType         string                `json:"rpaType"`
	Timezone        string                `json:"timezone"`
	Language        string                `json:"language"`
	Screen          BindingIdentityScreen `json:"screen"`
	Proxy           BindingIdentityProxy  `json:"proxy"`
	BrowserDataPath string                `json:"browserDataPath"`
}

type BindingIdentityScreen struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

type BindingIdentityProxy struct {
	Type       string `json:"type"`
	ConfigHash string `json:"configHash"`
}

type BindingStorage struct {
	ContainerUserDataDir string `json:"containerUserDataDir"`
	HostUserDataDir      string `json:"hostUserDataDir"`
}

type BindingSession struct {
	Platform        string  `json:"platform"`
	AccountMask     *string `json:"accountMask"`
	HasLoginState   bool    `json:"hasLoginState"`
	Status          string  `json:"status"`
	LastLoginAt     *int64  `json:"lastLoginAt"`
	LastValidatedAt *int64  `json:"lastValidatedAt"`
}

type BindingFingerprint struct {
	SnapshotPath      string `json:"snapshotPath"`
	BackupPath        string `json:"backupPath"`
	RuntimeConfigPath string `json:"runtimeConfigPath"`
	Restored          bool   `json:"restored"`
}

type RuntimeProtection struct {
	FingerprintRestored *bool  `json:"fingerprintRestored"`
	RuntimeDrift        *bool  `json:"runtimeDrift"`
	ExitIPChanged       *bool  `json:"exitIpChanged"`
	HighRisk            *bool  `json:"highRisk"`
	LastCheckedAt       *int64 `json:"lastCheckedAt"`
}

// ContainerFile 是 container.json 的落盘结构。
//
// 它只保存最近一次本机 Docker 运行信息，可以在迁移或重建时覆盖。
type ContainerFile struct {
	EnvID         string            `json:"envId"`
	ContainerName string            `json:"containerName"`
	ContainerID   *string           `json:"containerId"`
	Image         string            `json:"image"`
	Status        string            `json:"status"`
	Ports         BrowserEnvPorts   `json:"ports"`
	Docker        ContainerDocker   `json:"docker"`
	Labels        map[string]string `json:"labels"`
	CreatedAt     int64             `json:"createdAt"`
	StartedAt     *int64            `json:"startedAt"`
	StoppedAt     *int64            `json:"stoppedAt"`
	UpdatedAt     int64             `json:"updatedAt"`
}

type ContainerDocker struct {
	APIURL     string  `json:"apiUrl"`
	DeviceArch *string `json:"deviceArch"`
}

type FingerprintSnapshotFile struct {
	OK        bool             `json:"ok"`
	Source    string           `json:"source"`
	TestedAt  *int64           `json:"testedAt"`
	TargetURL string           `json:"targetUrl"`
	PageURL   string           `json:"pageUrl"`
	Title     string           `json:"title"`
	Score     FingerprintScore `json:"score"`
	Raw       map[string]any   `json:"raw"`
}

type FingerprintScore struct {
	Value     *int   `json:"value"`
	RiskLevel string `json:"riskLevel"`
	Summary   string `json:"summary"`
}

type FingerprintBackupFile struct {
	Available          bool                         `json:"available"`
	SavedAt            *int64                       `json:"savedAt"`
	SourceSnapshotPath string                       `json:"sourceSnapshotPath"`
	Fingerprint        *RestorableFingerprintConfig `json:"fingerprint"`
	Raw                any                          `json:"raw"`
}

type ProxyRuntimeFile struct {
	CheckedAt *int64  `json:"checkedAt"`
	ExitIP    *string `json:"exitIp"`
	Region    *string `json:"region"`
	Country   *string `json:"country"`
	Timezone  *string `json:"timezone"`
	Source    *string `json:"source"`
	Drift     bool    `json:"drift"`
}
