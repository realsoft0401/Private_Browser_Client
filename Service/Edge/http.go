package Edge

import (
	"github.com/gin-gonic/gin"

	"private_browser_client/Pkg/HttpResponse"
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
