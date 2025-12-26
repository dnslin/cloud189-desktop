package cloud189

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// FlexString 兼容字符串和数字的 JSON 字段。
type FlexString string

func (f *FlexString) UnmarshalJSON(data []byte) error {
	// 尝试解析为字符串
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		*f = FlexString(s)
		return nil
	}
	// 尝试解析为数字
	var n json.Number
	if err := json.Unmarshal(data, &n); err == nil {
		*f = FlexString(n.String())
		return nil
	}
	return nil
}

// String 返回字符串值。
func (f FlexString) String() string {
	return string(f)
}

// CodeResponse 兼容 code/res_code 返回结构，实现业务错误检测。
type CodeResponse struct {
	CodeValue  string     `json:"code,omitempty"`
	Msg        string     `json:"msg,omitempty"`
	ResCode    FlexString `json:"res_code,omitempty"`
	ResMessage string     `json:"res_message,omitempty"`
}

// IsSuccess 判断业务码是否为成功。
func (r *CodeResponse) IsSuccess() bool {
	if r == nil {
		return true
	}
	code := r.CodeValue
	if code == "" {
		code = string(r.ResCode)
	}
	if code == "" {
		return true
	}
	upper := strings.ToUpper(code)
	return upper == "SUCCESS" || upper == "0"
}

// Error 满足 error 接口，便于 httpclient 包装。
func (r *CodeResponse) Error() string {
	return fmt.Sprintf("%s: %s", r.Code(), r.Message())
}

// Code 返回统一错误码。
func (r *CodeResponse) Code() string {
	if r == nil {
		return ""
	}
	if r.CodeValue != "" {
		return r.CodeValue
	}
	return string(r.ResCode)
}

// Message 返回服务端消息。
func (r *CodeResponse) Message() string {
	if r == nil {
		return ""
	}
	if r.Msg != "" {
		return r.Msg
	}
	return r.ResMessage
}

func buildRequest(ctx context.Context, method, base, path string, params map[string]string) (*http.Request, error) {
	u := joinURL(base, path)
	vals := url.Values{}
	for k, v := range params {
		vals.Set(k, v)
	}

	var body io.Reader
	switch strings.ToUpper(method) {
	case http.MethodGet:
		if len(vals) > 0 {
			if strings.Contains(u, "?") {
				u += "&" + vals.Encode()
			} else {
				u += "?" + vals.Encode()
			}
		}
	case http.MethodPost:
		body = strings.NewReader(vals.Encode())
	default:
		return nil, fmt.Errorf("cloud189: 不支持的方法 %s", method)
	}

	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json;charset=UTF-8")
	if body != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader(vals.Encode())), nil
		}
	}
	return req, nil
}

func joinURL(base, path string) string {
	if base == "" {
		return path
	}
	base = strings.TrimSuffix(base, "/")
	if path == "" {
		return base
	}
	if strings.HasPrefix(path, "/") {
		return base + path
	}
	return base + "/" + path
}
