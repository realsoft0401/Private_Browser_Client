package HttpResponse

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type ResponseData struct {
	Code ResCode `json:"code"`
	Msg  string  `json:"message"`
	Data any     `json:"data,omitempty"`
}

func ResponseSuccess(ctx *gin.Context, data any) {
	ctx.JSON(http.StatusOK, &ResponseData{
		Code: CodeSuccess,
		Msg:  CodeSuccess.Msg(),
		Data: data,
	})
}

func ResponseError(ctx *gin.Context, code ResCode) {
	ctx.JSON(http.StatusOK, &ResponseData{
		Code: code,
		Msg:  code.Msg(),
	})
}

func ResponseErrorWithMsg(ctx *gin.Context, code ResCode, msg string) {
	ctx.JSON(http.StatusOK, &ResponseData{
		Code: code,
		Msg:  msg,
	})
}
