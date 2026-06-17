package Runtime

// EventSlotCreated 表示 slot 已创建并进入可用态。
const EventSlotCreated = "slot_created"

// EventSlotStatusChanged 表示 slot 当前状态变化。
const EventSlotStatusChanged = "slot_status_changed"

// EventRunStarted 表示运行关系已开始。
const EventRunStarted = "run_started"

// EventRunFinished 表示运行关系已结束。
const EventRunFinished = "run_finished"

// EventSlotReinitialized 表示 slot 已完成重初始化。
const EventSlotReinitialized = "slot_reinitialized"

// SSEEvent 是 Client 向外推送的最小事件模型。
//
// 当前阶段先把事件公共骨架定住，后续具体 SSE 实现继续按这套字段扩展。
type SSEEvent struct {
	Event      string         `json:"event"`
	SlotID     string         `json:"slotId,omitempty"`
	RunID      string         `json:"runId,omitempty"`
	PackageID  string         `json:"packageId,omitempty"`
	FromStatus string         `json:"fromStatus,omitempty"`
	ToStatus   string         `json:"toStatus,omitempty"`
	Result     string         `json:"result,omitempty"`
	Error      string         `json:"error,omitempty"`
	Data       map[string]any `json:"data,omitempty"`
	OccurredAt int64          `json:"occurredAt"`
}
