package Slot

// StatusWaiting 表示 slot 当前空闲，允许被分配。
const StatusWaiting = "waiting"

// StatusLoading 表示 slot 正在加载 package。
const StatusLoading = "loading"

// StatusOccupied 表示 slot 已经被某个 package 占用。
const StatusOccupied = "occupied"

// StatusReleasing 表示 slot 正在释放和清理现场。
const StatusReleasing = "releasing"

// Slot 是新 Client 里的 slot 主模型。
//
// 设计来源：
// - 这次新模型已经明确 slot 是一等资源对象，不只是“数量”；
// - Node Server 会按 slotId 指定运行位置，Client 负责执行和上报当前态；
// - WebVNC 也已经改为 slot 视角，因此 slot 本身必须成为稳定的模型对象。
//
// 职责边界：
// - 这里只表达本机某个 slot 当前的运行事实；
// - 不保存长期历史审计，长期历史归 Node Server；
// - 不表达 package 资产完整性，只表达当前资源位状态。
type Slot struct {
	SlotID           string  `json:"slotId"`
	Status           string  `json:"status"`
	CurrentPackageID *string `json:"currentPackageId,omitempty"`
	CurrentRunID     *string `json:"currentRunId,omitempty"`
	ContainerID      *string `json:"containerId,omitempty"`
	ContainerName    *string `json:"containerName,omitempty"`
	RuntimeImage     *string `json:"runtimeImage,omitempty"`
	ContainerStatus  *string `json:"containerStatus,omitempty"`
	CDPPort          *int    `json:"cdpPort,omitempty"`
	VNCPort          *int    `json:"vncPort,omitempty"`
	LastError        *string `json:"lastError,omitempty"`
	InitializedAt    int64   `json:"initializedAt"`
	UpdatedAt        int64   `json:"updatedAt"`
}
