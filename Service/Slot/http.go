package Slot

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	edgeModel "private_browser_client/Models/Edge"
	model "private_browser_client/Models/Slot"
	taskModel "private_browser_client/Models/Task"
	"private_browser_client/Pkg/HttpResponse"
	common "private_browser_client/Repository/Common"
	edgeService "private_browser_client/Service/Edge"
	taskService "private_browser_client/Service/Task"
)

// ListSlots 返回当前 Client 维护的全部 slot 当前态。
func ListSlots(c *gin.Context) {
	result, err := NewService().ListSlots()
	if err != nil {
		responseRepositoryError(c, err)
		return
	}
	HttpResponse.ResponseSuccess(c, result)
}

// GetSlotByID 返回指定 slot 当前态。
func GetSlotByID(c *gin.Context) {
	result, err := NewService().GetSlotByID(c.Param("slotId"))
	if err != nil {
		responseRepositoryError(c, err)
		return
	}
	HttpResponse.ResponseSuccess(c, result)
}

// CreateSlot 是 create-slot 正式接口的 HTTP 入口。
//
// ******** 平台端接口接入说明：
// HTTP 层这里只负责解析请求和返回结果，不建议把平台端接口调用直接写在这里。
// 后续平台端放行校验、创建成功回告，都应继续放在 Service/CreateSlot 内部，
// 保持 Routes/HTTP 只做协议入口，避免平台逻辑散落到控制器层。
func CreateSlot(c *gin.Context) {
	var request model.CreateSlotRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		HttpResponse.ResponseErrorWithMsg(c, HttpResponse.CodeInvalidParams, "请求体解析失败，请检查 slotId")
		return
	}

	result, err := NewService().CreateSlot(request.SlotID)
	if err != nil {
		responseRepositoryError(c, err)
		return
	}
	c.JSON(http.StatusCreated, &HttpResponse.ResponseData{
		Code: HttpResponse.CodeSuccess,
		Msg:  HttpResponse.CodeSuccess.Msg(),
		Data: result,
	})
}

// DestroySlot 是 destroy-slot 正式接口的 HTTP 入口。
//
// 当前骨架阶段先执行“未占用才允许销毁”；force 参数先保留在协议里，
// 等后续真正接上容器重置和关系强制结束时，再把强制销毁链路补齐。
func DestroySlot(c *gin.Context) {
	var request model.DestroySlotRequest
	if err := c.ShouldBindJSON(&request); err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, http.ErrBodyNotAllowed) {
		HttpResponse.ResponseErrorWithMsg(c, HttpResponse.CodeInvalidParams, "请求体解析失败")
		return
	}

	service := NewService()
	slot, err := service.GetSlotByID(c.Param("slotId"))
	if err != nil {
		responseRepositoryError(c, err)
		return
	}
	if slot.Status != model.StatusWaiting && !request.Force {
		HttpResponse.ResponseErrorWithMsg(c, HttpResponse.CodeConflict, "slot 当前不是 waiting，不能直接销毁")
		return
	}
	if err = service.DeleteSlotByID(slot.SlotID); err != nil {
		responseRepositoryError(c, err)
		return
	}
	HttpResponse.ResponseSuccess(c, gin.H{
		"slotId": slot.SlotID,
		"status": "deleted",
	})
}

// ReinitSlot 是 reinit-slot 正式接口的 HTTP 入口。
func ReinitSlot(c *gin.Context) {
	result, err := NewService().ReinitSlot(c.Param("slotId"))
	if err != nil {
		responseRepositoryError(c, err)
		return
	}
	HttpResponse.ResponseSuccess(c, result)
}

// GetSlotVNCInfo 返回 slot 视角的 VNC/noVNC 连接信息。
func GetSlotVNCInfo(c *gin.Context) {
	result, err := NewService().GetSlotVNCInfo(c.Param("slotId"), publicRequestBase(c), publicWebSocketBase(c))
	if err != nil {
		responseRepositoryError(c, err)
		return
	}
	HttpResponse.ResponseSuccess(c, result)
}

// GetSlotCDPInfo 返回 slot 视角的 CDP 连接信息。
func GetSlotCDPInfo(c *gin.Context) {
	result, err := NewService().GetSlotCDPInfo(c.Param("slotId"), publicRequestBase(c))
	if err != nil {
		responseRepositoryError(c, err)
		return
	}
	HttpResponse.ResponseSuccess(c, result)
}

