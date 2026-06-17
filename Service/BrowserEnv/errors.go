package BrowserEnv

type errorKind string

const (
	errorKindInvalid  errorKind = "invalid"
	errorKindConflict errorKind = "conflict"
	errorKindNotFound errorKind = "not_found"
	errorKindInternal errorKind = "internal"
)

// BusinessError 用于把正式 browser-env 接口的业务失败语义收口到 Service 层。
//
// 这层错误不是为了替代 Repository.Common 的通用错误，而是为了表达：
// - 参数非法
// - 生命周期冲突
// - 本机资产创建/写盘失败
// 这三类更贴近正式接口语义的错误。
type BusinessError struct {
	Kind    errorKind
	Message string
}

func (e *BusinessError) Error() string {
	return e.Message
}

func invalidError(message string) *BusinessError {
	return &BusinessError{Kind: errorKindInvalid, Message: message}
}

func conflictError(message string) *BusinessError {
	return &BusinessError{Kind: errorKindConflict, Message: message}
}

func notFoundError(message string) *BusinessError {
	return &BusinessError{Kind: errorKindNotFound, Message: message}
}

func internalError(message string) *BusinessError {
	return &BusinessError{Kind: errorKindInternal, Message: message}
}
