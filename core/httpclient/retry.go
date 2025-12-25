package httpclient

import (
	"errors"
	"net/http"
	"time"
)

// RetryPolicy 定义重试策略。
type RetryPolicy interface {
	ShouldRetry(req *http.Request, resp *http.Response, err error, attempt int) (bool, time.Duration, error)
}

// RetryConfig 配置指数退避重试。
type RetryConfig struct {
	MaxRetries int
	BaseDelay  time.Duration
	MaxDelay   time.Duration
	Refresh    func() error
	AuthCodes  []string
	Logger     Logger
}

// ExponentialBackoffRetry 实现指数退避重试。
type ExponentialBackoffRetry struct {
	maxRetries int
	baseDelay  time.Duration
	maxDelay   time.Duration
	refresh    func() error
	authCodes  map[string]struct{}
	logger     Logger
}

func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries: 3,
		BaseDelay:  200 * time.Millisecond,
		MaxDelay:   2 * time.Second,
		AuthCodes: []string{
			"InvalidSignature",
			"InvalidSessionKey",
			"InvalidAccessToken",
		},
	}
}

// NewExponentialBackoffRetry 创建重试策略。
func NewExponentialBackoffRetry(cfg RetryConfig) *ExponentialBackoffRetry {
	authCodes := make(map[string]struct{})
	for _, code := range cfg.AuthCodes {
		authCodes[code] = struct{}{}
	}
	logger := cfg.Logger
	if logger == nil {
		logger = NopLogger{}
	}
	return &ExponentialBackoffRetry{
		maxRetries: cfg.MaxRetries,
		baseDelay:  cfg.BaseDelay,
		maxDelay:   cfg.MaxDelay,
		refresh:    cfg.Refresh,
		authCodes:  authCodes,
		logger:     logger,
	}
}

// ShouldRetry 根据错误类型、状态码决定是否重试。
func (r *ExponentialBackoffRetry) ShouldRetry(req *http.Request, resp *http.Response, err error, attempt int) (bool, time.Duration, error) {
	if r == nil {
		return false, 0, nil
	}
	if attempt >= r.maxRetries {
		return false, 0, nil
	}
	delay := r.backoff(attempt)

	if resp != nil && resp.StatusCode >= http.StatusInternalServerError {
		r.logger.Debugf("服务端错误，第 %d 次重试", attempt+1)
		return true, delay, nil
	}

	var netErr *NetworkError
	if errors.As(err, &netErr) {
		r.logger.Debugf("网络错误，第 %d 次重试", attempt+1)
		return true, delay, nil
	}

	var decErr *DecodeError
	if errors.As(err, &decErr) {
		return false, 0, nil
	}

	var ec *ErrCode
	if errors.As(err, &ec) {
		if ec.Status >= http.StatusInternalServerError {
			r.logger.Debugf("服务端错误(code=%d)，第 %d 次重试", ec.Status, attempt+1)
			return true, delay, nil
		}
		if r.isAuth(ec) {
			if r.refresh != nil {
				if refreshErr := r.refresh(); refreshErr != nil {
					return false, 0, refreshErr
				}
			}
			r.logger.Debugf("认证错误，刷新后重试，第 %d 次", attempt+1)
			return true, delay, nil
		}
		return false, 0, nil
	}

	return false, 0, nil
}

func (r *ExponentialBackoffRetry) isAuth(ec *ErrCode) bool {
	if ec == nil {
		return false
	}
	if ec.Status == http.StatusUnauthorized || ec.Status == http.StatusForbidden {
		return true
	}
	if ec.Code == "" {
		return false
	}
	_, ok := r.authCodes[ec.Code]
	return ok
}

func (r *ExponentialBackoffRetry) backoff(attempt int) time.Duration {
	base := r.baseDelay
	if base <= 0 {
		base = 200 * time.Millisecond
	}
	max := r.maxDelay
	if max <= 0 {
		max = 2 * time.Second
	}
	delay := base << attempt
	if delay > max {
		delay = max
	}
	return delay
}