// ProxySlotVNC 代理 noVNC WebSocket 到 slot 对应的 VNC TCP 端口。
func ProxySlotVNC(c *gin.Context) {
	if err := NewService().ProxySlotVNC(c.Writer, c.Request, c.Param("slotId")); err != nil {
		if !c.Writer.Written() {
			responseRepositoryError(c, err)
		}
	}
}

// StartContainer 是 slot 容器运维诊断接口的 HTTP 入口。
//
// 这条接口只直接管理 slot 当前绑定的本机容器，不读取环境包资产，也不承诺 browser-env 已同步。
func StartContainer(c *gin.Context) {
	slotID := strings.TrimSpace(c.Param("slotId"))
	if slotID == "" {
		HttpResponse.ResponseErrorWithMsg(c, HttpResponse.CodeInvalidParams, "slotId 不能为空")
		return
	}

	taskID := taskService.GetService().CreateTask("docker_container_start", "docker_container", slotID)
	go runSlotContainerTask(taskID, slotID, "start", nil)

	HttpResponse.ResponseSuccess(c, buildContainerTaskAccepted(taskID, "docker_container_start", slotID))
}

// StopContainer 是 slot 容器停止诊断接口的 HTTP 入口。
func StopContainer(c *gin.Context) {
	slotID := strings.TrimSpace(c.Param("slotId"))
	if slotID == "" {
		HttpResponse.ResponseErrorWithMsg(c, HttpResponse.CodeInvalidParams, "slotId 不能为空")
		return
	}

	request, ok := bindOptionalContainerActionRequest(c)
	if !ok {
		return
	}

	taskID := taskService.GetService().CreateTask("docker_container_stop", "docker_container", slotID)
	go runSlotContainerTask(taskID, slotID, "stop", request)

	HttpResponse.ResponseSuccess(c, buildContainerTaskAccepted(taskID, "docker_container_stop", slotID))
}

// RestartContainer 是 slot 容器重启诊断接口的 HTTP 入口。
func RestartContainer(c *gin.Context) {
	slotID := strings.TrimSpace(c.Param("slotId"))
	if slotID == "" {
		HttpResponse.ResponseErrorWithMsg(c, HttpResponse.CodeInvalidParams, "slotId 不能为空")
		return
	}

	request, ok := bindOptionalContainerActionRequest(c)
	if !ok {
		return
	}

	taskID := taskService.GetService().CreateTask("docker_container_restart", "docker_container", slotID)
	go runSlotContainerTask(taskID, slotID, "restart", request)

	HttpResponse.ResponseSuccess(c, buildContainerTaskAccepted(taskID, "docker_container_restart", slotID))
}

func bindOptionalContainerActionRequest(c *gin.Context) (*edgeModel.ContainerActionRequest, bool) {
	request := new(edgeModel.ContainerActionRequest)
	if err := c.ShouldBindJSON(request); err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, http.ErrBodyNotAllowed) {
		HttpResponse.ResponseErrorWithMsg(c, HttpResponse.CodeInvalidParams, "请求体解析失败，请检查 timeoutSeconds")
		return nil, false
	}
	return request, true
}

func buildContainerTaskAccepted(taskID string, taskType string, slotID string) gin.H {
	return gin.H{
		"taskId":       taskID,
		"taskType":     taskType,
		"resourceType": "docker_container",
		"resourceId":   slotID,
		"eventsUrl":    "/api/v1/edge/tasks/" + taskID + "/events",
	}
}

