package Edge

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	model "private_browser_client/Models/Edge"
	taskModel "private_browser_client/Models/Task"
	"private_browser_client/Pkg/HttpResponse"
	taskService "private_browser_client/Service/Task"
)

// GetDeviceInfo 是 `/api/v1/edge/device-info` 的 HTTP 入口。
//
// 当前阶段先保持 old 的组织习惯：HTTP 入口放在 Service/Edge/http.go，
// 路由层只负责注册，不在 Routes 里直接拼业务响应。
func GetDeviceInfo(c *gin.Context) {
	result, err := NewEdgeService().GetDeviceInfo()
	if err != nil {
		HttpResponse.ResponseErrorWithMsg(c, HttpResponse.CodeRemoteError, err.Error())
		return
	}
	HttpResponse.ResponseSuccess(c, result)
}

// GetDockerStatus 返回本机 Docker 健康摘要。
func GetDockerStatus(c *gin.Context) {
	result, err := NewEdgeService().GetDockerStatus()
	if err != nil {
		HttpResponse.ResponseErrorWithMsg(c, HttpResponse.CodeRemoteError, err.Error())
		return
	}
	HttpResponse.ResponseSuccess(c, result)
}

// GetDockerImages 返回本机 Docker 镜像摘要列表。
func GetDockerImages(c *gin.Context) {
	result, err := NewEdgeService().GetDockerImages()
	if err != nil {
		HttpResponse.ResponseErrorWithMsg(c, HttpResponse.CodeRemoteError, err.Error())
		return
	}
	HttpResponse.ResponseSuccess(c, result)
}

// GetDockerContainers 返回本项目相关的本机容器摘要列表。
func GetDockerContainers(c *gin.Context) {
	result, err := NewEdgeService().GetDockerContainers()
	if err != nil {
		HttpResponse.ResponseErrorWithMsg(c, HttpResponse.CodeRemoteError, err.Error())
		return
	}
	HttpResponse.ResponseSuccess(c, result)
}

// PullDockerImage 是 `POST /api/v1/edge/docker/pull-image` 的 HTTP 入口。
//
// 镜像拉取是长动作，因此这里只接单并返回 `taskId/eventsUrl`，真实过程走 SSE。
func PullDockerImage(c *gin.Context) {
	var request model.PullImageRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		HttpResponse.ResponseErrorWithMsg(c, HttpResponse.CodeInvalidParams, "请求体解析失败，请检查 image")
		return
	}
	request.Image = strings.TrimSpace(request.Image)
	if request.Image == "" {
		HttpResponse.ResponseErrorWithMsg(c, HttpResponse.CodeInvalidParams, "image 不能为空")
		return
	}

	taskID := taskService.GetService().CreateTask("docker_pull_image", "docker_image", request.Image)
	go runPullDockerImageTask(taskID, request.Image)

	HttpResponse.ResponseSuccess(c, gin.H{
		"taskId":       taskID,
		"taskType":     "docker_pull_image",
		"resourceType": "docker_image",
		"resourceId":   request.Image,
		"eventsUrl":    "/api/v1/edge/tasks/" + taskID + "/events",
	})
}

// RemoveDockerImage 是 `POST /api/v1/edge/docker/remove-image` 的 HTTP 入口。
func RemoveDockerImage(c *gin.Context) {
	var request model.RemoveImageRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		HttpResponse.ResponseErrorWithMsg(c, HttpResponse.CodeInvalidParams, "请求体解析失败，请检查 image/force/noPrune")
		return
	}
	request.Image = strings.TrimSpace(request.Image)
	if request.Image == "" {
		HttpResponse.ResponseErrorWithMsg(c, HttpResponse.CodeInvalidParams, "image 不能为空")
		return
	}

	taskID := taskService.GetService().CreateTask("docker_remove_image", "docker_image", request.Image)
	go runRemoveDockerImageTask(taskID, &request)

	HttpResponse.ResponseSuccess(c, gin.H{
		"taskId":       taskID,
		"taskType":     "docker_remove_image",
		"resourceType": "docker_image",
		"resourceId":   request.Image,
		"eventsUrl":    "/api/v1/edge/tasks/" + taskID + "/events",
	})
}

