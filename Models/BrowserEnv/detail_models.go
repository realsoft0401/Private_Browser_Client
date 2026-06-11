package BrowserEnv

// 详情/索引/更新摘要模型单独拆出来，职责是“本机可查询事实”，不是 API 输入或 profile 落盘格式。

// BrowserEnvDetailResponse 是环境包详情接口响应。
//
// 设计来源：
// - 用户要求前端能进入单个环境包查看完整配置关系；
// - 详情必须围绕 SQLite 索引和环境包文件共同组成，不能只看列表摘要；
// - 代理配置和指纹原文可能很大且敏感，详情只返回摘要、hash 和路径，不返回 clash.yaml 明文或 fingerprint raw。
//
// 职责边界：
// - 负责给前端展示 profile/binding/container 的结构化事实；
// - 负责暴露运行中 VNC 入口；
// - 不负责修改环境包，也不做 Docker 实时探测。
type BrowserEnvDetailResponse struct {
	Index       *BrowserEnvIndex               `json:"index"`
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
// configSizeBytes 便于前端展示文件是否为空。
// 这里明确不返回代理配置正文，后续如果要编辑代理，应走专门的代理修改接口。
type BrowserEnvProxyDetail struct {
	Enabled         bool             `json:"enabled"`
	Type            string           `json:"type"`
	Mode            string           `json:"mode,omitempty"`
	ConfigPath      string           `json:"configPath"`
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
	ProfileMatchesIndex bool     `json:"profileMatchesIndex"`
	IdentityHashMatches bool     `json:"identityHashMatches"`
	ProxyConfigExists   bool     `json:"proxyConfigExists"`
	BrowserDataExists   bool     `json:"browserDataExists"`
	Errors              []string `json:"errors"`
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
// 完整运行事实仍以 container.json / profile.lastRuntime 和 Docker daemon 为准。
type BrowserEnvRuntimeUpdate struct {
	EnvID           string
	Status          string
	EnvSequence     *int
	CDPPort         *int
	VNCPort         *int
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

// BrowserEnvConfigUpdate 是配置修改后同步 browser_envs 索引的最小模型。
//
// 设计来源：
// - proxy/runtime 等配置事实保存在环境包文件里，SQLite 只需要同步 updated_at 和可展示状态；
// - 不能为了配置修改把 Service 直接伸手操作 Rom.Db/SQLite；
// - 因此单独建配置更新模型，继续保持 Service -> Dao -> Repository 的调用边界。
type BrowserEnvConfigUpdate struct {
	EnvID     string
	Status    string
	LastError *string
	UpdatedAt int64
}

// BrowserEnvBackupStateUpdate 是备份/恢复流程写回 SQLite 资产状态的模型。
//
// 设计来源：
// - 备份后环境目录会被删除，但这不是用户删除环境资产；
// - SQLite 需要保留 envId、备份包路径和恢复入口，所以不能复用 DeleteBrowserEnvIndex；
// - Repository 只按这些字段落库，不读取备份包，也不删除文件。
type BrowserEnvBackupStateUpdate struct {
	EnvID           string
	Status          string
	EnvSequence     *int
	CDPPort         *int
	VNCPort         *int
	ContainerID     *string
	ContainerStatus string
	MonitorStatus   string
	LastError       *string
	HasBrowserData  bool
	BackupPath      *string
	BackupChecksum  *string
	BackupSize      *int64
	BackupAt        *int64
	BackupVersion   *int
	LastRestoredAt  *int64
	UpdatedAt       int64
}

// BrowserEnvIndex 是 browser_envs 表的索引型元数据模型。
//
// 设计来源：
// - 用户要求边缘服务能查询“本机管理了多少配置文件”，并且支持后续状态监控；
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
	// BackupPath 保存本机备份包相对路径。
	//
	// 该字段只指向受控 data/browser-envs/users/{userId}/{rpaType}/ 下的 tar.gz，
	// 不保存外部任意路径，避免下载/恢复接口被扩展成任意文件读取入口。
	BackupPath *string `json:"backupPath,omitempty"`
	// BackupChecksum 保存备份包文件 sha256，用于恢复前确认包没有被替换或损坏。
	BackupChecksum *string `json:"backupChecksum,omitempty"`
	// BackupSize 保存备份包大小，供前端展示和排障。
	BackupSize *int64 `json:"backupSize,omitempty"`
	// BackupAt 保存最近一次备份完成时间。
	BackupAt *int64 `json:"backupAt,omitempty"`
	// BackupVersion 保存备份包协议版本。
	BackupVersion *int `json:"backupVersion,omitempty"`
	// LastRestoredAt 保存最近一次从备份恢复环境目录的时间。
	LastRestoredAt *int64 `json:"lastRestoredAt,omitempty"`
	// FingerprintRestored 表示是否已经把指纹注入运行态容器。
	FingerprintRestored bool `json:"fingerprintRestored"`
	// HasBrowserData 表示 browser-data/profile 目录是否已建立。
	HasBrowserData bool `json:"hasBrowserData"`
	// CreatedAt 是记录创建时间。
	CreatedAt int64 `json:"createdAt"`
	// UpdatedAt 是记录最近更新时间。
	UpdatedAt int64 `json:"updatedAt"`
	// DeletedAt 保留给历史假删除或后续归档兼容；当前 DELETE 会物理删除目录并移除索引。
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
