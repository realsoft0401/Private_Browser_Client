package BrowserEnv

const (
	errorKindInvalid  = "invalid"
	errorKindNotFound = "not_found"
	errorKindConflict = "conflict"
	errorKindRemote   = "remote"
	errorKindInternal = "internal"
)

// BusinessError 是 BrowserEnv Service 返回给 HTTP 层的稳定业务错误。
//
// 设计来源：
// - BrowserEnv Service 只负责业务和文件落盘，不应该直接依赖 Gin 或 HTTP 状态；
// - handler 根据 Kind 统一映射到项目响应码，避免 Service 层混入接口响应细节。
type BusinessError struct {
	Kind    string
	Message string
}

func (e *BusinessError) Error() string {
	return e.Message
}

func invalidError(message string) *BusinessError {
	return &BusinessError{Kind: errorKindInvalid, Message: message}
}

func notFoundError(message string) *BusinessError {
	return &BusinessError{Kind: errorKindNotFound, Message: message}
}

func conflictError(message string) *BusinessError {
	return &BusinessError{Kind: errorKindConflict, Message: message}
}

func remoteError(message string) *BusinessError {
	return &BusinessError{Kind: errorKindRemote, Message: message}
}

func internalError(message string) *BusinessError {
	return &BusinessError{Kind: errorKindInternal, Message: message}
}

// IsBusinessError 判断错误是否为 BrowserEnv Service 的业务错误。
//
// HTTP 层只认这个稳定判断入口，不需要知道具体函数是在哪个文件里产生错误。
func IsBusinessError(err error) (*BusinessError, bool) {
	bizErr, ok := err.(*BusinessError)
	return bizErr, ok
}
