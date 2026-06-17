package Package

// StatusCreated 表示环境包资产已建立，但当前还没有进入运行态。
//
// 设计来源：
// - 文档已经收口，正式 browser_envs.status 不能再继续使用 `pending` 这种过渡命名；
// - 当前本地 SQLite 虽然暂时还只是运行摘要，但对外正式 API 已经统一按 browser-env 生命周期暴露；
// - 因此这里先把本机运行视图的未运行态统一映射成 `created`，避免协议层和存储层出现两套状态名。
const StatusCreated = "created"

// StatusRunning 表示 package 当前已在某个 slot 中运行。
const StatusRunning = "running"

// StatusStopped 表示环境包已经安全退出运行态。
const StatusStopped = "stopped"

// StatusBackedUp 表示环境包资产已经备份并释放了本机运行目录。
const StatusBackedUp = "backed_up"

// StatusDeleted 表示环境包资产已经被彻底删除。
const StatusDeleted = "deleted"

// StatusError 表示环境包运行或校验失败，需要管理员排查。
const StatusError = "error"

// RuntimeView 是 package 在 Client 本机的当前运行视图。
//
// 注意这里不是完整 package 资产模型，只是 Client 为 run/stop/查询当前态保留的本机摘要。
// 真正的 package 长期资产内容仍然在环境包目录和相关资产文件中。
type RuntimeView struct {
	PackageID     string  `json:"packageId"`
	CurrentRunID  *string `json:"currentRunId,omitempty"`
	CurrentSlotID *string `json:"currentSlotId,omitempty"`
	RuntimeStatus string  `json:"runtimeStatus"`
	LastRunAt     *int64  `json:"lastRunAt,omitempty"`
	LastStopAt    *int64  `json:"lastStopAt,omitempty"`
	LastError     *string `json:"lastError,omitempty"`
}
