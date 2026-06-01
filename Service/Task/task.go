package Task

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"private_browser_client/Pkg/HttpResponse"
)

const (
	taskStatusQueued  = "queued"
	taskStatusRunning = "running"
	taskStatusSuccess = "success"
	taskStatusFailed  = "failed"
)

const taskEventLimit = 200

// EdgeTask 表示边缘服务内存中的一个长动作任务。
//
// 设计来源：
// - 用户实测 Docker pull、timezone 设置、running 环境代理重建这类动作会超过普通 HTTP 请求可承受时间；
// - 第一阶段先用内存任务中心验证 SSE 交互，避免为了一个试点提前引入任务表和持久化迁移；
// - 后续如果中心端需要审计、断点恢复或跨进程查询，再把这个模型下沉到 SQLite/Server 任务表。
//
// 职责边界：
// - 只保存任务摘要、最近事件和当前进程内订阅者；
// - 不保存代理明文、浏览器登录态、profile raw 等敏感内容；
// - 服务重启后任务丢失是当前阶段的明确取舍，OpenAPI/README 也会按“实时观察通道”记录。
type EdgeTask struct {
	ID           string                          `json:"taskId"`
	Type         string                          `json:"taskType"`
	Status       string                          `json:"status"`
	ResourceType string                          `json:"resourceType"`
	ResourceID   string                          `json:"resourceId"`
	Message      string                          `json:"message"`
	LastError    string                          `json:"lastError,omitempty"`
	Result       any                             `json:"result,omitempty"`
	CreatedAt    int64                           `json:"createdAt"`
	UpdatedAt    int64                           `json:"updatedAt"`
	Events       []EdgeTaskEvent                 `json:"events,omitempty"`
	subscribers  map[chan EdgeTaskEvent]struct{} `json:"-"`
}

// EdgeTaskEvent 是 SSE 推给前端的一条任务事件。
//
// event 字段用于前端区分 queued/running/progress/done/error/heartbeat；
// stage 表达当前业务阶段，例如 docker_pull、container_recreate、browser_env_stop。
type EdgeTaskEvent struct {
	TaskID    string         `json:"taskId"`
	Event     string         `json:"event"`
	Status    string         `json:"status"`
	Stage     string         `json:"stage"`
	Message   string         `json:"message"`
	Data      map[string]any `json:"data,omitempty"`
	CreatedAt int64          `json:"createdAt"`
}

// StartResponse 是所有 SSE 任务化接口的统一快速响应。
//
// 设计来源：
// - 多个动作接口都需要“HTTP 快速返回 + SSE 观察后台结果”；
// - 统一响应字段能让前端少写分支，也能让 OpenAPI 明确标注哪些接口是 SSE。
type StartResponse struct {
	TaskID       string `json:"taskId"`
	TaskType     string `json:"taskType"`
	Status       string `json:"status"`
	ResourceType string `json:"resourceType"`
	ResourceID   string `json:"resourceId"`
	EventsURL    string `json:"eventsUrl"`
	Message      string `json:"message"`
}

type hub struct {
	mu    sync.Mutex
	tasks map[string]*EdgeTask
}

var taskHub = &hub{tasks: make(map[string]*EdgeTask)}

// Create 创建一个内存任务并写入 queued 事件。
//
// 这里不接收任意 map 作为初始数据，是为了让任务元信息保持稳定；
// 业务进度细节应通过 Progress/Done 事件追加，避免任务摘要被不同接口随意扩展。
func Create(taskType, resourceType, resourceID, message string) *EdgeTask {
	now := time.Now().Unix()
	task := &EdgeTask{
		ID:           newTaskID(),
		Type:         strings.TrimSpace(taskType),
		Status:       taskStatusQueued,
		ResourceType: strings.TrimSpace(resourceType),
		ResourceID:   strings.TrimSpace(resourceID),
		Message:      strings.TrimSpace(message),
		CreatedAt:    now,
		UpdatedAt:    now,
		subscribers:  make(map[chan EdgeTaskEvent]struct{}),
	}
	taskHub.mu.Lock()
	taskHub.tasks[task.ID] = task
	taskHub.mu.Unlock()
	emit(task.ID, "queued", taskStatusQueued, "queued", task.Message, nil)
	return task
}

// NewStartResponse 把任务对象转换为统一接口响应。
//
// baseURL 来自当前 HTTP 请求，避免文档、Docker、反向代理部署时写死 127.0.0.1。
func NewStartResponse(task *EdgeTask, baseURL string) StartResponse {
	if task == nil {
		return StartResponse{}
	}
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	return StartResponse{
		TaskID:       task.ID,
		TaskType:     task.Type,
		Status:       task.Status,
		ResourceType: task.ResourceType,
		ResourceID:   task.ResourceID,
		EventsURL:    baseURL + "/api/v1/edge/tasks/" + task.ID + "/events",
		Message:      task.Message,
	}
}

func MarkRunning(taskID, stage, message string, data map[string]any) {
	emit(taskID, "running", taskStatusRunning, stage, message, data)
}

func Progress(taskID, stage, message string, data map[string]any) {
	emit(taskID, "progress", taskStatusRunning, stage, message, data)
}

func Heartbeat(taskID, stage, message string) {
	emit(taskID, "heartbeat", taskStatusRunning, stage, message, nil)
}

func Done(taskID, stage, message string, result any) {
	data := map[string]any(nil)
	if result != nil {
		data = map[string]any{"result": result}
	}
	emitWithResult(taskID, "done", taskStatusSuccess, stage, message, data, result)
}

func Failed(taskID, stage, message string) {
	emit(taskID, "error", taskStatusFailed, stage, message, nil)
}

