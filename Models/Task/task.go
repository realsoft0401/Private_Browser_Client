package Task

// StatusQueued 表示任务已创建，等待执行。
const StatusQueued = "queued"

// StatusRunning 表示任务正在执行。
const StatusRunning = "running"

// StatusSuccess 表示任务已成功完成。
const StatusSuccess = "success"

// StatusFailed 表示任务已失败结束。
const StatusFailed = "failed"

// EventProgress 表示任务阶段性进度事件。
const EventProgress = "task.progress"

// EventCompleted 表示任务成功完成事件。
const EventCompleted = "task.completed"

// EventFailed 表示任务失败结束事件。
const EventFailed = "task.failed"

// Event 是正式 SSE 任务流对外统一暴露的最小事件模型。
//
// 设计来源：
// - 文档已经收紧，所有长链路动作都要通过统一 task 事件协议对外暴露；
// - 当前先以最小公共字段实现，避免每条接口各自发一套不同事件结构；
// - 后续就算增加更多业务字段，也应继续向后兼容这套基础骨架。
type Event struct {
	Event        string `json:"event"`
	TaskID       string `json:"taskId"`
	TaskType     string `json:"taskType"`
	ResourceType string `json:"resourceType"`
	ResourceID   string `json:"resourceId"`
	EnvID        string `json:"envId,omitempty"`
	SlotID       string `json:"slotId,omitempty"`
	Stage        string `json:"stage"`
	Status       string `json:"status"`
	Message      string `json:"message,omitempty"`
	Error        string `json:"error,omitempty"`
	Suggestion   string `json:"suggestion,omitempty"`
	Timestamp    string `json:"timestamp"`
}

// GetEvent 返回 SSE 外层的事件名。
//
// 当前统一保留这个方法，是为了让 SSE 输出层不需要知道具体事件模型结构，
// 后续如果替换底层实现，也能继续复用相同的写流逻辑。
func (e Event) GetEvent() string {
	return e.Event
}
