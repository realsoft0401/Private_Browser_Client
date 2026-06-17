package Runtime

// StatusLoading 表示当前运行关系已开始加载。
const StatusLoading = "loading"

// StatusRunning 表示当前运行关系已稳定进入运行中。
const StatusRunning = "running"

// StatusEnding 表示当前运行关系正在结束和收尾。
const StatusEnding = "ending"

// RuntimeRelation 是当前 `package <-> slot` 运行关系模型。
//
// 设计来源：
// - 新模型已经明确 package、slot、runtime relation 是三层对象；
// - slot 状态和 package 状态允许松耦合，因此必须保留独立运行关系层；
// - Client 只维护当前现场，不维护完整历史。
//
// 职责边界：
// - 只表达当前一次运行关系；
// - 不承担长期审计归档；
// - 不保存 package 长期资产内容和 slot 历史统计。
type RuntimeRelation struct {
	RunID     string  `json:"runId"`
	PackageID string  `json:"packageId"`
	SlotID    string  `json:"slotId"`
	Status    string  `json:"status"`
	LastError *string `json:"lastError,omitempty"`
	StartedAt int64   `json:"startedAt"`
	UpdatedAt int64   `json:"updatedAt"`
}
