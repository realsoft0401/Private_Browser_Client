package BrowserEnv

import (
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	model "private_browser_client/Models/BrowserEnv"
	"private_browser_client/Pkg/HttpResponse"
	common "private_browser_client/Repository/Common"
)

// CreateBrowserEnv 是正式 browser-env 创建接口的 HTTP 入口。
func CreateBrowserEnv(c *gin.Context) {
	var request model.CreateBrowserEnvRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		HttpResponse.ResponseErrorWithMsg(c, HttpResponse.CodeInvalidParams, "请求体解析失败，请检查 create-browser-env 参数")
		return
	}

	result, err := NewService().Create(&request)
	if err != nil {
		responseBrowserEnvError(c, err)
		return
	}
	HttpResponse.ResponseSuccess(c, result)
}

// Run 是正式 browser-env run 接口的 HTTP 入口。
func Run(c *gin.Context) {
	var request model.RunRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		HttpResponse.ResponseErrorWithMsg(c, HttpResponse.CodeInvalidParams, "请求体解析失败，请检查 slotId")
		return
	}

	result, err := NewService().Run(c.Param("envId"), request)
	if err != nil {
		responseBrowserEnvError(c, err)
		return
	}
	HttpResponse.ResponseSuccess(c, result)
}

// Stop 是正式 browser-env stop 接口的 HTTP 入口。
func Stop(c *gin.Context) {
	var request model.StopRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		if !errors.Is(err, io.EOF) && !errors.Is(err, http.ErrBodyNotAllowed) {
			HttpResponse.ResponseErrorWithMsg(c, HttpResponse.CodeInvalidParams, "请求体解析失败，请检查 timeoutSeconds")
			return
		}
	}

	result, err := NewService().Stop(c.Param("envId"), request)
	if err != nil {
		responseBrowserEnvError(c, err)
		return
	}
	HttpResponse.ResponseSuccess(c, result)
}

// UpdateProxy 是正式 browser-env 代理修改接口的 HTTP 入口。
func UpdateProxy(c *gin.Context) {
	var request model.UpdateBrowserEnvProxyRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		HttpResponse.ResponseErrorWithMsg(c, HttpResponse.CodeInvalidParams, "请求体解析失败，请检查 proxy 修改参数")
		return
	}

	result, err := NewService().UpdateProxy(c.Param("envId"), &request)
	if err != nil {
		responseBrowserEnvError(c, err)
		return
	}
	HttpResponse.ResponseSuccess(c, result)
}

// Backup 是正式 browser-env backup 接口的 HTTP 入口。
func Backup(c *gin.Context) {
	result, err := NewService().Backup(c.Param("envId"))
	if err != nil {
		responseBrowserEnvError(c, err)
		return
	}
	HttpResponse.ResponseSuccess(c, result)
}

// Restore 是正式 browser-env restore 接口的 HTTP 入口。
func Restore(c *gin.Context) {
	result, err := NewService().Restore(c.Param("envId"))
	if err != nil {
		responseBrowserEnvError(c, err)
		return
	}
	HttpResponse.ResponseSuccess(c, result)
}

// Revalidate 是正式 browser-env revalidate 接口的 HTTP 入口。
func Revalidate(c *gin.Context) {
	result, err := NewService().Revalidate(c.Param("envId"))
	if err != nil {
		responseBrowserEnvError(c, err)
		return
	}
	HttpResponse.ResponseSuccess(c, result)
}

// ImportPackage 是正式 browser-env import-package 接口的 HTTP 入口。
func ImportPackage(c *gin.Context) {
	fileHeader, err := c.FormFile("file")
	if err != nil {
		HttpResponse.ResponseErrorWithMsg(c, HttpResponse.CodeInvalidParams, "缺少导入文件字段 file")
		return
	}
	file, err := fileHeader.Open()
	if err != nil {
		HttpResponse.ResponseErrorWithMsg(c, HttpResponse.CodeServerBusy, "打开导入文件失败，请检查上传内容")
		return
	}
	defer file.Close()

	result, err := NewService().ImportPackage(file, fileHeader.Filename)
	if err != nil {
		responseBrowserEnvError(c, err)
		return
	}
	HttpResponse.ResponseSuccess(c, result)
}

// DeletePackage 是正式 browser-env package delete 接口的 HTTP 入口。
func DeletePackage(c *gin.Context) {
	result, err := NewService().DeletePackage(c.Param("envId"))
	if err != nil {
		responseBrowserEnvError(c, err)
		return
	}
	HttpResponse.ResponseSuccess(c, result)
}

func responseBrowserEnvError(c *gin.Context, err error) {
	var businessErr *BusinessError
	switch {
	case errors.As(err, &businessErr):
		switch businessErr.Kind {
		case errorKindInvalid:
			HttpResponse.ResponseErrorWithMsg(c, HttpResponse.CodeInvalidParams, businessErr.Message)
		case errorKindConflict:
			HttpResponse.ResponseErrorWithMsg(c, HttpResponse.CodeConflict, businessErr.Message)
		case errorKindNotFound:
			HttpResponse.ResponseErrorWithMsg(c, HttpResponse.CodeNotFound, businessErr.Message)
		default:
			HttpResponse.ResponseErrorWithMsg(c, HttpResponse.CodeServerBusy, businessErr.Message)
		}
	case errors.Is(err, common.ErrNotFound):
		HttpResponse.ResponseError(c, HttpResponse.CodeNotFound)
	case errors.Is(err, common.ErrConflict):
		HttpResponse.ResponseErrorWithMsg(c, HttpResponse.CodeConflict, "browser env lifecycle conflict")
	default:
		HttpResponse.ResponseErrorWithMsg(c, HttpResponse.CodeServerBusy, err.Error())
	}
}
