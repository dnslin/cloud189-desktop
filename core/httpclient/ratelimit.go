package httpclient

import (
	"context"
	"net/http"
	"sync"
	"time"
)

// Limit 每秒产生的令牌数。
type Limit float64

// Limiter 简化版令牌桶实现，兼容常见 Wait 接口。
type Limiter struct {
	limit  Limit
	burst  int
	mu     sync.Mutex
	tokens float64
	last   time.Time
}

// NewLimiter 创建 limiter。
func NewLimiter(limit Limit, burst int) *Limiter {
	now := time.Now()
	return &Limiter{
		limit:  limit,
		burst:  burst,
		tokens: float64(burst),
		last:   now,
	}
}

// Wait 阻塞直到获得令牌或上下文取消。
func (l *Limiter) Wait(ctx context.Context) error {
	for {
		wait := l.reserve(time.Now())
		if wait <= 0 {
			return nil
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (l *Limiter) reserve(now time.Time) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.limit <= 0 {
		return 0
	}
	elapsed := now.Sub(l.last).Seconds()
	l.tokens += elapsed * float64(l.limit)
	if l.tokens > float64(l.burst) {
		l.tokens = float64(l.burst)
	}
	if l.tokens >= 1 {
		l.tokens -= 1
		l.last = now
		return 0
	}
	need := 1 - l.tokens
	l.last = now
	seconds := need / float64(l.limit)
	return time.Duration(seconds * float64(time.Second))
}

// RateLimiter 按 host/路由限流。
type RateLimiter interface {
	Wait(ctx context.Context, req *http.Request) error
}

// TokenBucketLimiter 基于令牌桶的限流实现。
type TokenBucketLimiter struct {
	limiters map[string]*Limiter
	mu       sync.Mutex
	keyFn    func(*http.Request) string
	limit    Limit
	burst    int
}

// NewTokenBucketLimiter 创建按 key（host/路由）区分的限流器。
func NewTokenBucketLimiter(qps float64, burst int, keyFn func(*http.Request) string) *TokenBucketLimiter {
	return &TokenBucketLimiter{
		limiters: make(map[string]*Limiter),
		keyFn:    keyFn,
		limit:    Limit(qps),
		burst:    burst,
	}
}

// Wait 在发起请求前阻塞，直到当前 key 拿到令牌。
func (l *TokenBucketLimiter) Wait(ctx context.Context, req *http.Request) error {
	if l == nil || l.limit <= 0 {
		return nil
	}
	limiter := l.getLimiter(req)
	return limiter.Wait(ctx)
}

func (l *TokenBucketLimiter) getLimiter(req *http.Request) *Limiter {
	key := ""
	if req != nil && req.URL != nil {
		key = req.URL.Host
	}
	if l.keyFn != nil {
		if k := l.keyFn(req); k != "" {
			key = k
		}
	}
	if key == "" {
		key = "default"
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if limiter, ok := l.limiters[key]; ok {
		return limiter
	}
	limiter := NewLimiter(l.limit, l.burst)
	l.limiters[key] = limiter
	return limiter
}
