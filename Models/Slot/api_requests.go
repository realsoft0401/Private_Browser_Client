package Slot

// CreateSlotRequest 是创建本机 slot 的请求体。
//
// 当前口径下，slot 由 Node Server 明确创建和销毁；
// Client 只负责按指定 slotId 建立本机资源位。
//
// 命名规则已经收口为固定三位编号：
// - `slot001`
// - `slot002`
// - `slot120`
//
// 不再接受：
// - `slot-1`
// - `slot-e2e-001`
// - 其它自由字符串
type CreateSlotRequest struct {
	SlotID string `json:"slotId" binding:"required"`
}

// DestroySlotRequest 是销毁本机 slot 的请求体。
//
// 当前阶段保留 force，是因为已经收口：
// slot 销毁前必须确认没有未结束运行关系，必要时允许强制结束后再销毁。
type DestroySlotRequest struct {
	Force bool `json:"force"`
}

// ReinitSlotRequest 是重初始化本机 slot 的请求体。
//
// 当前阶段先不额外扩参数，保留空结构是为了协议稳定。
type ReinitSlotRequest struct{}
