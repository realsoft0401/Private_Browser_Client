package NodeRegister

import (
	"context"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"

	Model "private_browser_client/Models/NodeRegister"
	"private_browser_client/Pkg/HttpResponse"
)

// GetStatus 返回当前 Client 的中心注册状态。
//
// 这个接口的价值是让你在 Client 本机直接看见：
// - Node 注册配置是否完整
// - 当前将以什么 baseUrl/clientIp/dockerApiUrl 去注册
// - Node 当前是否已经存在这台 Client 的中心登记结果
func GetStatus(ctx *gin.Context) {
	requestCtx, cancel := context.WithTimeout(ctx.Request.Context(), 10*time.Second)
	defer cancel()
	HttpResponse.ResponseSuccess(ctx, NewService().BuildStatusView(requestCtx))
}

// Assign 接收 Node Server 反向下发的 clientId，并写入本地 JSON 留痕。
//
// 设计来源：
// - 第一阶段正式主线已经改成“Node bind -> Node push -> Client assign”；
// - 因此 Client 需要一条受控写入口，把中心下发结果留到本机文件；
// - 这里显式校验 `X-Edge-API-Key`，避免接口退化成任何人都能改本地中心身份缓存。
func Assign(ctx *gin.Context) {
	var request Model.AssignRequest
	if err := ctx.ShouldBindJSON(&request); err != nil {
		HttpResponse.ResponseErrorWithMsg(ctx, HttpResponse.CodeInvalidParams, "assign clientId failed: request body 非法")
		return
	}
	requestCtx, cancel := context.WithTimeout(ctx.Request.Context(), 10*time.Second)
	defer cancel()
	result, err := NewService().AssignClientID(requestCtx, ctx.GetHeader("X-Edge-API-Key"), request)
	if err != nil {
		code := HttpResponse.CodeRemoteError
		if isAssignUnauthorized(err) {
			code = HttpResponse.CodeUnauthorized
		} else if isAssignInvalidParams(err) {
			code = HttpResponse.CodeInvalidParams
		}
		HttpResponse.ResponseErrorWithMsg(ctx, code, fmt.Sprintf("assign clientId failed: %s", err.Error()))
		return
	}
	HttpResponse.ResponseSuccess(ctx, result)
}
