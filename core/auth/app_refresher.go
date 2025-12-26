package auth

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/dnslin/cloud189-desktop/core/httpclient"
	"github.com/dnslin/cloud189-desktop/core/store"
)

// AppRefresher 使用 accessToken 刷新 Session，失败时回退密码登录。
type AppRefresher struct {
	client     *httpclient.Client
	store      store.SessionStore[Session]
	login      *LoginClient
	creds      Credentials
	refreshURL string
	appID      string
	now        func() time.Time
	logger     httpclient.Logger
}

// AppRefresherOption 自定义 AppRefresher。
type AppRefresherOption func(*AppRefresher)

// WithAppRefreshURL 替换刷新接口地址。
func WithAppRefreshURL(url string) AppRefresherOption {
	return func(r *AppRefresher) {
		r.refreshURL = url
	}
}

// WithAppID 替换 appId。
func WithAppID(appID string) AppRefresherOption {
	return func(r *AppRefresher) {
		r.appID = appID
	}
}

// WithAppLogger 注入日志。
func WithAppLogger(logger httpclient.Logger) AppRefresherOption {
	return func(r *AppRefresher) {
		r.logger = logger
	}
}

// WithAppNow 替换时间来源。
func WithAppNow(now func() time.Time) AppRefresherOption {
	return func(r *AppRefresher) {
		r.now = now
	}
}

// NewAppRefresher 创建 App 端刷新器。
func NewAppRefresher(client *httpclient.Client, store store.SessionStore[Session], login *LoginClient, creds Credentials, opts ...AppRefresherOption) *AppRefresher {
	if client == nil {
		client = httpclient.NewClient()
	}
	if login == nil {
		login = NewLoginClient(client)
	}
	r := &AppRefresher{
		client:     client,
		store:      store,
		login:      login,
		creds:      creds,
		refreshURL: "https://api.cloud.189.cn/getSessionForPC.action",
		appID:      "9317140619",
		now:        time.Now,
		logger:     httpclient.NopLogger{},
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

// Refresh 按优先级刷新会话。
func (r *AppRefresher) Refresh(ctx context.Context) error {
	if r.store == nil {
		return ErrSessionStoreNil
	}
	session, err := r.store.LoadSession()
	if err != nil && !errors.Is(err, ErrSessionNotFound) {
		return err
	}

	if session != nil && session.AccessToken != "" {
		if refreshed, refreshErr := r.refreshByToken(ctx, session.AccessToken); refreshErr == nil {
			// 保留无法从接口返回的字段。
			refreshed.SSON = session.SSON
			refreshed.CookieLoginUser = session.CookieLoginUser
			if refreshed.AccessToken == "" {
				refreshed.AccessToken = session.AccessToken
			}
			return r.store.SaveSession(refreshed)
		} else {
			r.logger.Errorf("accessToken 刷新失败，准备回退密码登录: %v", refreshErr)
		}
	}

	if r.creds.Username == "" || r.creds.Password == "" {
		return ErrMissingCredentials
	}
	newSession, err := r.login.AppLogin(ctx, r.creds)
	if err != nil {
		return err
	}
	return r.store.SaveSession(newSession)
}

// NeedsRefresh 判断当前会话是否需要刷新。
func (r *AppRefresher) NeedsRefresh() bool {
	if r.store == nil {
		return true
	}
	session, err := r.store.LoadSession()
	if err != nil || session == nil {
		return true
	}
	if session.SessionKey == "" || session.SessionSecret == "" {
		return true
	}
	return session.Expired(r.now())
}

func (r *AppRefresher) refreshByToken(ctx context.Context, accessToken string) (*Session, error) {
	form := url.Values{}
	form.Set("appId", r.appID)
	form.Set("accessToken", accessToken)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.refreshURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json;charset=UTF-8")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	var payload struct {
		Session
		KeepAlive int `json:"keepAlive,omitempty"`
		ExpiresIn int `json:"expiresIn,omitempty"`
	}
	if err := r.client.Do(req, &payload); err != nil {
		return nil, err
	}
	session := payload.Session
	if payload.KeepAlive > 0 {
		session.ExpiresAt = r.now().Add(time.Duration(payload.KeepAlive) * time.Second)
	} else if payload.ExpiresIn > 0 {
		session.ExpiresAt = r.now().Add(time.Duration(payload.ExpiresIn) * time.Second)
	}
	return &session, nil
}
