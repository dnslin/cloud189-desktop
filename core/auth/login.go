package auth

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gowsp/cloud189-desktop/core/crypto"
	"github.com/gowsp/cloud189-desktop/core/httpclient"
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

// AppLogin 执行 App 端用户名密码登录，并换取会话。
func (l *LoginClient) AppLogin(ctx context.Context, creds Credentials) (*Session, error) {
	if err := l.validateCreds(creds); err != nil {
		return nil, err
	}
	result, _, err := l.passwordLogin(ctx, l.endpoints.AppLoginURL, l.beforeLoginParams(), creds)
	if err != nil {
		return nil, err
	}
	session, err := l.exchangeSession(ctx, result.ToURL)
	if err != nil {
		return nil, err
	}
	session.SSON = result.SSON
	return session, nil
}

// WebLogin 执行 Web 端用户名密码登录，刷新 Cookie。
func (l *LoginClient) WebLogin(ctx context.Context, creds Credentials) (*Session, error) {
	if err := l.validateCreds(creds); err != nil {
		return nil, err
	}
	result, _, err := l.passwordLogin(ctx, l.endpoints.WebLoginURL, nil, creds)
	if err != nil {
		return nil, err
	}
	if result.ToURL == "" {
		return nil, errors.New("auth: 登录缺少跳转地址")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, result.ToURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := l.client.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		return nil, fmt.Errorf("auth: 登录跳转失败，状态码 %d", resp.StatusCode)
	}
	cookies := l.client.Cookies(resp.Request.URL)
	session := &Session{
		SSON:            pickNonEmpty(result.SSON, findCookieValue(cookies, "SSON")),
		CookieLoginUser: findCookieValue(cookies, "COOKIE_LOGIN_USER"),
	}
	if session.CookieLoginUser == "" {
		return nil, errors.New("auth: 登录后未返回 COOKIE_LOGIN_USER")
	}
	return session, nil
}

func (l *LoginClient) validateCreds(creds Credentials) error {
	if creds.Username == "" || creds.Password == "" {
		return ErrMissingCredentials
	}
	return nil
}

func (l *LoginClient) passwordLogin(ctx context.Context, loginURL string, params url.Values, creds Credentials) (*loginResult, *loginContext, error) {
	loginCtx, err := l.prepareLogin(ctx, loginURL, params)
	if err != nil {
		return nil, nil, err
	}
	appConf, err := l.fetchAppConf(ctx, loginCtx)
	if err != nil {
		return nil, nil, err
	}
	encryptConf, err := l.fetchEncryptConf(ctx, loginCtx)
	if err != nil {
		return nil, nil, err
	}
	req, err := l.buildPwdRequest(ctx, loginCtx, appConf, encryptConf, creds)
	if err != nil {
		return nil, nil, err
	}
	var result loginResult
	if err := l.client.Do(req, &result); err != nil {
		return nil, nil, err
	}
	if result.Result != 0 {
		return nil, nil, fmt.Errorf("auth: 登录失败: %s", result.Msg)
	}
	cookies := l.client.Cookies(req.URL)
	if result.SSON == "" {
		result.SSON = findCookieValue(cookies, "SSON")
	}
	return &result, loginCtx, nil
}