func runSlotContainerTask(taskID string, slotID string, action string, request *edgeModel.ContainerActionRequest) {
	publisher := taskService.GetService()
	taskType := "docker_container_" + action

	_ = publisher.PublishProgress(taskID, newSlotTaskEvent(taskModel.EventProgress, taskID, taskType, slotID, "load_slot", taskModel.StatusQueued, "task accepted", "", ""))

	slot, err := NewService().GetSlotByID(slotID)
	if err != nil {
		_ = publisher.PublishFailed(taskID, newSlotTaskEvent(taskModel.EventFailed, taskID, taskType, slotID, "load_slot_failed", taskModel.StatusFailed, "slot container action failed", err.Error(), "check whether the slot exists"))
		return
	}

	containerID := ""
	if slot.ContainerID != nil {
		containerID = strings.TrimSpace(*slot.ContainerID)
	}
	if containerID == "" {
		_ = publisher.PublishFailed(taskID, newSlotTaskEvent(taskModel.EventFailed, taskID, taskType, slotID, "check_container_failed", taskModel.StatusFailed, "slot container action failed", "slot 没有关联容器", "recreate or reinitialize the slot first"))
		return
	}

	edge := edgeService.NewEdgeService()
	_ = publisher.PublishProgress(taskID, newSlotTaskEvent(taskModel.EventProgress, taskID, taskType, slotID, "check_docker", taskModel.StatusRunning, "checking docker", "", ""))
	if _, err = edge.GetDockerStatus(); err != nil {
		_ = publisher.PublishFailed(taskID, newSlotTaskEvent(taskModel.EventFailed, taskID, taskType, slotID, "check_docker_failed", taskModel.StatusFailed, "slot container action failed", err.Error(), "check docker api availability first"))
		return
	}

	stage := action + "_container"
	_ = publisher.PublishProgress(taskID, newSlotTaskEvent(taskModel.EventProgress, taskID, taskType, slotID, stage, taskModel.StatusRunning, "executing container action", "", ""))

	switch action {
	case "start":
		_, err = edge.StartDockerContainer(containerID)
	case "stop":
		_, err = edge.StopDockerContainer(containerID, request)
	case "restart":
		_, err = edge.RestartDockerContainer(containerID, request)
	default:
		err = errors.New("unsupported container action")
	}
	if err != nil {
		updateSlotContainerRuntimeAfterAction(slot, action, false, err)
		_ = publisher.PublishFailed(taskID, newSlotTaskEvent(taskModel.EventFailed, taskID, taskType, slotID, "finalize_failed", taskModel.StatusFailed, "slot container action failed", err.Error(), "check client logs and docker container state"))
		return
	}

	updateSlotContainerRuntimeAfterAction(slot, action, true, nil)
	_ = publisher.PublishCompleted(taskID, newSlotTaskEvent(taskModel.EventCompleted, taskID, taskType, slotID, "finalize_success", taskModel.StatusSuccess, "slot container action completed", "", ""))
}

func updateSlotContainerRuntimeAfterAction(slot *model.Slot, action string, success bool, err error) {
	if slot == nil {
		return
	}
	if success {
		slot.LastError = nil
		switch action {
		case "start", "restart":
			slot.ContainerStatus = optionalSlotString("running")
		case "stop":
			slot.ContainerStatus = optionalSlotString("stopped")
		}
	} else if err != nil {
		slot.LastError = optionalSlotString(err.Error())
	}
	_ = NewService().UpdateSlot(slot)
}

func newSlotTaskEvent(eventName string, taskID string, taskType string, slotID string, stage string, status string, message string, eventError string, suggestion string) taskModel.Event {
	return taskModel.Event{
		Event:        eventName,
		TaskID:       taskID,
		TaskType:     taskType,
		ResourceType: "docker_container",
		ResourceID:   slotID,
		SlotID:       slotID,
		Stage:        stage,
		Status:       status,
		Message:      message,
		Error:        eventError,
		Suggestion:   suggestion,
		Timestamp:    time.Now().Format(time.RFC3339),
	}
}

func publicRequestBase(c *gin.Context) string {
	scheme := "http"
	if c.Request.TLS != nil {
		scheme = "https"
	} else if forwardedProto := strings.TrimSpace(c.GetHeader("X-Forwarded-Proto")); forwardedProto != "" {
		scheme = forwardedProto
	}
	return scheme + "://" + c.Request.Host
}

func publicWebSocketBase(c *gin.Context) string {
	scheme := "ws"
	if c.Request.TLS != nil {
		scheme = "wss"
	} else if forwardedProto := strings.TrimSpace(c.GetHeader("X-Forwarded-Proto")); strings.EqualFold(forwardedProto, "https") {
		scheme = "wss"
	}
	return scheme + "://" + c.Request.Host
}

func responseRepositoryError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, common.ErrNotFound):
		HttpResponse.ResponseError(c, HttpResponse.CodeNotFound)
	case errors.Is(err, common.ErrDuplicate), errors.Is(err, common.ErrConflict):
		HttpResponse.ResponseError(c, HttpResponse.CodeConflict)
	default:
		HttpResponse.ResponseErrorWithMsg(c, HttpResponse.CodeServerBusy, err.Error())
	}
}

func optionalSlotString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}
