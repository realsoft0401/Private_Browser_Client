package BrowserEnv

// 这些结构体对应环境包目录中的真实文件协议。
//
// 不能把它们和 HTTP 请求/响应混用，因为：
// - HTTP 请求是外部输入协议；
// - profile/binding/container 是本机长期资产事实；
// - 后续 import/rebuild-index/restore 都要直接复用这些文件模型。

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

type PackageLastRuntime struct {
	ContainerID   *string `json:"containerId"`
	ContainerName *string `json:"containerName"`
	LastStartedAt *int64  `json:"lastStartedAt"`
	LastStoppedAt *int64  `json:"lastStoppedAt"`
}

type ProfileFile struct {
	SchemaVersion int                `json:"schemaVersion"`
	EnvID         string             `json:"envId"`
	UserID        string             `json:"userId"`
	RPAType       string             `json:"rpaType"`
	SnowflakeID   string             `json:"snowflakeId"`
	EnvSequence   int                `json:"envSequence"`
	Name          string             `json:"name"`
	IdentityHash  string             `json:"identityHash"`
	Runtime       ProfileRuntime     `json:"runtime"`
	Environment   ProfileEnvironment `json:"environment"`
	Ports         BrowserEnvPorts    `json:"ports"`
	Proxy         ProfileProxy       `json:"proxy"`
	Paths         PackagePaths       `json:"paths"`
	LastRuntime   PackageLastRuntime `json:"lastRuntime"`
	Metadata      ProfileMetadata    `json:"metadata"`
}

type ProfileRuntime struct {
	Image                string `json:"image"`
	ContainerUserDataDir string `json:"containerUserDataDir"`
	StartupURL           string `json:"startupUrl"`
	EnableVNC            bool   `json:"enableVnc"`
	ShmSize              string `json:"shmSize"`
}

type ProfileEnvironment struct {
	Timezone string        `json:"timezone"`
	Language string        `json:"language"`
	Screen   ProfileScreen `json:"screen"`
}

type ProfileScreen struct {
	Width  int `json:"width"`
	Height int `json:"height"`
	Depth  int `json:"depth"`
}

type ProfileProxy struct {
	Enabled    bool   `json:"enabled"`
	Type       string `json:"type"`
	ConfigPath string `json:"configPath"`
}

type ProfileMetadata struct {
	Source      string `json:"source"`
	Description string `json:"description"`
	CreatedAt   int64  `json:"createdAt"`
	UpdatedAt   int64  `json:"updatedAt"`
}

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
	Platform      string `json:"platform"`
	HasLoginState bool   `json:"hasLoginState"`
	Status        string `json:"status"`
}

type BindingFingerprint struct {
	SnapshotPath      string `json:"snapshotPath"`
	BackupPath        string `json:"backupPath"`
	RuntimeConfigPath string `json:"runtimeConfigPath"`
	Restored          bool   `json:"restored"`
}

type RuntimeProtection struct {
	TimezoneStatus     string `json:"timezoneStatus,omitempty"`
	RiskStatus         string `json:"riskStatus,omitempty"`
	AvailabilityStatus string `json:"availabilityStatus,omitempty"`
	LastError          string `json:"lastError,omitempty"`
}

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
	APIURL string `json:"apiUrl"`
}

type FingerprintSnapshotFile struct {
	OK        bool             `json:"ok"`
	Source    string           `json:"source"`
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
	Available          bool           `json:"available"`
	SourceSnapshotPath string         `json:"sourceSnapshotPath"`
	Raw                map[string]any `json:"raw"`
}

type ProxyRuntimeFile struct {
	Source *string `json:"source"`
	Status string  `json:"status,omitempty"`
	Drift  bool    `json:"drift"`
}
