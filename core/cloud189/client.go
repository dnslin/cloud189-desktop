package cloud189

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/gowsp/cloud189-desktop/core/auth"
	"github.com/gowsp/cloud189-desktop/core/httpclient"
)

// Client 扁平 API 封装，负责会话刷新与账号切换。
type Client struct {
	authManager *auth.AuthManager
	accountID   string
	http        *httpclient.Client
	logger      httpclient.Logger
	appBaseURL  string
	webBaseURL  string
	uploadBase  string
}

// Option 自定义客户端配置。
type Option func(*Client)

// WithHTTPClient 注入自定义 httpclient.Client。
func WithHTTPClient(cli *httpclient.Client) Option {
	return func(c *Client) {
		if cli != nil {
			c.http = cli
		}
	}
}

// WithLogger 注入日志接口。
func WithLogger(logger httpclient.Logger) Option {
	return func(c *Client) {
		if logger != nil {
			c.logger = logger
			if c.http != nil {
				c.http.Logger = logger
			}
		}
	}
}

// WithBaseURLs 替换默认的 App/Web/Upload 基础地址。
func WithBaseURLs(app, web, upload string) Option {
	return func(c *Client) {
		if app != "" {
			c.appBaseURL = app
		}
		if web != "" {
			c.webBaseURL = web
		}
		if upload != "" {
			c.uploadBase = upload
		}
	}
}

// NewClient 创建默认客户端。
func NewClient(authManager *auth.AuthManager, opts ...Option) *Client {
	cli := &Client{
		authManager: authManager,
		http:        httpclient.NewClient(),
		logger:      httpclient.NopLogger{},
		appBaseURL:  DefaultAppBaseURL,
		webBaseURL:  DefaultWebBaseURL,
		uploadBase:  DefaultUploadBaseURL,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(cli)
		}
	}
	if cli.http == nil {
		cli.http = httpclient.NewClient()
	}
	if cli.logger == nil {
		cli.logger = httpclient.NopLogger{}
	}
	cli.http.Logger = cli.logger
	cli.configureRetry()
	return cli
}

// WithAccount 切换当前账号 ID。
func (c *Client) WithAccount(accountID string) *Client {
	c.accountID = accountID
	return c
}

// AppGet 以 App 签名发送 GET。
func (c *Client) AppGet(ctx context.Context, path string, params map[string]string, out any) error {
	signer, err := c.prepareAppSigner(ctx)
	if err != nil {
		return err
	}
	return c.doRequest(ctx, http.MethodGet, c.appBaseURL, path, params, out, signer.Middleware())
}

// AppPost 以 App 签名发送 POST。
func (c *Client) AppPost(ctx context.Context, path string, params map[string]string, out any) error {
	signer, err := c.prepareAppSigner(ctx)
	if err != nil {
		return err
	}
	return c.doRequest(ctx, http.MethodPost, c.appBaseURL, path, params, out, signer.Middleware())
}

// WebGet 带 Cookie 的 Web GET。
func (c *Client) WebGet(ctx context.Context, path string, params map[string]string, out any) error {
	session, err := c.prepareSessionProvider(ctx)
	if err != nil {
		return err
	}
	mw := WithWebCookies(session)
	return c.doRequest(ctx, http.MethodGet, c.webBaseURL, path, params, out, mw)
}

// WebPost 带 Cookie 的 Web POST。
func (c *Client) WebPost(ctx context.Context, path string, params map[string]string, out any) error {
	session, err := c.prepareSessionProvider(ctx)
	if err != nil {
		return err
	}
	mw := WithWebCookies(session)
	return c.doRequest(ctx, http.MethodPost, c.webBaseURL, path, params, out, mw)
}

func (c *Client) prepareAppSigner(ctx context.Context) (*AppSigner, error) {
	session, err := c.prepareSessionProvider(ctx)
	if err != nil {
		return nil, err
	}
	return NewAppSigner(session), nil
}

func (c *Client) prepareWebSigner(ctx context.Context) (*WebSigner, error) {
	session, err := c.prepareSessionProvider(ctx)
	if err != nil {
		return nil, err
	}
	return NewWebSigner(session), nil
}

func (c *Client) prepareSessionProvider(ctx context.Context) (auth.SessionProvider, error) {
	if c == nil || c.authManager == nil {
		return nil, WrapCloudError(ErrCodeInvalidToken, "认证管理器未配置", errors.New("cloud189: AuthManager 未设置"))
	}
	if _, err := c.authManager.GetAccount(ctx, c.accountID); err != nil {
		return nil, ensureCloudError(ErrCodeInvalidToken, "获取会话失败", err)
	}
	provider, err := c.authManager.SessionProvider(c.accountID)
	if err != nil {
		return nil, ensureCloudError(ErrCodeInvalidToken, "获取会话失败", err)
	}
	return provider, nil
}

func (c *Client) refreshCurrent(ctx context.Context) error {
	if c == nil || c.authManager == nil {
		return WrapCloudError(ErrCodeInvalidToken, "认证管理器未配置", errors.New("cloud189: AuthManager 未设置"))
	}
	if err := c.authManager.RefreshAccount(ctx, c.accountID); err != nil {
		return WrapCloudError(ErrCodeInvalidToken, "凭证刷新失败", err)
	}
	return nil
}

func (c *Client) configureRetry() {
	if c.http == nil {
		return
	}
	cfg := httpclient.DefaultRetryConfig()
	cfg.Refresh = func() error { return c.refreshCurrent(context.Background()) }
	cfg.Logger = c.logger
	c.http.Retry = httpclient.NewExponentialBackoffRetry(cfg)
}

// useMiddlewares 在一次请求内临时追加中间件。
func (c *Client) useMiddlewares(req *http.Request, out any, mw ...httpclient.Middleware) error {
	if c.http == nil {
		return errors.New("cloud189: httpclient 未初始化")
	}
	orig := c.http.Prepare
	combined := append(httpclient.PrepareChain{}, orig...)
	combined = append(combined, mw...)
	c.http.Prepare = combined
	defer func() { c.http.Prepare = orig }()
	return c.http.Do(req, out)
}

func (c *Client) doRequest(ctx context.Context, method, base, path string, params map[string]string, out any, middlewares ...httpclient.Middleware) error {
	if c == nil {
		return WrapCloudError(ErrCodeInvalidRequest, "客户端未初始化", errors.New("cloud189: Client 未初始化"))
	}
	req, err := buildRequest(ctx, method, base, path, params)
	if err != nil {
		return WrapCloudError(ErrCodeInvalidRequest, "构建请求失败", err)
	}
	// 调试日志：打印请求路径
	fmt.Printf("[DEBUG] doRequest: %s %s%s\n", method, base, path)
	rawErr := c.useMiddlewares(req, out, middlewares...)
	if rawErr != nil {
		fmt.Printf("[DEBUG] doRequest 原始错误: %T %v\n", rawErr, rawErr)
	}
	return ensureCloudError(ErrCodeUnknown, "请求失败", toCloudError(rawErr))
}

func ensureCloudError(code int, message string, err error) error {
	if err == nil {
		return nil
	}
	if ce, ok := err.(*CloudError); ok {
		return ce
	}
	if conv, ok := toCloudError(err).(*CloudError); ok {
		return conv
	}
	return WrapCloudError(code, message, err)
}
