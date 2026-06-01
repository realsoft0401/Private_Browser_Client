package Edge

import (
	"context"
	"errors"
	"io"
	"strings"

	"github.com/gin-gonic/gin"

	edgeModel "private_browser_client/Models/Edge"
	"private_browser_client/Pkg/HttpResponse"
	TaskService "private_browser_client/Service/Task"
)

// GetDeviceInfo 返回边缘服务所在机器的设备能力。
//
// 这个 HTTP 入口只做协议层处理，不保存数据库，也不要求 nodeId。
// 这正是边缘服务和未来中心服务端的边界：边缘服务只说明“我这台机器是什么状态”。
func GetDeviceInfo(c *gin.Context) {
	result, err := NewEdgeService().GetDeviceInfo()
	if err != nil {
		HttpResponse.ResponseErrorWithMsg(c, HttpResponse.CodeRemoteError, err.Error())
		return
	}
	HttpResponse.ResponseSuccess(c, result)
}

// GetDockerStatus 返回本机 Docker 状态摘要。
//
// 它适合给前端或未来中心服务端做快速健康判断；完整镜像和容器列表后续通过独立接口扩展。
func GetDockerStatus(c *gin.Context) {
	result, err := NewEdgeService().GetDockerStatus()
	if err != nil {
		HttpResponse.ResponseErrorWithMsg(c, HttpResponse.CodeRemoteError, err.Error())
		return
	}
	HttpResponse.ResponseSuccess(c, result)
}

// GetDockerImages 返回本机 Docker 镜像列表。
//
// 这是边缘服务的只读接口，只访问本机 Docker 2375，不写数据库，也不处理用户或节点归属。
func GetDockerImages(c *gin.Context) {
	result, err := NewEdgeService().GetDockerImages()
	if err != nil {
		HttpResponse.ResponseErrorWithMsg(c, HttpResponse.CodeRemoteError, err.Error())
		return
	}
	HttpResponse.ResponseSuccess(c, result)
}

// GetDockerContainers 返回本机 Docker 容器列表。
//
// 这里默认包含停止容器，因为边缘服务后续需要支持对已停止容器执行 start 操作。
func GetDockerContainers(c *gin.Context) {
	result, err := NewEdgeService().GetDockerContainers()
	if err != nil {
		HttpResponse.ResponseErrorWithMsg(c, HttpResponse.CodeRemoteError, err.Error())
		return
	}
	HttpResponse.ResponseSuccess(c, result)
}

// PullDockerImage 拉取本机 Docker 镜像。
//
// 这个接口会改变本机 Docker 状态，因此只负责协议解析和调用 Service。
// 镜像名策略、哪些镜像允许拉取，后续应由中心服务端或配置下发，不写死在 HTTP 层。
// Docker pull 会返回长时间流式进度，HTTP 入口统一改为 SSE 任务，避免外部客户端等待超时。
func PullDockerImage(c *gin.Context) {
	param := new(edgeModel.PullImageRequest)
	if err := c.ShouldBindJSON(param); err != nil {
		HttpResponse.ResponseErrorWithMsg(c, HttpResponse.CodeInvalidParams, "请求参数格式错误")
		return
	}
	if strings.TrimSpace(param.Image) == "" {
		HttpResponse.ResponseErrorWithMsg(c, HttpResponse.CodeInvalidParams, "image 不能为空")
		return
	}
	task := TaskService.Create("docker_pull_image", "docker_image", pullImageResourceID(param), "Docker 镜像拉取任务已创建")
	go pullDockerImageTask(task.ID, param)
	HttpResponse.ResponseSuccess(c, TaskService.NewStartResponse(task, publicRequestBase(c)))
}

