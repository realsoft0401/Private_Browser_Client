package BrowserEnv

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	model "private_browser_client/Models/BrowserEnv"
	"private_browser_client/Pkg/HttpResponse"
	common "private_browser_client/Repository/Common"
)

// ListBrowserEnvs 是正式 browser-env 列表接口的 HTTP 入口。
//
// HTTP 层只负责读取 query 参数和地址基准；真正的筛选、分页、默认排除 deleted 和统计口径
// 都统一留在 Service/Repository，避免接口层变成第二套查询业务。
func ListBrowserEnvs(c *gin.Context) {
	page, err := parseOptionalIntQuery(c, "page")
	if err != nil {
		HttpResponse.ResponseErrorWithMsg(c, HttpResponse.CodeInvalidParams, "page 必须是整数")
		return
	}
	pageSize, err := parseOptionalIntQuery(c, "pageSize")
	if err != nil {
		HttpResponse.ResponseErrorWithMsg(c, HttpResponse.CodeInvalidParams, "pageSize 必须是整数")
		return
	}

	result, err := NewService().List(model.ListBrowserEnvQuery{
		UserID:   c.Query("userId"),
		RPAType:  c.Query("rpaType"),
		Status:   c.Query("status"),
		Page:     page,
		PageSize: pageSize,
	}, publicRequestBase(c), publicWebSocketBase(c))
	if err != nil {
		responseBrowserEnvError(c, err)
		return
	}
	HttpResponse.ResponseSuccess(c, result)
}

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

// GetBrowserEnvDetail 是正式 browser-env 详情接口的 HTTP 入口。
func GetBrowserEnvDetail(c *gin.Context) {
	result, err := NewService().GetDetail(c.Param("envId"), publicRequestBase(c), publicWebSocketBase(c))
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

func parseOptionalIntQuery(c *gin.Context, key string) (int, error) {
	value := strings.TrimSpace(c.Query(key))
	if value == "" {
		return 0, nil
	}
	return strconv.Atoi(value)
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
