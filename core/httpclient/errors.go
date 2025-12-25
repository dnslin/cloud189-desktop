package httpclient

import (
	"fmt"
	"net/http"
)

// ErrCode 表示业务错误码，兼容接口返回的 code 与 message 字段。
type ErrCode struct {
	Code    string
	Message string
	Status  int
}

func (e *ErrCode) Error() string {
	if e == nil {
		return ""
	}
	switch {
	case e.Code != "" && e.Message != "":
		return fmt.Sprintf("%s: %s", e.Code, e.Message)
	case e.Code != "":
		return e.Code
	case e.Message != "":
		return e.Message
	default:
		return fmt.Sprintf("http 状态码: %d", e.Status)
	}
}

// NetworkError 包装底层网络错误，便于区分可重试场景。
type NetworkError struct {
	Err error
}

func (e *NetworkError) Error() string {
	return fmt.Sprintf("网络错误: %v", e.Err)
}

func (e *NetworkError) Unwrap() error {
	return e.Err
}

// DecodeError 表示响应解码失败。
type DecodeError struct {
	Status int
	Err    error
}

func (e *DecodeError) Error() string {
	return fmt.Sprintf("解码失败(status=%d): %v", e.Status, e.Err)
}

func (e *DecodeError) Unwrap() error {
	return e.Err
}

// OkRsp 用于判断业务层是否成功。
type OkRsp interface {
	error
	IsSuccess() bool
}

type coder interface {
	Code() string
}

type messager interface {
	Message() string
}

func toErrCode(err error, status int) *ErrCode {
	if err == nil {
		return nil
	}
	if ec, ok := err.(*ErrCode); ok {
		if ec.Status == 0 {
			ec.Status = status
		}
		return ec
	}
	code := ""
	msg := err.Error()
	if c, ok := err.(coder); ok {
		code = c.Code()
	}
	if m, ok := err.(messager); ok {
		msg = m.Message()
	}
	return &ErrCode{Code: code, Message: msg, Status: status}
}

func statusToErr(status int) *ErrCode {
	return &ErrCode{
		Status:  status,
		Code:    fmt.Sprintf("HTTP_%d", status),
		Message: http.StatusText(status),
	}
}
