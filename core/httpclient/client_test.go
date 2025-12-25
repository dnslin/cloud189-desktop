package httpclient

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

type mockResponse struct {
	ResCode    int    `json:"res_code"`
	ResMessage string `json:"res_message"`
}

func (m *mockResponse) IsSuccess() bool {
	return m.ResCode == 0
}

func (m *mockResponse) Error() string {
	return fmt.Sprintf("%d: %s", m.ResCode, m.ResMessage)
}

func (m *mockResponse) Code() string {
	return strconv.Itoa(m.ResCode)
}

func (m *mockResponse) Message() string {
	return m.ResMessage
}

func TestDoSuccess(t *testing.T) {
	client := NewClient(WithHTTPClient(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return jsonResponse(http.StatusOK, `{"res_code":0,"res_message":"ok"}`), nil
		}),
	}))
	req, _ := http.NewRequest(http.MethodGet, "http://mock/success", nil)
	var rsp mockResponse
	if err := client.Do(req, &rsp); err != nil {
		t.Fatalf("预期成功，得到错误: %v", err)
	}
	if rsp.ResCode != 0 {
		t.Fatalf("业务码解析错误: %+v", rsp)
	}
}

func TestBusinessErrorNoRetry(t *testing.T) {
	client := NewClient(WithHTTPClient(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return jsonResponse(http.StatusOK, `{"res_code":1,"res_message":"failed"}`), nil
		}),
	}))
	req, _ := http.NewRequest(http.MethodGet, "http://mock/business", nil)
	var rsp mockResponse
	err := client.Do(req, &rsp)
	if err == nil {
		t.Fatal("预期业务错误，但返回 nil")
	}
	var ec *ErrCode
	if !errors.As(err, &ec) {
		t.Fatalf("错误类型应为 ErrCode，实际: %v", err)
	}
	if ec.Code != "1" {
		t.Fatalf("错误码不匹配，得到 %s", ec.Code)
	}
}

func TestAuthRefresh(t *testing.T) {
	attempt := 0
	refreshCalled := 0
	policy := NewExponentialBackoffRetry(RetryConfig{
		MaxRetries: 2,
		BaseDelay:  1 * time.Millisecond,
		MaxDelay:   5 * time.Millisecond,
		Refresh: func() error {
			refreshCalled++
			return nil
		},
		AuthCodes: []string{"401"},
		Logger:    NopLogger{},
	})

	client := NewClient(
		WithRetryPolicy(policy),
		WithHTTPClient(&http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				attempt++
				if attempt == 1 {
					return jsonResponse(http.StatusOK, `{"res_code":401,"res_message":"expired"}`), nil
				}
				return jsonResponse(http.StatusOK, `{"res_code":0,"res_message":"ok"}`), nil
			}),
		}),
	)
	req, _ := http.NewRequest(http.MethodGet, "http://mock/auth", nil)
	var rsp mockResponse
	if err := client.Do(req, &rsp); err != nil {
		t.Fatalf("刷新后应重试成功: %v", err)
	}
	if refreshCalled != 1 {
		t.Fatalf("刷新调用次数不正确，得到 %d", refreshCalled)
	}
	if attempt != 2 {
		t.Fatalf("请求次数不正确，得到 %d", attempt)
	}
}

func TestNetworkRetry(t *testing.T) {
	transport := &flakyTransport{
		failures: 1,
		inner: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return jsonResponse(http.StatusOK, `{"res_code":0,"res_message":"ok"}`), nil
		}),
	}
	policy := NewExponentialBackoffRetry(RetryConfig{
		MaxRetries: 1,
		BaseDelay:  1 * time.Millisecond,
		MaxDelay:   5 * time.Millisecond,
		Logger:     NopLogger{},
	})
	client := NewClient(
		WithHTTPClient(&http.Client{Transport: transport}),
		WithRetryPolicy(policy),
	)
	req, _ := http.NewRequest(http.MethodGet, "http://mock/network", nil)
	var rsp mockResponse
	if err := client.Do(req, &rsp); err != nil {
		t.Fatalf("网络错误后应重试成功: %v", err)
	}
	if transport.attempts != 2 {
		t.Fatalf("应尝试 2 次，实际 %d", transport.attempts)
	}
}

func TestRateLimiter(t *testing.T) {
	limiter := NewTokenBucketLimiter(5, 1, nil)
	client := NewClient(
		WithRateLimiter(limiter),
		WithHTTPClient(&http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return jsonResponse(http.StatusOK, `{"res_code":0,"res_message":"ok"}`), nil
		})}),
	)
	start := time.Now()
	for i := 0; i < 2; i++ {
		req, _ := http.NewRequest(http.MethodGet, "http://mock/ratelimit", nil)
		var rsp mockResponse
		if err := client.Do(req, &rsp); err != nil {
			t.Fatalf("限流请求失败: %v", err)
		}
	}
	elapsed := time.Since(start)
	if elapsed < 150*time.Millisecond {
		t.Fatalf("限流未生效，耗时过短: %v", elapsed)
	}
}

func TestDecodeError(t *testing.T) {
	client := NewClient(WithHTTPClient(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return jsonResponse(http.StatusOK, `invalid json`), nil
		}),
	}))
	req, _ := http.NewRequest(http.MethodGet, "http://mock/decode", nil)
	var rsp mockResponse
	err := client.Do(req, &rsp)
	if err == nil {
		t.Fatal("预期解码失败错误")
	}
	var de *DecodeError
	if !errors.As(err, &de) {
		t.Fatalf("错误类型应为 DecodeError，实际: %v", err)
	}
}

func TestBodyWithoutGetBodyCannotRetry(t *testing.T) {
	policy := NewExponentialBackoffRetry(RetryConfig{
		MaxRetries: 1,
		BaseDelay:  1 * time.Millisecond,
		MaxDelay:   1 * time.Millisecond,
		Logger:     NopLogger{},
	})
	client := NewClient(
		WithRetryPolicy(policy),
		WithHTTPClient(&http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return jsonResponse(http.StatusInternalServerError, ``), nil
		})}),
	)

	req, _ := http.NewRequest(http.MethodPost, "http://mock/body", bytes.NewBufferString("data"))
	req.GetBody = nil // 模拟无法重试的场景
	err := client.Do(req, &mockResponse{})
	if err == nil {
		t.Fatal("预期因无法重试请求体而失败")
	}
	if err.Error() != "httpclient: 请求体不可重试" {
		t.Fatalf("错误信息不符合预期: %v", err)
	}
}

type flakyTransport struct {
	failures int
	inner    http.RoundTripper
	attempts int
}

func (f *flakyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	f.attempts++
	if f.failures > 0 {
		f.failures--
		return nil, errors.New("模拟网络失败")
	}
	return f.inner.RoundTrip(req)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func jsonResponse(status int, body string) *http.Response {
	rec := httptest.NewRecorder()
	rec.Header().Set("Content-Type", "application/json")
	rec.WriteHeader(status)
	rec.Body.WriteString(body)
	return rec.Result()
}