func runPullDockerImageTask(taskID string, image string) {
	publisher := taskService.GetService()
	taskType := "docker_pull_image"

	_ = publisher.PublishProgress(taskID, newDockerImageTaskEvent(taskModel.EventProgress, taskID, taskType, image, "validate_request", taskModel.StatusQueued, "task accepted", "", ""))
	edge := NewEdgeService()
	_ = publisher.PublishProgress(taskID, newDockerImageTaskEvent(taskModel.EventProgress, taskID, taskType, image, "check_docker", taskModel.StatusRunning, "checking docker", "", ""))
	if _, err := edge.GetDockerStatus(); err != nil {
		_ = publisher.PublishFailed(taskID, newDockerImageTaskEvent(taskModel.EventFailed, taskID, taskType, image, "check_docker_failed", taskModel.StatusFailed, "pull image failed", err.Error(), "check docker api availability first"))
		return
	}

	_ = publisher.PublishProgress(taskID, newDockerImageTaskEvent(taskModel.EventProgress, taskID, taskType, image, "pull_image", taskModel.StatusRunning, "pulling docker image", "", ""))
	events, err := edge.PullDockerImage(image)
	if err != nil {
		_ = publisher.PublishFailed(taskID, newDockerImageTaskEvent(taskModel.EventFailed, taskID, taskType, image, "finalize_failed", taskModel.StatusFailed, "pull image failed", err.Error(), "check registry auth or network connectivity"))
		return
	}

	for _, event := range events {
		message := strings.TrimSpace(event.Status)
		if event.Error != "" {
			message = event.Error
		}
		if message == "" {
			continue
		}
		_ = publisher.PublishProgress(taskID, newDockerImageTaskEvent(taskModel.EventProgress, taskID, taskType, image, "stream_progress", taskModel.StatusRunning, message, "", ""))
	}
	_ = publisher.PublishCompleted(taskID, newDockerImageTaskEvent(taskModel.EventCompleted, taskID, taskType, image, "finalize_success", taskModel.StatusSuccess, "docker image pulled", "", ""))
}

func runRemoveDockerImageTask(taskID string, request *model.RemoveImageRequest) {
	publisher := taskService.GetService()
	taskType := "docker_remove_image"
	image := ""
	if request != nil {
		image = strings.TrimSpace(request.Image)
	}

	_ = publisher.PublishProgress(taskID, newDockerImageTaskEvent(taskModel.EventProgress, taskID, taskType, image, "validate_request", taskModel.StatusQueued, "task accepted", "", ""))
	edge := NewEdgeService()
	_ = publisher.PublishProgress(taskID, newDockerImageTaskEvent(taskModel.EventProgress, taskID, taskType, image, "check_docker", taskModel.StatusRunning, "checking docker", "", ""))
	if _, err := edge.GetDockerStatus(); err != nil {
		_ = publisher.PublishFailed(taskID, newDockerImageTaskEvent(taskModel.EventFailed, taskID, taskType, image, "check_docker_failed", taskModel.StatusFailed, "remove image failed", err.Error(), "check docker api availability first"))
		return
	}

	_ = publisher.PublishProgress(taskID, newDockerImageTaskEvent(taskModel.EventProgress, taskID, taskType, image, "remove_image", taskModel.StatusRunning, "removing docker image", "", ""))
	if _, err := edge.RemoveDockerImage(request); err != nil {
		_ = publisher.PublishFailed(taskID, newDockerImageTaskEvent(taskModel.EventFailed, taskID, taskType, image, "finalize_failed", taskModel.StatusFailed, "remove image failed", err.Error(), "stop referencing containers or retry with force if appropriate"))
		return
	}
	_ = publisher.PublishCompleted(taskID, newDockerImageTaskEvent(taskModel.EventCompleted, taskID, taskType, image, "finalize_success", taskModel.StatusSuccess, "docker image removed", "", ""))
}

func newDockerImageTaskEvent(eventName string, taskID string, taskType string, image string, stage string, status string, message string, eventError string, suggestion string) taskModel.Event {
	return taskModel.Event{
		Event:        eventName,
		TaskID:       taskID,
		TaskType:     taskType,
		ResourceType: "docker_image",
		ResourceID:   image,
		Stage:        stage,
		Status:       status,
		Message:      message,
		Error:        eventError,
		Suggestion:   suggestion,
		Timestamp:    time.Now().Format(time.RFC3339),
	}
}
