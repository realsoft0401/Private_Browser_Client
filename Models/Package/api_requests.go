package Package

// RunPackageRequest 是 package 进入指定 slot 运行的请求体。
//
// 这里的核心收口是：不管第一次运行还是人工重跑，`slotId` 都是固定参数。
type RunPackageRequest struct {
	SlotID string `json:"slotId"`
}

// StopPackageRequest 是停止当前 package 运行关系的请求体。
//
// stop 也必须显式带 slotId，避免运行关系收口时产生歧义。
type StopPackageRequest struct {
	SlotID string `json:"slotId"`
}
