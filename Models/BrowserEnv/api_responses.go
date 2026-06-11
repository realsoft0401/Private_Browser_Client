package BrowserEnv

// API 响应模型和请求模型拆开，避免“输入协议”和“输出摘要”继续纠缠在一个千行文件里。

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
	CreatedAt    int64             `json:"createdAt"`
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
	// TimezoneStatus 表示本次 run 附带的容器内 timezone 探测状态。
	//
	// 设计来源：
	// - 用户实测 provider/CDP 请求可能耗时过长，导致 PATCH proxy 或 run 没有 HTTP 返回；
	// - 因此 timezone 探测不再无限阻塞容器生命周期响应，失败会写入 proxy-runtime 并通过这里返回摘要。
	TimezoneStatus string `json:"timezoneStatus,omitempty"`
	TimezoneError  string `json:"timezoneError,omitempty"`
	StartedAt      int64  `json:"startedAt"`
	Reused         bool   `json:"reused"`
}

// BrowserEnvCDPTestResponse 是 CDP 基础诊断结果。
//
// 设计来源：
//   - 用户需要一个最小测试程序判断浏览器环境包的 CDP 端口是否可用；
//   - 这个结果只回答“端口、HTTP /json/version、Target 创建、WebSocket、Runtime.evaluate 是否通”，
//     不承载 timezone、代理出口或业务自动化判断。
//
// 职责边界：
// - ok=false 代表诊断失败，但接口本身仍可 code=1000 返回，方便前端展示 stage/error；
// - 环境包不存在、未运行、未分配 CDP 端口这类前置状态错误仍走统一业务错误；
// - endpoint 是边缘服务内部访问 Docker published port 的地址，前端不要把它当公网连接地址。
type BrowserEnvCDPTestResponse struct {
	EnvID           string `json:"envId"`
	CDPPort         int    `json:"cdpPort"`
	Endpoint        string `json:"endpoint"`
	OK              bool   `json:"ok"`
	Stage           string `json:"stage"`
	Browser         string `json:"browser,omitempty"`
	ProtocolVersion string `json:"protocolVersion,omitempty"`
	WebSocketURL    string `json:"webSocketUrl,omitempty"`
	RuntimeResult   string `json:"runtimeResult,omitempty"`
	Error           string `json:"error,omitempty"`
	TestedAt        int64  `json:"testedAt"`
	DurationMs      int64  `json:"durationMs"`
}

// StopBrowserEnvResponse 是停止环境包后的运行态摘要。
//
// 这些字段用于让中心服务端或前端确认“本次 stop 已经收口到环境包状态”，
// actionStatus 保留 Docker 304 的 not-modified 语义，用来表达容器原本已经停止。
type StopBrowserEnvResponse struct {
	EnvID           string  `json:"envId"`
	ContainerID     *string `json:"containerId,omitempty"`
	ContainerName   *string `json:"containerName,omitempty"`
	Status          string  `json:"status"`
	ContainerStatus string  `json:"containerStatus"`
	ActionStatus    string  `json:"actionStatus"`
	Message         string  `json:"message"`
	StoppedAt       int64   `json:"stoppedAt"`
}

// RevalidateBrowserEnvResponse 是异常环境重新准入后的状态摘要。
//
// 设计来源：
// - 用户明确要求 status=error 不能被 run/stop/backup/proxy update 隐式恢复；
// - 管理员排查后必须走受控 revalidate，只校验原子材料、Docker 事实和本机端口；
// - revalidate 不启动容器、不拉镜像、不证明网络指纹可用，因此 runtimeProtection 只能回到 pending。
type RevalidateBrowserEnvResponse struct {
	EnvID                   string          `json:"envId"`
	Status                  string          `json:"status"`
	ContainerStatus         string          `json:"containerStatus"`
	ContainerID             *string         `json:"containerId,omitempty"`
	ContainerName           *string         `json:"containerName,omitempty"`
	EnvSequence             int             `json:"envSequence"`
	Ports                   BrowserEnvPorts `json:"ports"`
	RuntimeProtectionStatus string          `json:"runtimeProtectionStatus"`
	RevalidatedAt           int64           `json:"revalidatedAt"`
	Message                 string          `json:"message"`
}

