package Edge

import (
	"errors"
	"io"

	"github.com/gin-gonic/gin"

	edgeModel "private_browser_client/Models/Edge"
	"private_browser_client/Pkg/HttpResponse"
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
func PullDockerImage(c *gin.Context) {
	param := new(edgeModel.PullImageRequest)
	if err := c.ShouldBindJSON(param); err != nil {
		HttpResponse.ResponseErrorWithMsg(c, HttpResponse.CodeInvalidParams, "请求参数格式错误")
		return
	}
	result, err := NewEdgeService().PullDockerImage(param)
	if err != nil {
		HttpResponse.ResponseErrorWithMsg(c, HttpResponse.CodeRemoteError, err.Error())
		return
	}
	HttpResponse.ResponseSuccess(c, result)
}

// RemoveDockerImage 删除本机 Docker 镜像。
//
// 这是本机管理动作，不涉及数据库和中心节点归属。
// 删除策略由请求体的 force / noPrune 显式控制，避免 handler 里出现隐式强删行为。
func RemoveDockerImage(c *gin.Context) {
	param := new(edgeModel.RemoveImageRequest)
	if err := c.ShouldBindJSON(param); err != nil {
		HttpResponse.ResponseErrorWithMsg(c, HttpResponse.CodeInvalidParams, "请求参数格式错误")
		return
	}
	result, err := NewEdgeService().RemoveDockerImage(param)
	if err != nil {
		HttpResponse.ResponseErrorWithMsg(c, HttpResponse.CodeRemoteError, err.Error())
		return
	}
	HttpResponse.ResponseSuccess(c, result)
}

// StartDockerContainer 启动本机 Docker 容器。
//
// 容器 ID 来自路径参数，handler 不做业务归属判断；边缘服务当前只面对本机 Docker。
// 后续中心服务端如需限制权限，应在调用边缘服务前完成鉴权和设备归属校验。
func StartDockerContainer(c *gin.Context) {
	result, err := NewEdgeService().StartDockerContainer(c.Param("id"))
	if err != nil {
		HttpResponse.ResponseErrorWithMsg(c, HttpResponse.CodeRemoteError, err.Error())
		return
	}
	HttpResponse.ResponseSuccess(c, result)
}

// StopDockerContainer 停止本机 Docker 容器。
//
// 请求体可为空；为空时 Service 使用默认等待时间。
// 这里保留可选 JSON，是为了前端或中心服务端在需要快速停止时能显式传 timeoutSeconds。
func StopDockerContainer(c *gin.Context) {
	param, ok := bindOptionalContainerActionRequest(c)
	if !ok {
		return
	}
	result, err := NewEdgeService().StopDockerContainer(c.Param("id"), param)
	if err != nil {
		HttpResponse.ResponseErrorWithMsg(c, HttpResponse.CodeRemoteError, err.Error())
		return
	}
	HttpResponse.ResponseSuccess(c, result)
}

// RestartDockerContainer 重启本机 Docker 容器。
//
// 这个 HTTP 入口只做参数绑定和响应封装，真正的 Docker API 状态码处理在 Service 中完成。
func RestartDockerContainer(c *gin.Context) {
	param, ok := bindOptionalContainerActionRequest(c)
	if !ok {
		return
	}
	result, err := NewEdgeService().RestartDockerContainer(c.Param("id"), param)
	if err != nil {
		HttpResponse.ResponseErrorWithMsg(c, HttpResponse.CodeRemoteError, err.Error())
		return
	}
	HttpResponse.ResponseSuccess(c, result)
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
