package HttpResponse

type ResCode int64

const (
	CodeSuccess       ResCode = 1000
	CodeInvalidParams ResCode = 1001
	CodeNotFound      ResCode = 1002
	CodeConflict      ResCode = 1003
	CodeRemoteError   ResCode = 1004
	CodeServerBusy    ResCode = 1005
	CodeUnauthorized  ResCode = 1006
)

var codeMsgMap = map[ResCode]string{
	CodeSuccess:       "success",
	CodeInvalidParams: "请求参数错误",
	CodeNotFound:      "数据不存在",
	CodeConflict:      "数据状态冲突",
	CodeRemoteError:   "本机依赖调用失败",
	CodeServerBusy:    "服务繁忙",
	CodeUnauthorized:  "未授权",
}

func (c ResCode) Msg() string {
	if msg, ok := codeMsgMap[c]; ok {
		return msg
	}
	return codeMsgMap[CodeServerBusy]
}