// DeleteBrowserEnvResponse 是环境包物理删除后的响应摘要。
//
// 设计来源：
// - 用户确认 DELETE 代表彻底删除，避免未来 rebuild-index 把不想要的环境包重新扫回来；
// - 前端负责给出“无法找回”的强提示，后端负责校验路径边界并删除环境包目录和索引记录；
// - status 保持 deleted 是对接口调用结果的表达，不表示数据库仍保留一条 deleted 记录。
type DeleteBrowserEnvResponse struct {
	EnvID        string `json:"envId"`
	Status       string `json:"status"`
	ActionStatus string `json:"actionStatus"`
	Message      string `json:"message"`
	DeletedAt    int64  `json:"deletedAt"`
	UpdatedAt    int64  `json:"updatedAt"`
}

// DeleteBrowserEnvImageResponse 是删除环境包关联 Docker 镜像后的响应摘要。
//
// 设计来源：
// - 用户要求把镜像删除与环境包销毁拆成两个独立端点，避免镜像清理时误删整个环境包；
// - 镜像删除只影响本机 Docker，不碰环境包目录、browser-data/profile 和 SQLite 索引；
// - 如果同一镜像被其他环境包引用，Docker 会拒绝删除，结果里会包含镜像仍被使用的提示。
type DeleteBrowserEnvImageResponse struct {
	EnvID          string                       `json:"envId"`
	Image          string                       `json:"image"`
	ImageRemoved   bool                         `json:"imageRemoved"`
	Results        []DockerImageRemoveResultRef `json:"results,omitempty"`
	WarningMessage string                       `json:"warningMessage,omitempty"`
	DeletedAt      int64                        `json:"deletedAt"`
}

// DockerImageRemoveResultRef 是镜像删除结果的引用摘要。
//
// 这里复用 Edge Models 里的 DockerImageRemoveResult 结构，但作为独立引用避免循环依赖。
type DockerImageRemoveResultRef struct {
	Image    string `json:"image"`
	Deleted  string `json:"deleted,omitempty"`
	Untagged string `json:"untagged,omitempty"`
	Error    string `json:"error,omitempty"`
}

// ImportBrowserEnvPackageResponse 是导入环境包后的摘要。
//
// 设计来源：
// - 备份/导出包会携带原 envId，但导入到本机时端口和 envSequence 属于本机资源，必须重新分配；
// - 导入只恢复环境包文件和 SQLite 索引，不启动 Docker，不沿用旧容器 ID；
// - timezone 会在下一次 run 时通过容器内 provider 重新确认，因此这里返回 status=created。
type ImportBrowserEnvPackageResponse struct {
	EnvID       string          `json:"envId"`
	UserID      string          `json:"userId"`
	RPAType     string          `json:"rpaType"`
	EnvSequence int             `json:"envSequence"`
	Ports       BrowserEnvPorts `json:"ports"`
	EnvPath     string          `json:"envPath"`
	Status      string          `json:"status"`
	ImportedAt  int64           `json:"importedAt"`
}

// BackupBrowserEnvResponse 是备份环境包后的资产状态摘要。
//
// 设计来源：
// - 新的备份不再是“打包并下载”，而是状态变化动作；
// - 备份成功后会删除 Docker 容器和 browser-envs 源目录，但 SQLite 索引必须保留；
// - 前端依靠这些字段展示“已备份、可恢复、可下载”的环境资产。
type BackupBrowserEnvResponse struct {
	EnvID          string `json:"envId"`
	UserID         string `json:"userId"`
	RPAType        string `json:"rpaType"`
	Status         string `json:"status"`
	BackupPath     string `json:"backupPath"`
	BackupChecksum string `json:"backupChecksum"`
	BackupSize     int64  `json:"backupSize"`
	BackupAt       int64  `json:"backupAt"`
	Message        string `json:"message"`
}

// RestoreBrowserEnvResponse 是从本机备份包恢复环境目录后的摘要。
//
// restore 不启动 Docker，只把备份包恢复为可运行环境包，并把容器运行态重置为 created。
// 后续 run 会重新创建容器并再次执行 timezone 探测。
type RestoreBrowserEnvResponse struct {
	EnvID      string          `json:"envId"`
	UserID     string          `json:"userId"`
	RPAType    string          `json:"rpaType"`
	Status     string          `json:"status"`
	Ports      BrowserEnvPorts `json:"ports"`
	EnvPath    string          `json:"envPath"`
	RestoredAt int64           `json:"restoredAt"`
	Message    string          `json:"message"`
}

// BrowserEnvRebuildCandidate 是重建 SQLite 索引前的只读候选项。
//
// 设计来源：
// - 用户要求 SQLite 丢失时可以从本机环境包文件恢复索引；
// - 但候选扫描只能诊断，不能把坏目录自动写进系统；
// - 因此这里明确给出 status/errors/indexed，让管理员先看见原因，再决定是否逐个 rebuild-index。
type BrowserEnvRebuildCandidate struct {
	EnvID   string   `json:"envId"`
	UserID  string   `json:"userId"`
	RPAType string   `json:"rpaType"`
	Name    string   `json:"name"`
	EnvPath string   `json:"envPath"`
	Status  string   `json:"status"`
	Indexed bool     `json:"indexed"`
	Errors  []string `json:"errors"`
}

