package Package

import (
	"errors"

	"github.com/gin-gonic/gin"

	model "private_browser_client/Models/Package"
	"private_browser_client/Pkg/HttpResponse"
	common "private_browser_client/Repository/Common"
)

// GetRuntimeView 返回指定 package 的当前主状态视图。
//
// 注意：
// - 这条接口还属于旧的过渡期 package 入口；
// - 新的正式业务入口已经转到 browser-envs/*；
// - 这里暂时保留，仅用于当前本机调试和兼容过渡，不再继续扩展新能力。
func GetRuntimeView(c *gin.Context) {
	result, err := NewService().GetByPackageID(c.Param("packageId"))
	if err != nil {
		responseRepositoryError(c, err)
		return
	}
	HttpResponse.ResponseSuccess(c, result)
}

// RunPackage 是旧 package run 入口的 HTTP 入口。
//
// ******** 平台端接口接入说明：
// run 的平台放行、中心任务登记、成功回告都不要直接写在 HTTP handler。
// 统一继续收口在 Service/Package.RunPackage，避免协议层和业务层耦合。
func RunPackage(c *gin.Context) {
	var request model.RunPackageRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		HttpResponse.ResponseErrorWithMsg(c, HttpResponse.CodeInvalidParams, "请求体解析失败，请检查 slotId")
		return
	}

	result, err := NewService().RunPackage(c.Param("packageId"), request.SlotID)
	if err != nil {
		responseRepositoryError(c, err)
		return
	}
	HttpResponse.ResponseSuccess(c, result)
}

// StopPackage 是旧 package stop 入口的 HTTP 入口。
//
// ******** 平台端接口接入说明：
// stop 的平台确认、结束回告、slot 释放同步也不要写在 HTTP handler。
// 后续平台端接口到位后，直接按 Service/Package.StopPackage 里的注释接入。
func StopPackage(c *gin.Context) {
	var request model.StopPackageRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		HttpResponse.ResponseErrorWithMsg(c, HttpResponse.CodeInvalidParams, "请求体解析失败，请检查 slotId")
		return
	}

	result, err := NewService().StopPackage(c.Param("packageId"), request.SlotID)
	if err != nil {
		responseRepositoryError(c, err)
		return
	}
	HttpResponse.ResponseSuccess(c, result)
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
