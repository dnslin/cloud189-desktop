package auth

import (
	"time"

	coreerrors "github.com/dnslin/cloud189-desktop/core/errors"
	"github.com/dnslin/cloud189-desktop/core/httpclient"
)

// Credentials 表示账号口令组合。
type Credentials struct {
	Username string
	Password string
}

// LoginEndpoints 允许替换登录相关接口地址，便于测试或自定义环境。
type LoginEndpoints struct {
	AppLoginURL    string
	WebLoginURL    string
	AppConfURL     string
	EncryptConfURL string
	LoginSubmitURL string
	SessionURL     string
}

// LoginClient 负责用户名密码登录的全流程。
type LoginClient struct {
	client    *httpclient.Client
	logger    httpclient.Logger
	endpoints LoginEndpoints
	now       func() time.Time
}

// LoginOption 自定义登录客户端。
type LoginOption func(*LoginClient)

// WithLoginLogger 注入日志。
func WithLoginLogger(logger httpclient.Logger) LoginOption {
	return func(l *LoginClient) {
		l.logger = logger
	}
}

// WithLoginEndpoints 替换默认接口地址。
func WithLoginEndpoints(ep LoginEndpoints) LoginOption {
	return func(l *LoginClient) {
		l.endpoints = ep
	}
}

// WithLoginNow 替换时间来源，便于测试。
func WithLoginNow(now func() time.Time) LoginOption {
	return func(l *LoginClient) {
		l.now = now
	}
}

// NewLoginClient 创建登录客户端。
func NewLoginClient(client *httpclient.Client, opts ...LoginOption) *LoginClient {
	if client == nil {
		client = httpclient.NewClient()
	}
	l := &LoginClient{
		client:    client,
		logger:    httpclient.NopLogger{},
		endpoints: defaultLoginEndpoints(),
		now:       time.Now,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(l)
		}
	}
	if l.logger == nil {
		l.logger = httpclient.NopLogger{}
	}
	return l
}

func defaultLoginEndpoints() LoginEndpoints {
	return LoginEndpoints{
		AppLoginURL:    "https://cloud.189.cn/unifyLoginForPC.action",
		WebLoginURL:    "https://cloud.189.cn/api/portal/loginUrl.action",
		AppConfURL:     "https://open.e.189.cn/api/logbox/oauth2/appConf.do",
		EncryptConfURL: "https://open.e.189.cn/api/logbox/config/encryptConf.do",
		LoginSubmitURL: "https://open.e.189.cn/api/logbox/oauth2/loginSubmit.do",
		SessionURL:     "https://api.cloud.189.cn/getSessionForPC.action",
	}
}

// ErrMissingCredentials 标记缺少用户名或密码。
var ErrMissingCredentials = coreerrors.New(coreerrors.ErrCodeInvalidArgument, "auth: 缺少登录凭证")