type BrowserEnvRebuildCandidatesResponse struct {
	Total int                          `json:"total"`
	Items []BrowserEnvRebuildCandidate `json:"items"`
}

// BrowserEnvRebuildIndexResponse 是单个环境包重建 SQLite 索引后的结果。
//
// rebuild-index 不启动容器、不拉镜像、不验证最终网络指纹；它只把原子完整目录重新纳入 SQLite 索引。
type BrowserEnvRebuildIndexResponse struct {
	EnvID     string          `json:"envId"`
	UserID    string          `json:"userId"`
	RPAType   string          `json:"rpaType"`
	EnvPath   string          `json:"envPath"`
	Status    string          `json:"status"`
	Ports     BrowserEnvPorts `json:"ports"`
	RebuiltAt int64           `json:"rebuiltAt"`
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

// UpdateBrowserEnvProxyResponse 是代理配置修改后的摘要。
//
// 响应不返回代理正文，只返回大小和 binding 版本，避免接口把代理敏感内容反吐给前端。
type UpdateBrowserEnvProxyResponse struct {
	EnvID           string                `json:"envId"`
	Status          string                `json:"status"`
	Image           string                `json:"image"`
	Proxy           BrowserEnvProxyDetail `json:"proxy"`
	BindingVersion  int                   `json:"bindingVersion"`
	IdentityHash    string                `json:"identityHash"`
	Changed         bool                  `json:"changed"`
	RestartRequired bool                  `json:"restartRequired"`
	// RestartQueued 表示本次修改已经把运行中环境放入后台重建队列。
	//
	// 设计来源：
	// - rule 模式 timezone 需要通过 CDP 请求外部 provider，耗时不可控；
	// - 如果 PATCH proxy 同步等待 Docker 重建和 timezone，前端/Apifox 容易出现 socket hang up；
	// - 因此 running 环境改为快速返回，后台串行 forceRecreate。
	RestartQueued bool `json:"restartQueued"`
	// TaskID 是后台重建任务 ID。
	//
	// running 环境修改代理时返回该字段，前端可用 EventsURL 连接 SSE 观察 Docker 重建和 timezone 探测进度。
	TaskID    string `json:"taskId,omitempty"`
	EventsURL string `json:"eventsUrl,omitempty"`
	// Restarted 表示本次修改已由边缘服务同步完成容器重建。
	//
	// 用户明确要求配置修改后的启动过程对使用者无感知；
	// 该字段保留给同步重建场景；当前 PATCH proxy 的 running 重建主要通过 RestartQueued 表达。
	Restarted bool                   `json:"restarted"`
	Run       *RunBrowserEnvResponse `json:"run,omitempty"`
	UpdatedAt int64                  `json:"updatedAt"`
}

// StatusSyncSnapshot 是后台状态同步任务的健康快照。
//
// 设计来源：
// - 用户要求定时任务必须有哨兵机制，挂掉后自动拉起；
// - 任务是否健康不能只留在日志里，/health 和后续调试接口需要能看到 Worker/Watchdog 的状态；
// - 这里只暴露任务元信息和最近一轮统计，不暴露任何代理配置、指纹或登录态数据。
type StatusSyncSnapshot struct {
	Enabled           bool    `json:"enabled"`
	WorkerRunning     bool    `json:"workerRunning"`
	Restarts          int64   `json:"restarts"`
	IntervalSeconds   int     `json:"intervalSeconds"`
	WatchdogSeconds   int     `json:"watchdogSeconds"`
	StaleSeconds      int     `json:"staleSeconds"`
	LastStartedAt     *int64  `json:"lastStartedAt,omitempty"`
	LastHeartbeatAt   *int64  `json:"lastHeartbeatAt,omitempty"`
	LastFinishedAt    *int64  `json:"lastFinishedAt,omitempty"`
	LastSuccessAt     *int64  `json:"lastSuccessAt,omitempty"`
	LastError         *string `json:"lastError,omitempty"`
	LastPanic         *string `json:"lastPanic,omitempty"`
	LastSkippedReason *string `json:"lastSkippedReason,omitempty"`
	LastScannedCount  int     `json:"lastScannedCount"`
	LastUpdatedCount  int     `json:"lastUpdatedCount"`
}