// RunHeartbeat 在长动作执行期间定期推送心跳。
//
// 它只说明任务仍在执行，不代表业务阶段成功；调用方必须在真实动作结束后显式 Done/Failed。
func RunHeartbeat(ctx context.Context, taskID, stage, message string) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			Heartbeat(taskID, stage, message)
		}
	}
}

func emit(taskID, event, status, stage, message string, data map[string]any) {
	emitWithResult(taskID, event, status, stage, message, data, nil)
}

func emitWithResult(taskID, event, status, stage, message string, data map[string]any, result any) {
	taskHub.mu.Lock()
	task := taskHub.tasks[taskID]
	if task == nil {
		taskHub.mu.Unlock()
		return
	}
	now := time.Now().Unix()
	task.Status = status
	task.Message = strings.TrimSpace(message)
	task.UpdatedAt = now
	if status == taskStatusFailed {
		task.LastError = task.Message
	}
	if status == taskStatusSuccess {
		task.Result = result
	}
	taskEvent := EdgeTaskEvent{
		TaskID:    taskID,
		Event:     event,
		Status:    status,
		Stage:     strings.TrimSpace(stage),
		Message:   task.Message,
		Data:      data,
		CreatedAt: now,
	}
	task.Events = append(task.Events, taskEvent)
	if len(task.Events) > taskEventLimit {
		task.Events = task.Events[len(task.Events)-taskEventLimit:]
	}
	subscribers := make([]chan EdgeTaskEvent, 0, len(task.subscribers))
	for subscriber := range task.subscribers {
		subscribers = append(subscribers, subscriber)
	}
	taskHub.mu.Unlock()

	for _, subscriber := range subscribers {
		select {
		case subscriber <- taskEvent:
		default:
		}
	}
}

func get(taskID string) (*EdgeTask, bool) {
	taskHub.mu.Lock()
	defer taskHub.mu.Unlock()
	task := taskHub.tasks[strings.TrimSpace(taskID)]
	if task == nil {
		return nil, false
	}
	copyTask := *task
	copyTask.Events = append([]EdgeTaskEvent(nil), task.Events...)
	copyTask.subscribers = nil
	return &copyTask, true
}

func subscribe(taskID string) ([]EdgeTaskEvent, <-chan EdgeTaskEvent, func(), bool) {
	taskHub.mu.Lock()
	defer taskHub.mu.Unlock()
	task := taskHub.tasks[strings.TrimSpace(taskID)]
	if task == nil {
		return nil, nil, nil, false
	}
	ch := make(chan EdgeTaskEvent, 32)
	task.subscribers[ch] = struct{}{}
	history := append([]EdgeTaskEvent(nil), task.Events...)
	cancel := func() {
		taskHub.mu.Lock()
		defer taskHub.mu.Unlock()
		if current := taskHub.tasks[strings.TrimSpace(taskID)]; current != nil {
			delete(current.subscribers, ch)
		}
	}
	return history, ch, cancel, true
}

// GetEdgeTask 查询当前进程内任务摘要。
//
// 这是 SSE 的补充接口：前端如果错过实时事件，可以先查一次任务历史再决定是否继续订阅。
func GetEdgeTask(c *gin.Context) {
	task, ok := get(c.Param("taskId"))
	if !ok {
		HttpResponse.ResponseErrorWithMsg(c, HttpResponse.CodeNotFound, "任务不存在或服务已重启")
		return
	}
	HttpResponse.ResponseSuccess(c, task)
}

// StreamEdgeTaskEvents 以 SSE 形式输出任务事件流。
//
// 当前接口只用于动作型 API 的后台进度，不承载文件下载；备份/导出这类 gzip 文件流后续要设计 artifactUrl 后再任务化。
func StreamEdgeTaskEvents(c *gin.Context) {
	history, events, cancel, ok := subscribe(c.Param("taskId"))
	if !ok {
		HttpResponse.ResponseErrorWithMsg(c, HttpResponse.CodeNotFound, "任务不存在或服务已重启")
		return
	}
	defer cancel()

	w := c.Writer
	header := w.Header()
	header.Set("Content-Type", "text/event-stream; charset=utf-8")
	header.Set("Cache-Control", "no-cache")
	header.Set("Connection", "keep-alive")
	header.Set("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)

	for _, event := range history {
		writeSSE(c, event)
		if isTerminalEvent(event) {
			return
		}
	}

	heartbeat := time.NewTicker(20 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-c.Request.Context().Done():
			return
		case <-heartbeat.C:
			writeSSE(c, EdgeTaskEvent{
				TaskID:    c.Param("taskId"),
				Event:     "heartbeat",
				Status:    taskStatusRunning,
				Stage:     "sse",
				Message:   "SSE 连接保持中",
				CreatedAt: time.Now().Unix(),
			})
		case event := <-events:
			writeSSE(c, event)
			if isTerminalEvent(event) {
				return
			}
		}
	}
}

func writeSSE(c *gin.Context, event EdgeTaskEvent) {
	payload, err := json.Marshal(event)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintf(c.Writer, "event:%s\n", event.Event)
	_, _ = fmt.Fprintf(c.Writer, "data:%s\n\n", payload)
	if flusher, ok := c.Writer.(http.Flusher); ok {
		flusher.Flush()
	}
}

func isTerminalEvent(event EdgeTaskEvent) bool {
	return event.Status == taskStatusSuccess || event.Status == taskStatusFailed || event.Event == "done" || event.Event == "error"
}

func newTaskID() string {
	return fmt.Sprintf("task_%d_%d", time.Now().UnixNano(), time.Now().Nanosecond()%100000)
}