// RemoveDockerImage 删除本机 Docker 镜像。
//
// 这是本机管理动作，不涉及数据库和中心节点归属。
// 删除策略由请求体的 force / noPrune 显式控制，避免 handler 里出现隐式强删行为。
// 删除镜像可能等待 Docker 清理层数据，改为 SSE 任务后前端可以看到最终 Docker 返回结果。
func RemoveDockerImage(c *gin.Context) {
	param := new(edgeModel.RemoveImageRequest)
	if err := c.ShouldBindJSON(param); err != nil {
		HttpResponse.ResponseErrorWithMsg(c, HttpResponse.CodeInvalidParams, "请求参数格式错误")
		return
	}
	if strings.TrimSpace(param.Image) == "" {
		HttpResponse.ResponseErrorWithMsg(c, HttpResponse.CodeInvalidParams, "image 不能为空")
		return
	}
	task := TaskService.Create("docker_remove_image", "docker_image", strings.TrimSpace(param.Image), "Docker 镜像删除任务已创建")
	go removeDockerImageTask(task.ID, param)
	HttpResponse.ResponseSuccess(c, TaskService.NewStartResponse(task, publicRequestBase(c)))
}

// StartDockerContainer 启动本机 Docker 容器。
//
// 容器 ID 来自路径参数，handler 不做业务归属判断；边缘服务当前只面对本机 Docker。
// 后续中心服务端如需限制权限，应在调用边缘服务前完成鉴权和设备归属校验。
// 容器生命周期动作统一走任务，保持和环境包 run/stop 的前端处理方式一致。
func StartDockerContainer(c *gin.Context) {
	task := TaskService.Create("docker_container_start", "docker_container", c.Param("id"), "Docker 容器启动任务已创建")
	go startDockerContainerTask(task.ID, c.Param("id"))
	HttpResponse.ResponseSuccess(c, TaskService.NewStartResponse(task, publicRequestBase(c)))
}

// StopDockerContainer 停止本机 Docker 容器。
//
// 请求体可为空；为空时 Service 使用默认等待时间。
// 这里保留可选 JSON，是为了前端或中心服务端在需要快速停止时能显式传 timeoutSeconds。
// stop 可能等待 Docker grace period，因此通过 SSE 返回最终结果。
func StopDockerContainer(c *gin.Context) {
	param, ok := bindOptionalContainerActionRequest(c)
	if !ok {
		return
	}
	task := TaskService.Create("docker_container_stop", "docker_container", c.Param("id"), "Docker 容器停止任务已创建")
	go stopDockerContainerTask(task.ID, c.Param("id"), param)
	HttpResponse.ResponseSuccess(c, TaskService.NewStartResponse(task, publicRequestBase(c)))
}

// RestartDockerContainer 重启本机 Docker 容器。
//
// 这个 HTTP 入口只做参数绑定和响应封装，真正的 Docker API 状态码处理在 Service 中完成。
// restart 底层包含 stop/start 等待，也统一返回 SSE 任务。
func RestartDockerContainer(c *gin.Context) {
	param, ok := bindOptionalContainerActionRequest(c)
	if !ok {
		return
	}
	task := TaskService.Create("docker_container_restart", "docker_container", c.Param("id"), "Docker 容器重启任务已创建")
	go restartDockerContainerTask(task.ID, c.Param("id"), param)
	HttpResponse.ResponseSuccess(c, TaskService.NewStartResponse(task, publicRequestBase(c)))
}

// pullDockerImageTask 在后台拉取镜像并把 Docker 原始层进度转成 SSE progress。
//
// 这里刻意不把任务逻辑放进 Edge Service，避免 Docker API 适配层依赖 HTTP/SSE；
// Service 只提供 onEvent 回调，HTTP 层负责把它映射为前端可订阅的任务事件。
func pullDockerImageTask(taskID string, param *edgeModel.PullImageRequest) {
	TaskService.MarkRunning(taskID, "docker_pull", "开始拉取 Docker 镜像", map[string]any{"image": pullImageResourceID(param)})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go TaskService.RunHeartbeat(ctx, taskID, "docker_pull", "Docker 镜像仍在拉取")
	events, err := NewEdgeService().PullDockerImageWithProgress(param, func(event edgeModel.DockerPullEvent) {
		TaskService.Progress(taskID, "docker_pull", dockerPullEventMessage(event), map[string]any{"event": event})
	})
	if err != nil {
		TaskService.Failed(taskID, "docker_pull", err.Error())
		return
	}
	TaskService.Done(taskID, "docker_pull", "Docker 镜像拉取完成", events)
}

