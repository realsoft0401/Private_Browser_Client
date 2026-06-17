package Slot

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	model "private_browser_client/Models/Slot"
	"private_browser_client/Pkg/HttpResponse"
	common "private_browser_client/Repository/Common"
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
