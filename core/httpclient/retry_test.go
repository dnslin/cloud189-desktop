package httpclient

import (
	"errors"
	"net/http"
	"testing"
	"time"
)

// TestExponentialBackoffRetry_BackoffClamp 验证指数退避的初始值与上限。
func TestExponentialBackoffRetry_BackoffClamp(t *testing.T) {
	cfg := RetryConfig{
		MaxRetries: 3,
		BaseDelay:  100 * time.Millisecond,
		MaxDelay:   150 * time.Millisecond,
	}
	retry := NewExponentialBackoffRetry(cfg)

	if delay := retry.backoff(0); delay != 100*time.Millisecond {
		t.Fatalf("首次退避应为 100ms，实际 %v", delay)
	}
	if delay := retry.backoff(2); delay != 150*time.Millisecond {
		t.Fatalf("退避应被上限 150ms 截断，实际 %v", delay)
	}
}

// TestExponentialBackoffRetry_ShouldRetry 覆盖不同错误场景的重试判定。
func TestExponentialBackoffRetry_ShouldRetry(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "https://example.com", nil)

	t.Run("server_error", func(t *testing.T) {
		cfg := DefaultRetryConfig()
		cfg.MaxRetries = 3
		cfg.BaseDelay = 50 * time.Millisecond
		cfg.MaxDelay = 200 * time.Millisecond
		retry := NewExponentialBackoffRetry(cfg)

		should, delay, err := retry.ShouldRetry(req, &http.Response{StatusCode: http.StatusInternalServerError}, nil, 0)
		if err != nil {
			t.Fatalf("不期望错误: %v", err)
		}
		if !should {
			t.Fatalf("500 场景应触发重试")
		}
		if delay != cfg.BaseDelay {
			t.Fatalf("退避时间应为 %v，实际 %v", cfg.BaseDelay, delay)
		}
	})

	t.Run("network_error", func(t *testing.T) {
		cfg := DefaultRetryConfig()
		cfg.MaxRetries = 2
		retry := NewExponentialBackoffRetry(cfg)

		should, _, err := retry.ShouldRetry(req, nil, &NetworkError{Err: errors.New("dial failed")}, 1)
		if err != nil {
			t.Fatalf("不期望错误: %v", err)
		}
		if !should {
			t.Fatalf("网络错误应触发重试")
		}
	})

	t.Run("decode_error", func(t *testing.T) {
		cfg := DefaultRetryConfig()
		cfg.MaxRetries = 2
		retry := NewExponentialBackoffRetry(cfg)

		should, _, err := retry.ShouldRetry(req, nil, &DecodeError{Status: http.StatusOK, Err: errors.New("bad json")}, 0)
		if err != nil {
			t.Fatalf("不期望错误: %v", err)
		}
		if should {
			t.Fatalf("解码错误不应重试")
		}
	})

	t.Run("auth_error_triggers_refresh", func(t *testing.T) {
		cfg := DefaultRetryConfig()
		cfg.MaxRetries = 2
		cfg.BaseDelay = 80 * time.Millisecond
		refreshCalled := 0
		cfg.Refresh = func() error {
			refreshCalled++
			return nil
		}
		retry := NewExponentialBackoffRetry(cfg)

		should, delay, err := retry.ShouldRetry(req, nil, &ErrCode{Code: "InvalidSignature", Status: http.StatusUnauthorized}, 0)
		if err != nil {
			t.Fatalf("不期望错误: %v", err)
		}
		if !should {
			t.Fatalf("认证错误应触发刷新后重试")
		}
		if delay != cfg.BaseDelay {
			t.Fatalf("退避时间应为 %v，实际 %v", cfg.BaseDelay, delay)
		}
		if refreshCalled != 1 {
			t.Fatalf("刷新回调应被调用一次，实际 %d 次", refreshCalled)
		}
	})

	t.Run("non_retriable_errcode", func(t *testing.T) {
		cfg := DefaultRetryConfig()
		cfg.MaxRetries = 2
		retry := NewExponentialBackoffRetry(cfg)

		should, _, err := retry.ShouldRetry(req, nil, &ErrCode{Code: "BadRequest", Status: http.StatusBadRequest}, 0)
		if err != nil {
			t.Fatalf("不期望错误: %v", err)
		}
		if should {
			t.Fatalf("业务错误不应重试")
		}
	})

	t.Run("max_attempts_reached", func(t *testing.T) {
		cfg := DefaultRetryConfig()
		cfg.MaxRetries = 1
		retry := NewExponentialBackoffRetry(cfg)

		should, _, err := retry.ShouldRetry(req, &http.Response{StatusCode: http.StatusInternalServerError}, nil, cfg.MaxRetries)
		if err != nil {
			t.Fatalf("不期望错误: %v", err)
		}
		if should {
			t.Fatalf("超过最大重试次数后应停止重试")
		}
	})
}