func (l *LoginClient) prepareLogin(ctx context.Context, loginURL string, params url.Values) (*loginContext, error) {
	u := loginURL
	if len(params) > 0 {
		u += "?" + params.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := l.client.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// 获取重定向后的 location（参考项目使用 resp.Request.Response.Header.Get("location")）
	var referer string
	if resp.Request.Response != nil {
		referer = resp.Request.Response.Header.Get("Location")
	}
	if referer == "" {
		referer = resp.Request.URL.String()
	}

	refURL, _ := url.Parse(referer)
	var q url.Values
	if refURL != nil {
		q = refURL.Query()
	} else {
		q = resp.Request.URL.Query()
	}

	appKey := q.Get("appId")
	if appKey == "" && params != nil {
		appKey = params.Get("appId")
	}
	return &loginContext{
		Referer: referer,
		AppKey:  appKey,
		ReqID:   q.Get("reqId"),
		Lt:      q.Get("lt"),
	}, nil
}

func (l *LoginClient) fetchAppConf(ctx context.Context, content *loginContext) (*appConf, error) {
	form := url.Values{}
	form.Set("version", "2.0")
	form.Set("appKey", content.AppKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, l.endpoints.AppConfURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "https://open.e.189.cn")
	req.Header.Set("Referer", content.Referer)
	if content.ReqID != "" {
		req.Header.Set("Reqid", content.ReqID)
	}
	if content.Lt != "" {
		req.Header.Set("lt", content.Lt)
	}
	var conf appConf
	if err := l.client.Do(req, &conf); err != nil {
		return nil, err
	}
	return &conf, nil
}

func (l *LoginClient) fetchEncryptConf(ctx context.Context, content *loginContext) (*encryptConf, error) {
	form := url.Values{}
	form.Set("appId", "cloud")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, l.endpoints.EncryptConfURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", content.Referer)
	var conf encryptConf
	if err := l.client.Do(req, &conf); err != nil {
		return nil, err
	}
	return &conf, nil
}

func (l *LoginClient) buildPwdRequest(ctx context.Context, loginCtx *loginContext, appConf *appConf, encryptConf *encryptConf, creds Credentials) (*http.Request, error) {
	pubKey := crypto.WrapRSAPubKey(encryptConf.Data.PubKey)
	encUser, err := crypto.Encrypt(pubKey, []byte(creds.Username))
	if err != nil {
		return nil, err
	}
	encPwd, err := crypto.Encrypt(pubKey, []byte(creds.Password))
	if err != nil {
		return nil, err
	}
	userParam := encryptConf.Data.Pre + hex.EncodeToString(encUser)
	pwdParam := encryptConf.Data.Pre + hex.EncodeToString(encPwd)

	params := make(url.Values)
	params.Set("version", "v2.0")
	params.Set("appKey", appConf.Data.AppKey)
	params.Set("accountType", appConf.Data.AccountType)
	params.Set("userName", userParam)
	params.Set("epd", pwdParam)
	params.Set("captchaType", "")
	params.Set("validateCode", "")
	params.Set("smsValidateCode", "")
	params.Set("captchaToken", "")
	params.Set("returnUrl", loginCtx.Referer)
	params.Set("mailSuffix", appConf.Data.MailSuffix)
	params.Set("dynamicCheck", "FALSE")
	params.Set("clientType", fmt.Sprintf("%d", appConf.Data.ClientType))
	params.Set("cb_SaveName", "0")
	params.Set("isOauth2", fmt.Sprintf("%t", appConf.Data.IsOauth2))
	params.Set("state", "")
	params.Set("paramId", appConf.Data.ParamID)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, l.endpoints.LoginSubmitURL, strings.NewReader(params.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", loginCtx.Referer)
	if loginCtx.ReqID != "" {
		req.Header.Set("Reqid", loginCtx.ReqID)
	}
	if loginCtx.Lt != "" {
		req.Header.Set("lt", loginCtx.Lt)
	}
	return req, nil
}

func (l *LoginClient) exchangeSession(ctx context.Context, redirect string) (*Session, error) {
	params := url.Values{}
	params.Set("redirectURL", redirect)
	// 添加必要的客户端参数（参考 cloud189-example/pkg/app/api.go sign 方法）
	params.Set("clientType", "TELEPC")
	params.Set("version", "7.1.8.0")
	params.Set("channelId", "web_cloud.189.cn")
	params.Set("rand", fmt.Sprintf("%d", l.now().UnixMilli()))

	body := params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, l.endpoints.SessionURL, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json;charset=UTF-8")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader(body)), nil
	}
	var session Session
	if err := l.client.Do(req, &session); err != nil {
		return nil, err
	}
	return &session, nil
}

func (l *LoginClient) beforeLoginParams() url.Values {
	params := url.Values{}
	params.Set("appId", "9317140619")
	params.Set("clientType", "10020")
	params.Set("timeStamp", fmt.Sprintf("%d", l.now().UnixMilli()))
	params.Set("returnURL", "https://m.cloud.189.cn/zhuanti/2020/loginErrorPc/index.html")
	return params
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

type loginContext struct {
	Referer string
	AppKey  string
	ReqID   string
	Lt      string
}

type appConf struct {
	Data struct {
		AccountType string `json:"accountType"`
		AppKey      string `json:"appKey"`
		ClientType  int    `json:"clientType"`
		MailSuffix  string `json:"mailSuffix"`
		IsOauth2    bool   `json:"isOauth2"`
		ParamID     string `json:"paramId"`
	} `json:"data"`
}

type encryptConf struct {
	Data struct {
		Pre    string `json:"pre"`
		PubKey string `json:"pubKey"`
	} `json:"data"`
}

type loginResult struct {
	Result int    `json:"result,omitempty"`
	Msg    string `json:"msg,omitempty"`
	ToURL  string `json:"toUrl,omitempty"`
	SSON   string
}

func findCookieValue(cookies []*http.Cookie, name string) string {
	for _, c := range cookies {
		if c != nil && c.Name == name {
			return c.Value
		}
	}
	return ""
}

func pickNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// ErrMissingCredentials 标记缺少用户名或密码。
var ErrMissingCredentials = errors.New("auth: 缺少登录凭证")
