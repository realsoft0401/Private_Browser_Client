package BrowserEnv

// 环境包落盘文件模型单独拆文件，是为了明确这些结构体才是 browser-envs 目录的真实文件协议。
// 它们和 HTTP 请求/响应虽然字段相似，但职责完全不同，后续维护时不能混着改。

// PackageExportSource 记录环境包来源。
//
// 字段名沿用早期 export 协议，但从 2026-06-09 起写入 profile.package，
// 不再单独生成 manifest.json，避免环境包出现两份互相竞争的主配置。
// 这些字段只写入 staging 副本，用于后续 import-package 审计包从哪里来；
// 它们不是账号环境稳定身份，不能参与 identityHash。
type PackageExportSource struct {
	Type           string `json:"type"`
	Env            string `json:"env"`
	ServiceVersion string `json:"serviceVersion"`
}

// PackagePaths 统一记录环境包内相对路径。
type PackagePaths struct {
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

// PackageLastRuntime 记录环境包最近一次运行位置。
//
// 这些字段只用于审计和排障，不参与 identityHash。
type PackageLastRuntime struct {
	EdgeClientID  *string `json:"edgeClientId"`
	DeviceArch    *string `json:"deviceArch"`
	DockerAPIURL  *string `json:"dockerApiUrl"`
	ContainerID   *string `json:"containerId"`
	ContainerName *string `json:"containerName"`
	LastStartedAt *int64  `json:"lastStartedAt"`
	LastStoppedAt *int64  `json:"lastStoppedAt"`
}

// ProfilePackageMetadata 保存环境包打包、备份和导入阶段的审计信息。
//
// 设计来源：
// - 用户确认 profile.json 是环境包唯一详细配置文档和 SQLite 重建入口；
// - 因此旧 manifest.json 里的 packageVersion/export/checksums 不能继续独立存在；
// - 这些字段只服务包完整性校验和来源追踪，不参与 identityHash，不承载运行态决策。
type ProfilePackageMetadata struct {
	Version      *int                 `json:"version,omitempty"`
	ExportedAt   *int64               `json:"exportedAt,omitempty"`
	ExportSource *PackageExportSource `json:"exportSource,omitempty"`
	ExportAction string               `json:"exportAction,omitempty"`
	Checksums    map[string]string    `json:"checksums,omitempty"`
}

// ProfileFile 是 profile.json 的落盘结构。
type ProfileFile struct {
	SchemaVersion int                    `json:"schemaVersion"`
	EnvID         string                 `json:"envId"`
	UserID        string                 `json:"userId"`
	RPAType       string                 `json:"rpaType"`
	SnowflakeID   string                 `json:"snowflakeId"`
	EnvSequence   int                    `json:"envSequence"`
	Name          string                 `json:"name"`
	IdentityHash  string                 `json:"identityHash"`
	Runtime       ProfileRuntime         `json:"runtime"`
	Environment   ProfileEnv             `json:"environment"`
	Ports         BrowserEnvPorts        `json:"ports"`
	Proxy         ProfileProxy           `json:"proxy"`
	Paths         PackagePaths           `json:"paths"`
	LastRuntime   PackageLastRuntime     `json:"lastRuntime"`
	Package       ProfilePackageMetadata `json:"package"`
	Metadata      ProfileMetadata        `json:"metadata"`
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
	Identity          BindingIdentity    `json:"identity"`
	Storage           BindingStorage     `json:"storage"`
	SessionState      BindingSession     `json:"sessionState"`
	Fingerprint       BindingFingerprint `json:"fingerprint"`
	RuntimeProtection RuntimeProtection  `json:"runtimeProtection"`
	CreatedAt         int64              `json:"createdAt"`
	UpdatedAt         int64              `json:"updatedAt"`
}

// BindingIdentity 是 identityHash 的来源结构。
//
// 当前身份摘要只用于确认环境包稳定身份，用户已经明确 timezone、language、screen、
// proxy、browserDataPath 和运行位置都不参与身份计算。
type BindingIdentity struct {
	EnvID   string `json:"envId"`
	UserID  string `json:"userId"`
	RPAType string `json:"rpaType"`
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
	LastVerifiedAt      *int64 `json:"lastVerifiedAt,omitempty"`
	TimezoneStatus      string `json:"timezoneStatus,omitempty"`
	RiskStatus          string `json:"riskStatus,omitempty"`
	AvailabilityStatus  string `json:"availabilityStatus,omitempty"`
	LastError           string `json:"lastError,omitempty"`
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
	CheckedAt *int64                 `json:"checkedAt"`
	ExitIP    *string                `json:"exitIp"`
	Region    *string                `json:"region"`
	Country   *string                `json:"country"`
	Timezone  *string                `json:"timezone"`
	Source    *string                `json:"source"`
	Status    string                 `json:"status,omitempty"`
	Attempts  []TimezoneProbeAttempt `json:"attempts,omitempty"`
	Drift     bool                   `json:"drift"`
}

// TimezoneProbeAttempt 记录容器内 timezone 探测的单个 provider 结果。
//
// 它只记录排障摘要，不保存完整响应体，避免把外部服务返回的大 JSON 写入环境包。
type TimezoneProbeAttempt struct {
	Provider string `json:"provider"`
	URL      string `json:"url"`
	OK       bool   `json:"ok"`
	Error    string `json:"error,omitempty"`
}
