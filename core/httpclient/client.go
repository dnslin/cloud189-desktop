package httpclient

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"time"
)

// Logger 由外部注入，满足 core 层无输出原则。
type Logger interface {
	Debugf(format string, args ...any)
	Errorf(format string, args ...any)
}

// NopLogger 默认空日志实现。
type NopLogger struct{}

func (NopLogger) Debugf(string, ...any) {}
func (NopLogger) Errorf(string, ...any) {}

// Client 为统一 HTTP 客户端封装。
type Client struct {
	HTTP    *http.Client
	Jar     http.CookieJar
	Prepare PrepareChain
	Retry   RetryPolicy
	Limiter RateLimiter
	Logger  Logger
}

// Option 配置客户端。
type Option func(*Client)

// WithHTTPClient 自定义 http.Client。
func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Client) {
		c.HTTP = httpClient
	}
}

// WithCookieJar 设置 CookieJar。
func WithCookieJar(jar http.CookieJar) Option {
	return func(c *Client) {
		c.Jar = jar
	}
}

// WithRetryPolicy 设置重试策略。
func WithRetryPolicy(policy RetryPolicy) Option {
	return func(c *Client) {
		c.Retry = policy
	}
}

// WithRateLimiter 设置限流。
func WithRateLimiter(limiter RateLimiter) Option {
	return func(c *Client) {
		c.Limiter = limiter
	}
}

// WithLogger 注入日志。
func WithLogger(logger Logger) Option {
	return func(c *Client) {
		c.Logger = logger
	}
}

// WithMiddlewares 设置请求中间件链。
func WithMiddlewares(mw ...Middleware) Option {
	return func(c *Client) {
		c.Prepare = append(c.Prepare, mw...)
	}
}

// NewClient 创建带默认重试、CookieJar 的客户端。
func NewClient(opts ...Option) *Client {
	// cookiejar.New(nil) 仅在传入非 nil Options 且 PublicSuffixList 为 nil 时返回错误
	// 传入 nil 时不会返回错误，因此可以安全忽略
	jar, _ := cookiejar.New(nil)
	client := &Client{
		HTTP:    &http.Client{Jar: jar},
		Jar:     jar,
		Prepare: PrepareChain{},
		Logger:  NopLogger{},
	}
	client.Retry = NewExponentialBackoffRetry(DefaultRetryConfig())
	for _, opt := range opts {
		opt(client)
	}
	if client.HTTP == nil {
		client.HTTP = &http.Client{}
	}
	if client.Logger == nil {
		client.Logger = NopLogger{}
	}
	if client.Jar == nil {
		client.Jar = client.HTTP.Jar
	}
	if client.Jar == nil {
		j, _ := cookiejar.New(nil)
		client.Jar = j
	}
	if client.HTTP.Jar == nil {
		client.HTTP.Jar = client.Jar
	}
	return client
}

// Cookies 读取当前 jar 中的 cookies。
func (c *Client) Cookies(u *url.URL) []*http.Cookie {
	if c == nil || c.Jar == nil {
		return nil
	}
	return c.Jar.Cookies(u)
}

// Use 添加中间件。
func (c *Client) Use(mw ...Middleware) {
	c.Prepare = append(c.Prepare, mw...)
}

// Do 发送请求并按需解码 JSON，包含重试、限流、中间件。
func (c *Client) Do(req *http.Request, out any) error {
	if req == nil {
		return errors.New("httpclient: 请求为空")
	}
	if c.HTTP == nil {
		return errors.New("httpclient: http.Client 未配置")
	}
	attempt := 0
	for {
		clonedReq, cloneErr := c.cloneRequest(req, attempt)
		if cloneErr != nil {
			return cloneErr
		}
		resp, err := c.execute(clonedReq, out)
		if err == nil {
			return nil
		}
		if resp != nil && resp.Body != nil {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}
		if c.Retry == nil {
			return err
		}
		retry, wait, refreshErr := c.Retry.ShouldRetry(clonedReq, resp, err, attempt)
		if refreshErr != nil {
			return refreshErr
		}
		if !retry {
			return err
		}
		attempt++
		if wait > 0 {
			time.Sleep(wait)
		}
	}
}

func (c *Client) execute(req *http.Request, out any) (*http.Response, error) {
	if c.Prepare != nil {
		if err := c.Prepare.Apply(req); err != nil {
			return nil, err
		}
	}
	if c.Limiter != nil {
		if err := c.Limiter.Wait(req.Context(), req); err != nil {
			return nil, err
		}
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, &NetworkError{Err: err}
	}
	if out == nil {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode >= http.StatusInternalServerError {
			return resp, statusToErr(resp.StatusCode)
		}
		return resp, nil
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusInternalServerError {
		return resp, statusToErr(resp.StatusCode)
	}
	if resp.StatusCode >= http.StatusBadRequest {
		var ec ErrCode
		if decodeErr := json.NewDecoder(resp.Body).Decode(&ec); decodeErr == nil {
			if ec.Status == 0 {
				ec.Status = resp.StatusCode
			}
			if ec.Message == "" {
				ec.Message = http.StatusText(resp.StatusCode)
			}
			return resp, &ec
		}
		return resp, statusToErr(resp.StatusCode)
	}

	dec := json.NewDecoder(resp.Body)
	dec.UseNumber() // 保留数字精度
	if decodeErr := dec.Decode(out); decodeErr != nil {
		if decodeErr == io.EOF {
			// 空响应体，视为成功
			return resp, nil
		}
		return resp, &DecodeError{Status: resp.StatusCode, Err: decodeErr}
	}
	if ok, okType := out.(OkRsp); okType && !ok.IsSuccess() {
		return resp, toErrCode(ok, resp.StatusCode)
	}
	return resp, nil
}

func (c *Client) cloneRequest(req *http.Request, attempt int) (*http.Request, error) {
	cloned := req.Clone(req.Context())
	cloned.Header = req.Header.Clone()
	cloned.GetBody = req.GetBody
	cloned.ContentLength = req.ContentLength
	cloned.TransferEncoding = append([]string(nil), req.TransferEncoding...)
	if req.Body != nil {
		if attempt == 0 {
			cloned.Body = req.Body
		} else {
			if req.GetBody == nil {
				return nil, fmt.Errorf("httpclient: 请求体不可重试")
			}
			body, err := req.GetBody()
			if err != nil {
				return nil, err
			}
			cloned.Body = body
		}
	}
	return cloned, nil
}
