package BrowserEnv

// 详情/索引/更新摘要模型单独拆出来，职责是“本机可查询事实”，不是 HTTP 输入协议或 profile 落盘结构。

// BrowserEnvPorts 是环境包在当前 Client 上分配的本机端口。
//
// 这些端口属于本机运行资源，不属于账号身份；
// 因此导入、恢复、revalidate 时允许在不破坏 envId 的前提下重新分配。
type BrowserEnvPorts struct {
	CDP int `json:"cdp"`
	VNC int `json:"vnc"`
}

// BrowserEnvDetailResponse 是环境包详情接口响应。
//
// 设计来源：
// - 列表只返回摘要，不足以支撑单环境排障和后续配置修改；
// - 详情必须围绕 SQLite 索引和环境包文件共同组成；
// - 敏感原文如 clash.yaml、fingerprint raw、browser-data 内容不能直接通过详情接口泄漏。
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
type BrowserEnvProxyDetail struct {
	Enabled         bool             `json:"enabled"`
	Type            string           `json:"type"`
	Mode            string           `json:"mode,omitempty"`
	ConfigPath      string           `json:"configPath"`
	ConfigSizeBytes int              `json:"configSizeBytes"`
	Runtime         ProxyRuntimeFile `json:"runtime"`
}

// BrowserEnvFingerprintDetail 是指纹详情摘要。
type BrowserEnvFingerprintDetail struct {
	Snapshot      BrowserEnvFingerprintSnapshotDetail `json:"snapshot"`
	Backup        BrowserEnvFingerprintBackupDetail   `json:"backup"`
	RuntimeConfig BrowserEnvFingerprintRuntimeDetail  `json:"runtimeConfig"`
}

type BrowserEnvFingerprintSnapshotDetail struct {
	OK        bool             `json:"ok"`
	Source    string           `json:"source"`
	TargetURL string           `json:"targetUrl"`
	PageURL   string           `json:"pageUrl"`
	Title     string           `json:"title"`
	Score     FingerprintScore `json:"score"`
}

type BrowserEnvFingerprintBackupDetail struct {
	Available          bool           `json:"available"`
	SourceSnapshotPath string         `json:"sourceSnapshotPath"`
	HasFingerprint     bool           `json:"hasFingerprint"`
	Fingerprint        map[string]any `json:"fingerprint,omitempty"`
}

type BrowserEnvFingerprintRuntimeDetail struct {
	Available   bool           `json:"available"`
	Fingerprint map[string]any `json:"fingerprint,omitempty"`
}

// BrowserEnvConsistencyCheck 表示详情读取时做的轻量一致性检查。
//
// 这里只校验索引和文件能否对上，不做 Docker 实时探测。
type BrowserEnvConsistencyCheck struct {
	ProfileMatchesIndex bool     `json:"profileMatchesIndex"`
	IdentityHashMatches bool     `json:"identityHashMatches"`
	ProxyConfigExists   bool     `json:"proxyConfigExists"`
	BrowserDataExists   bool     `json:"browserDataExists"`
	Errors              []string `json:"errors"`
}

// BrowserEnvDetailVNCConnection 是详情接口里返回的运行态 VNC 摘要。
type BrowserEnvDetailVNCConnection struct {
	VNCURL    string `json:"vncUrl"`
	VNCWSURL  string `json:"vncWsUrl"`
	WebVNCURL string `json:"webVncUrl"`
}

// BrowserEnvIndex 是 SQLite `browser_envs` 索引表的轻量资产模型。
//
// 这张表只保存列表、状态和运行摘要，不保存 profile 原文、代理明文或浏览器登录态数据。
type BrowserEnvIndex struct {
	EnvID               string  `json:"envId"`
	UserID              string  `json:"userId"`
	RPAType             string  `json:"rpaType"`
	Name                string  `json:"name"`
	EnvSequence         int     `json:"envSequence"`
	CDPPort             int     `json:"cdpPort"`
	VNCPort             int     `json:"vncPort"`
	VNCURL              string  `json:"vncUrl,omitempty"`
	VNCWSURL            string  `json:"vncWsUrl,omitempty"`
	WebVNCURL           string  `json:"webVncUrl,omitempty"`
	EnvPath             string  `json:"envPath"`
	Status              string  `json:"status"`
	ContainerID         *string `json:"containerId,omitempty"`
	ContainerName       *string `json:"containerName,omitempty"`
	ContainerStatus     string  `json:"containerStatus"`
	MonitorStatus       string  `json:"monitorStatus"`
	LastError           *string `json:"lastError,omitempty"`
	BackupPath          *string `json:"backupPath,omitempty"`
	BackupChecksum      *string `json:"backupChecksum,omitempty"`
	BackupSize          *int64  `json:"backupSize,omitempty"`
	BackupAt            *int64  `json:"backupAt,omitempty"`
	FingerprintRestored bool    `json:"fingerprintRestored"`
	HasBrowserData      bool    `json:"hasBrowserData"`
	CreatedAt           int64   `json:"createdAt"`
	UpdatedAt           int64   `json:"updatedAt"`
	DeletedAt           *int64  `json:"deletedAt,omitempty"`
	LastStartedAt       *int64  `json:"lastStartedAt,omitempty"`
	LastStoppedAt       *int64  `json:"lastStoppedAt,omitempty"`
	LastCheckedAt       *int64  `json:"lastCheckedAt,omitempty"`
}

// BrowserEnvConfigUpdate 用于同步配置修改后的轻量索引字段。
type BrowserEnvConfigUpdate struct {
	EnvID     string
	Status    string
	LastError *string
	UpdatedAt int64
}

// BrowserEnvRuntimeUpdate 用于同步运行态摘要。
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

// BrowserEnvBackupStateUpdate 用于同步备份/恢复后的资产索引状态。
type BrowserEnvBackupStateUpdate struct {
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
	HasBrowserData  bool
	BackupPath      *string
	BackupChecksum  *string
	BackupSize      *int64
	BackupAt        *int64
	UpdatedAt       int64
	LastRestoredAt  *int64
	LastStartedAt   *int64
	LastStoppedAt   *int64
	LastCheckedAt   *int64
}
