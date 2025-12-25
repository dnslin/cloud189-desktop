package cloud189

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/gowsp/cloud189-desktop/core/httpclient"
)

// CodeResponse 兼容 code/res_code 返回结构，实现业务错误检测。
type CodeResponse struct {
	CodeValue  string `json:"code,omitempty"`
	Msg        string `json:"msg,omitempty"`
	ResCode    string `json:"res_code,omitempty"`
	ResMessage string `json:"res_message,omitempty"`
}

// IsSuccess 判断业务码是否为成功。
func (r *CodeResponse) IsSuccess() bool {
	if r == nil {
		return true
	}
	code := r.CodeValue
	if code == "" {
		code = r.ResCode
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
	return r.ResCode
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

// AppGet 以 App 签名发送 GET。
func (c *Client) AppGet(ctx context.Context, path string, params map[string]string, out any) error {
	return c.do(ctx, http.MethodGet, c.appBaseURL, path, params, out, c.appSigner.Middleware())
}

// AppPost 以 App 签名发送 POST。
func (c *Client) AppPost(ctx context.Context, path string, params map[string]string, out any) error {
	return c.do(ctx, http.MethodPost, c.appBaseURL, path, params, out, c.appSigner.Middleware())
}

// WebGet 带 Cookie 的 Web GET。
func (c *Client) WebGet(ctx context.Context, path string, params map[string]string, out any) error {
	mw := WithWebCookies(c.session)
	return c.do(ctx, http.MethodGet, c.webBaseURL, path, params, out, mw)
}

// WebPost 带 Cookie 的 Web POST。
func (c *Client) WebPost(ctx context.Context, path string, params map[string]string, out any) error {
	mw := WithWebCookies(c.session)
	return c.do(ctx, http.MethodPost, c.webBaseURL, path, params, out, mw)
}

func (c *Client) do(ctx context.Context, method, base, path string, params map[string]string, out any, middlewares ...httpclient.Middleware) error {
	if c == nil {
		return errors.New("cloud189: Client 未初始化")
	}
	req, err := buildRequest(ctx, method, base, path, params)
	if err != nil {
		return err
	}
	return c.useMiddlewares(req, out, middlewares...)
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