func removeDockerImageTask(taskID string, param *edgeModel.RemoveImageRequest) {
	TaskService.MarkRunning(taskID, "docker_remove_image", "开始删除 Docker 镜像", map[string]any{"image": strings.TrimSpace(param.Image)})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go TaskService.RunHeartbeat(ctx, taskID, "docker_remove_image", "Docker 镜像删除仍在执行")
	result, err := NewEdgeService().RemoveDockerImage(param)
	if err != nil {
		TaskService.Failed(taskID, "docker_remove_image", err.Error())
		return
	}
	TaskService.Done(taskID, "docker_remove_image", "Docker 镜像删除完成", result)
}

func startDockerContainerTask(taskID string, containerID string) {
	TaskService.MarkRunning(taskID, "docker_container_start", "开始启动 Docker 容器", map[string]any{"containerId": containerID})
	result, err := NewEdgeService().StartDockerContainer(containerID)
	if err != nil {
		TaskService.Failed(taskID, "docker_container_start", err.Error())
		return
	}
	TaskService.Done(taskID, "docker_container_start", "Docker 容器启动完成", result)
}

func stopDockerContainerTask(taskID string, containerID string, param *edgeModel.ContainerActionRequest) {
	TaskService.MarkRunning(taskID, "docker_container_stop", "开始停止 Docker 容器", map[string]any{"containerId": containerID})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go TaskService.RunHeartbeat(ctx, taskID, "docker_container_stop", "Docker 容器停止仍在执行")
	result, err := NewEdgeService().StopDockerContainer(containerID, param)
	if err != nil {
		TaskService.Failed(taskID, "docker_container_stop", err.Error())
		return
	}
	TaskService.Done(taskID, "docker_container_stop", "Docker 容器停止完成", result)
}

func restartDockerContainerTask(taskID string, containerID string, param *edgeModel.ContainerActionRequest) {
	TaskService.MarkRunning(taskID, "docker_container_restart", "开始重启 Docker 容器", map[string]any{"containerId": containerID})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go TaskService.RunHeartbeat(ctx, taskID, "docker_container_restart", "Docker 容器重启仍在执行")
	result, err := NewEdgeService().RestartDockerContainer(containerID, param)
	if err != nil {
		TaskService.Failed(taskID, "docker_container_restart", err.Error())
		return
	}
	TaskService.Done(taskID, "docker_container_restart", "Docker 容器重启完成", result)
}

func dockerPullEventMessage(event edgeModel.DockerPullEvent) string {
	if strings.TrimSpace(event.Error) != "" {
		return event.Error
	}
	status := strings.TrimSpace(event.Status)
	if status == "" {
		status = "Docker pull progress"
	}
	if strings.TrimSpace(event.ID) != "" {
		return event.ID + ": " + status
	}
	return status
}

func pullImageResourceID(param *edgeModel.PullImageRequest) string {
	if param == nil {
		return ""
	}
	image := strings.TrimSpace(param.Image)
	tag := strings.TrimSpace(param.Tag)
	if image == "" || tag == "" || strings.Contains(image, ":") {
		return image
	}
	return image + ":" + tag
}

// bindOptionalContainerActionRequest 绑定容器动作的可选 JSON 请求体。
//
// Gin 的 ShouldBindJSON 遇到空 body 会返回 EOF；stop/restart 允许不传 body，
// 因此这里把 EOF 归一化为空参数，其它 JSON 格式错误仍然按请求参数错误返回。
func bindOptionalContainerActionRequest(c *gin.Context) (*edgeModel.ContainerActionRequest, bool) {
	param := new(edgeModel.ContainerActionRequest)
	if err := c.ShouldBindJSON(param); err != nil {
		if errors.Is(err, io.EOF) {
			return param, true
		}
		HttpResponse.ResponseErrorWithMsg(c, HttpResponse.CodeInvalidParams, "请求参数格式错误")
		return nil, false
	}
	return param, true
}

func publicRequestBase(c *gin.Context) string {
	scheme := "http"
	if c.Request.TLS != nil || strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	host := c.Request.Host
	if host == "" {
		host = "127.0.0.1"
	}
	return scheme + "://" + host
}
