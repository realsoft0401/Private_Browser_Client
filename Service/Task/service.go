package Task

import (
	"fmt"
	"strings"
	"sync"
	"time"

	model "private_browser_client/Models/Task"
	common "private_browser_client/Repository/Common"
)

type record struct {
	id           string
	taskType     string
	resourceType string
	resourceID   string
	createdAt    string
	updatedAt    string
	finishedAt   string

	mu          sync.Mutex
	events      []model.Event
	done        bool
	subscribers map[int]chan model.Event
	nextSubID   int
}

// snapshot 是订阅前给 SSE 输出的静态快照。
// Snapshot 是订阅前返回给调用方的事件快照。
//
// 它只暴露“当前已经发生过哪些事件、任务是否已经结束”这两个稳定事实，
// 不把底层 record 或 subscriber 细节泄露给调用方。
type Snapshot struct {
	Events []model.Event
	Done   bool
}

// Service 管理当前 Client 进程内的长链路任务事件。
//
// 职责边界：
// - 只管理当前进程内短期可见的任务事件流；
// - 不承担长期任务历史归档；
// - 不替代 Node Server 的中心任务事实源。
type Service struct {
	mu    sync.RWMutex
	tasks map[string]*record
}

var (
	defaultService *Service
	once           sync.Once
)

// GetService 返回当前进程内共享的 task 事件服务。
func GetService() *Service {
	once.Do(func() {
		defaultService = &Service{
			tasks: make(map[string]*record),
		}
	})
	return defaultService
}

// CreateTask 创建一条新的进程内任务记录。
func (s *Service) CreateTask(taskType string, resourceType string, resourceID string) string {
	taskID := fmt.Sprintf("edge-task-%d", time.Now().UnixNano())
	now := time.Now().Format(time.RFC3339)
	item := &record{
		id:           taskID,
		taskType:     taskType,
		resourceType: resourceType,
		resourceID:   resourceID,
		createdAt:    now,
		updatedAt:    now,
		subscribers:  make(map[int]chan model.Event),
	}

	s.mu.Lock()
	s.tasks[taskID] = item
	s.mu.Unlock()
	return taskID
}

// PublishProgress 发布阶段性进度事件。
func (s *Service) PublishProgress(taskID string, event model.Event) error {
	return s.publish(taskID, event, false)
}

// PublishCompleted 发布成功完成事件，并关闭任务订阅。
func (s *Service) PublishCompleted(taskID string, event model.Event) error {
	return s.publish(taskID, event, true)
}

// PublishFailed 发布失败完成事件，并关闭任务订阅。
func (s *Service) PublishFailed(taskID string, event model.Event) error {
	return s.publish(taskID, event, true)
}

func (s *Service) publish(taskID string, event model.Event, markDone bool) error {
	item, err := s.getRecord(taskID)
	if err != nil {
		return err
	}

	item.mu.Lock()
	defer item.mu.Unlock()

	item.events = append(item.events, event)
	item.updatedAt = event.Timestamp
	// import-package 接单时还不知道 envId，只有后续读取 profile.json 后事件里才会带上。
	// 这里把非空 resourceId 回填到任务摘要，保证 Node Server 轮询 Edge task detail 时能拿到
	// 最终 envId，再刷新中心 server_browser_envs；不回填会导致导入成功但中心无法落库。
	if strings.TrimSpace(item.resourceID) == "" {
		if strings.TrimSpace(event.ResourceID) != "" {
			item.resourceID = strings.TrimSpace(event.ResourceID)
		} else if strings.TrimSpace(event.EnvID) != "" {
			item.resourceID = strings.TrimSpace(event.EnvID)
		}
	}
	for _, subscriber := range item.subscribers {
		select {
		case subscriber <- event:
		default:
		}
	}
	if markDone && !item.done {
		item.done = true
		item.finishedAt = event.Timestamp
		for id, subscriber := range item.subscribers {
			close(subscriber)
			delete(item.subscribers, id)
		}
	}
	return nil
}

// Subscribe 返回任务当前快照和后续事件流。
func (s *Service) Subscribe(taskID string) (Snapshot, <-chan model.Event, func(), error) {
	item, err := s.getRecord(taskID)
	if err != nil {
		return Snapshot{}, nil, nil, err
	}

	item.mu.Lock()
	defer item.mu.Unlock()

	base := Snapshot{
		Events: append([]model.Event(nil), item.events...),
		Done:   item.done,
	}
	if item.done {
		return base, nil, func() {}, nil
	}

	item.nextSubID++
	subID := item.nextSubID
	channel := make(chan model.Event, 16)
	item.subscribers[subID] = channel

	cancel := func() {
		item.mu.Lock()
		defer item.mu.Unlock()
		if subscriber, ok := item.subscribers[subID]; ok {
			close(subscriber)
			delete(item.subscribers, subID)
		}
	}
	return base, channel, cancel, nil
}

func (s *Service) getRecord(taskID string) (*record, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.tasks[taskID]
	if !ok {
		return nil, common.ErrNotFound
	}
	return item, nil
}

// GetDetail 返回当前进程内任务的最小摘要。
//
// 设计来源：
// - SSE 只适合实时订阅，前端刷新后仍需要一个普通 HTTP 入口回看当前任务；
// - Client 任务不做持久化，但在当前进程存活期间应能返回稳定摘要；
// - 这层只返回最小必要字段，不把完整事件数组默认回给普通详情接口。
func (s *Service) GetDetail(taskID string) (*model.DetailResponse, error) {
	item, err := s.getRecord(taskID)
	if err != nil {
		return nil, err
	}

	item.mu.Lock()
	defer item.mu.Unlock()

	currentStage := ""
	message := ""
	status := model.StatusQueued
	lastError := ""
	suggestion := ""
	if len(item.events) > 0 {
		last := item.events[len(item.events)-1]
		currentStage = last.Stage
		message = last.Message
		status = last.Status
		lastError = last.Error
		suggestion = last.Suggestion
	}
	if status == "" {
		if item.done {
			status = model.StatusSuccess
		} else {
			status = model.StatusQueued
		}
	}

	return &model.DetailResponse{
		TaskID:       item.id,
		TaskType:     item.taskType,
		ResourceType: item.resourceType,
		ResourceID:   item.resourceID,
		Status:       status,
		CurrentStage: currentStage,
		Message:      message,
		EventsURL:    fmt.Sprintf("/api/v1/edge/tasks/%s/events", item.id),
		CreatedAt:    item.createdAt,
		UpdatedAt:    item.updatedAt,
		FinishedAt:   item.finishedAt,
		Error:        lastError,
		Suggestion:   suggestion,
	}, nil
}
