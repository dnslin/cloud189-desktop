package errors

import "fmt"

// Code 代表核心业务的错误类型。
type Code string

const (
	// ErrCodeUnknown 表示未知或未分类错误。
	ErrCodeUnknown Code = "UNKNOWN"
	// ErrCodeNotFound 表示目标不存在。
	ErrCodeNotFound Code = "NOT_FOUND"
	// ErrCodeInvalidArgument 表示输入参数非法或缺失。
	ErrCodeInvalidArgument Code = "INVALID_ARGUMENT"
	// ErrCodeInvalidConfig 表示依赖未配置或状态异常。
	ErrCodeInvalidConfig Code = "INVALID_CONFIG"
	// ErrCodeInvalidState 表示流程或返回数据不符合预期。
	ErrCodeInvalidState Code = "INVALID_STATE"
)

// CoreError 提供核心层统一的结构化错误。
type CoreError struct {
	Code    Code
	Message string
	Raw     error
}

func (e *CoreError) Error() string {
	if e == nil {
		return ""
	}
	switch {
	case e.Code != "" && e.Message != "":
		return fmt.Sprintf("core: [%s] %s", e.Code, e.Message)
	case e.Message != "":
		return e.Message
	case e.Code != "":
		return fmt.Sprintf("core: 错误码=%s", e.Code)
	case e.Raw != nil:
		return e.Raw.Error()
	default:
		return "core: 未知错误"
	}
}

// Unwrap 允许 errors.Is/As 解构底层错误。
func (e *CoreError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Raw
}

// Is 支持按错误码或同一实例匹配，兼容 sentinel 用法。
func (e *CoreError) Is(target error) bool {
	if e == nil || target == nil {
		return false
	}
	if e == target {
		return true
	}
	t, ok := target.(*CoreError)
	if !ok {
		return false
	}
	return e.Code != "" && e.Code == t.Code
}

// New 创建基本的 CoreError。
func New(code Code, message string) *CoreError {
	return &CoreError{Code: code, Message: message}
}

// Wrap 在保留底层错误的同时生成 CoreError。
func Wrap(code Code, message string, raw error) *CoreError {
	if message == "" && raw != nil {
		message = raw.Error()
	}
	return &CoreError{
		Code:    code,
		Message: message,
		Raw:     raw,
	}
}
