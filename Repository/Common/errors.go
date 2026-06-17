package Common

import "errors"

var (
	// ErrNotFound 表示当前请求的数据不存在。
	ErrNotFound = errors.New("repository: record not found")
	// ErrDuplicate 表示写入时遇到唯一键冲突。
	ErrDuplicate = errors.New("repository: duplicate record")
	// ErrConflict 表示当前状态不允许执行本次写入。
	ErrConflict = errors.New("repository: state conflict")
)
