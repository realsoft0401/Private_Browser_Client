package BrowserEnv

// BrowserEnvPorts 是环境包在当前 Client 上分配的本机端口。
//
// 这些端口属于本机运行资源，不属于账号身份。
// 因此导入、恢复、revalidate 时允许在不破坏 envId 的前提下重新分配。
type BrowserEnvPorts struct {
	CDP int `json:"cdp"`
	VNC int `json:"vnc"`
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
//
// 代理修改不会改身份，也不会改端口和容器 ID，因此这里只更新状态/错误/更新时间。
type BrowserEnvConfigUpdate struct {
	EnvID     string
	Status    string
	LastError *string
	UpdatedAt int64
}

// BrowserEnvRuntimeUpdate 用于同步运行态摘要。
//
// 这层模型只服务 SQLite 索引回写，不承载 profile/binding/container 的完整文件内容。
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
