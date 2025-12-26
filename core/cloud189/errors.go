package cloud189

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/dnslin/cloud189-desktop/core/httpclient"
)

const (
	ErrCodeUnknown = iota
	ErrCodeInvalidToken
	ErrCodeUnauthorized
	ErrCodeForbidden
	ErrCodeFileNotFound
	ErrCodeInvalidRequest
	ErrCodeRateLimited
	ErrCodeServer
)

// CloudError 表示统一的业务错误。
type CloudError struct {
	Code       int
	Message    string
	HTTPStatus int
	Raw        error
}

func (e *CloudError) Error() string {
	if e == nil {
		return ""
	}
	switch {
	case e.Code != 0 && e.Message != "":
		return fmt.Sprintf("cloud189: [%d] %s", e.Code, e.Message)
	case e.Message != "":
		return e.Message
	case e.Code != 0:
		return fmt.Sprintf("cloud189: 错误码=%d", e.Code)
	case e.Raw != nil:
		return e.Raw.Error()
	default:
		return "cloud189: 未知错误"
	}
}

// Unwrap 允许 errors.Is/As 解构底层错误。
func (e *CloudError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Raw
}

// NewCloudError 创建基本 CloudError。
func NewCloudError(code int, message string) *CloudError {
	return &CloudError{Code: code, Message: message}
}

// WrapCloudError 在保留底层错误的同时生成 CloudError。
func WrapCloudError(code int, message string, raw error) *CloudError {
	status := httpStatusFromErr(raw)
	if message == "" && raw != nil {
		message = raw.Error()
	}
	return &CloudError{
		Code:       code,
		Message:    message,
		HTTPStatus: status,
		Raw:        raw,
	}
}

func httpStatusFromErr(err error) int {
	var ec *httpclient.ErrCode
	if errors.As(err, &ec) && ec.Status > 0 {
		return ec.Status
	}
	return 0
}

func mapErrCode(ec *httpclient.ErrCode) int {
	if ec == nil {
		return ErrCodeUnknown
	}
	upper := strings.ToUpper(ec.Code)
	switch {
	case strings.Contains(upper, "INVALIDSESSION"),
		strings.Contains(upper, "INVALIDTOKEN"):
		return ErrCodeInvalidToken
	case strings.Contains(upper, "UNAUTHORIZED"),
		strings.Contains(upper, "NOT_LOGIN"):
		return ErrCodeUnauthorized
	case strings.Contains(upper, "FORBIDDEN"),
		strings.Contains(upper, "NO_PERMISSION"),
		strings.Contains(upper, "PERMISSION"):
		return ErrCodeForbidden
	case strings.Contains(upper, "NOT_FOUND"),
		strings.Contains(upper, "NOTEXIST"),
		strings.Contains(upper, "NOT_EXIST"):
		return ErrCodeFileNotFound
	case strings.Contains(upper, "PARAM"),
		strings.Contains(upper, "BAD_REQUEST"):
		return ErrCodeInvalidRequest
	}

	switch ec.Status {
	case http.StatusUnauthorized:
		return ErrCodeUnauthorized
	case http.StatusForbidden:
		return ErrCodeForbidden
	case http.StatusNotFound:
		return ErrCodeFileNotFound
	case http.StatusTooManyRequests:
		return ErrCodeRateLimited
	}
	if ec.Status >= http.StatusInternalServerError && ec.Status < 600 {
		return ErrCodeServer
	}
	return ErrCodeUnknown
}

// toCloudError 将 httpclient.ErrCode 转换为 CloudError，未命中时返回原始错误。
func toCloudError(err error) error {
	if err == nil {
		return nil
	}
	var ce *CloudError
	if errors.As(err, &ce) {
		return ce
	}
	var ec *httpclient.ErrCode
	if errors.As(err, &ec) {
		msg := ec.Message
		if msg == "" {
			msg = ec.Code
		}
		if msg == "" && ec.Status > 0 {
			msg = http.StatusText(ec.Status)
		}
		return WrapCloudError(mapErrCode(ec), msg, err)
	}
	return err
}
