package cloud189

import (
	"errors"
	"net/http"

	"github.com/gowsp/cloud189-desktop/core/auth"
	"github.com/gowsp/cloud189-desktop/core/httpclient"
)

// Client 统一封装 App/Web API 调用与签名。
type Client struct {
	http       *httpclient.Client
	session    auth.SessionProvider
	logger     httpclient.Logger
	appSigner  *AppSigner
	webSigner  *WebSigner
	appBaseURL string
	webBaseURL string
	uploadBase string
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

// WithSessionProvider 替换 SessionProvider。
func WithSessionProvider(sp auth.SessionProvider) Option {
	return func(c *Client) {
		c.session = sp
	}
}

// WithSigners 注入自定义签名器，便于测试。
func WithSigners(app *AppSigner, web *WebSigner) Option {
	return func(c *Client) {
		if app != nil {
			c.appSigner = app
		}
		if web != nil {
			c.webSigner = web
		}
	}
}

// NewClient 创建默认客户端。
func NewClient(session auth.SessionProvider, opts ...Option) *Client {
	cli := &Client{
		http:       httpclient.NewClient(),
		session:    session,
		logger:     httpclient.NopLogger{},
		appBaseURL: DefaultAppBaseURL,
		webBaseURL: DefaultWebBaseURL,
		uploadBase: DefaultUploadBaseURL,
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
	if cli.appSigner == nil {
		cli.appSigner = NewAppSigner(cli.session)
	}
	if cli.webSigner == nil {
		cli.webSigner = NewWebSigner(cli.session)
	}
	return cli
}

// useMiddlewares 在一次请求内临时追加中间件。
func (c *Client) useMiddlewares(req *http.Request, out any, mw ...httpclient.Middleware) error {
	if c.http == nil {
		return &httpclient.NetworkError{Err: errors.New("cloud189: httpclient 未初始化")}
	}
	orig := c.http.Prepare
	combined := append(httpclient.PrepareChain{}, orig...)
	combined = append(combined, mw...)
	c.http.Prepare = combined
	defer func() { c.http.Prepare = orig }()
	return c.http.Do(req, out)
}
