package HttpResponse

type ResCode int64

const (
	// CodeSuccess 表示接口处理成功。
	CodeSuccess ResCode = 1000
	// CodeInvalidParams 表示请求体、路径参数或业务参数校验失败。
	CodeInvalidParams ResCode = 1001
	// CodeNotFound 表示请求的数据不存在。
	CodeNotFound ResCode = 1002
	// CodeConflict 表示数据状态冲突。
	CodeConflict ResCode = 1003
	// CodeRemoteError 表示调用本机 Docker API 或后续外部依赖失败。
	CodeRemoteError ResCode = 1004
	// CodeServerBusy 表示服务端内部错误或暂时无法处理。
	CodeServerBusy ResCode = 1005
)

// codeMsgMap 是统一响应码到默认中文文案的映射。
//
// 业务层如果需要更具体的文案，可以使用 ResponseErrorWithMsg 覆盖；
// 但新增响应码时必须同步补这里，避免 Msg 回退到服务繁忙造成排障困难。
var codeMsgMap = map[ResCode]string{
	CodeSuccess:       "success",
	CodeInvalidParams: "请求参数错误",
	CodeNotFound:      "数据不存在",
	CodeConflict:      "数据状态冲突",
	CodeRemoteError:   "Docker API 调用失败",
	CodeServerBusy:    "服务繁忙",
}

// Msg 返回响应码对应的默认消息。
//
// 未登记的响应码统一回退到 CodeServerBusy，是为了避免接口返回空 message。
// 后续新增响应码时仍应先补 codeMsgMap，不要依赖这个兜底分支。
func (c ResCode) Msg() string {
	if msg, ok := codeMsgMap[c]; ok {
		return msg
	}
	return codeMsgMap[CodeServerBusy]
}
