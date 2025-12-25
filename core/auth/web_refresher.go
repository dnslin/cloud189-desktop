package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gowsp/cloud189-desktop/core/httpclient"
)

// WebRefresher 通过访问登录页刷新 Cookie，失败回退密码登录。
type WebRefresher struct {
	client   *httpclient.Client
	store    SessionStore
	login    *LoginClient
	creds    Credentials
	loginURL string
	now      func() time.Time
	logger   httpclient.Logger
}

// WebRefresherOption 自定义 Web 刷新器。
type WebRefresherOption func(*WebRefresher)

// WithWebLoginURL 替换登录刷新地址。
func WithWebLoginURL(url string) WebRefresherOption {
	return func(r *WebRefresher) {
		r.loginURL = url
	}
}

// WithWebLogger 注入日志。
func WithWebLogger(logger httpclient.Logger) WebRefresherOption {
	return func(r *WebRefresher) {
		r.logger = logger
	}
}

// WithWebNow 替换时间来源。
func WithWebNow(now func() time.Time) WebRefresherOption {
	return func(r *WebRefresher) {
		r.now = now
	}
}

// NewWebRefresher 创建 Web 端刷新器。
func NewWebRefresher(client *httpclient.Client, store SessionStore, login *LoginClient, creds Credentials, opts ...WebRefresherOption) *WebRefresher {
	if client == nil {
		client = httpclient.NewClient()
	}
	if login == nil {
		login = NewLoginClient(client)
	}
	r := &WebRefresher{
		client:   client,
		store:    store,
		login:    login,
		creds:    creds,
		loginURL: "https://cloud.189.cn/api/portal/loginUrl.action",
		now:      time.Now,
		logger:   httpclient.NopLogger{},
	}
	for _, opt := range opts {
		if opt != nil {
			opt(r)
		}
	}
	if r.logger == nil {
		r.logger = httpclient.NopLogger{}
	}
	return r
}

// Refresh 优先刷新 Cookie，不成功则回退登录。
func (r *WebRefresher) Refresh(ctx context.Context) error {
	if r.store == nil {
		return ErrSessionStoreNil
	}
	session, err := r.store.LoadSession()
	if err != nil && !errors.Is(err, ErrSessionNotFound) {
		return err
	}

	if refreshed, refreshErr := r.refreshCookie(ctx, session); refreshErr == nil {
		return r.store.SaveSession(refreshed)
	} else {
		r.logger.Errorf("COOKIE_LOGIN_USER 刷新失败，准备回退密码登录: %v", refreshErr)
	}

	if r.creds.Username == "" || r.creds.Password == "" {
		return ErrMissingCredentials
	}
	newSession, err := r.login.WebLogin(ctx, r.creds)
	if err != nil {
		return err
	}
	return r.store.SaveSession(newSession)
}

// NeedsRefresh 判断 Cookie 是否缺失或过期。
func (r *WebRefresher) NeedsRefresh() bool {
	if r.store == nil {
		return true
	}
	session, err := r.store.LoadSession()
	if err != nil || session == nil {
		return true
	}
	if session.CookieLoginUser == "" {
		return true
	}
	return session.Expired(r.now())
}

func (r *WebRefresher) refreshCookie(ctx context.Context, session *Session) (*Session, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.loginURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := r.client.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		return nil, fmt.Errorf("auth: 刷新登录失败，状态码 %d", resp.StatusCode)
	}
	cookies := r.client.Cookies(resp.Request.URL)
	user := findCookieValue(cookies, "COOKIE_LOGIN_USER")
	if user == "" {
		return nil, errors.New("auth: 登录刷新未返回 COOKIE_LOGIN_USER")
	}
	var result *Session
	if session != nil {
		result = session.Clone()
	} else {
		result = &Session{}
	}
	result.CookieLoginUser = user
	if result.SSON == "" {
		result.SSON = findCookieValue(cookies, "SSON")
	}
	result.ExpiresAt = time.Time{}
	return result, nil
}
