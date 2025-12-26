package auth

import (
	"context"
	"fmt"
	"net/http"

	coreerrors "github.com/dnslin/cloud189-desktop/core/errors"
)

// WebLogin 执行 Web 端用户名密码登录，刷新 Cookie。
func (l *LoginClient) WebLogin(ctx context.Context, creds Credentials) (*Session, error) {
	return l.webLogin(ctx, creds, false)
}

// HybridLogin 一次登录同时获取 Web Cookie 和 APP 会话。
// 流程：WebLoginURL 登录 → 访问 ToUrl 获取 Cookie → getSessionForPC 获取 SessionKey
func (l *LoginClient) HybridLogin(ctx context.Context, creds Credentials) (*Session, error) {
	if err := l.validateCreds(creds); err != nil {
		return nil, err
	}
	// 1. Web 登录获取 ToUrl 和 SSON
	result, _, err := l.passwordLogin(ctx, l.endpoints.WebLoginURL, nil, creds)
	if err != nil {
		return nil, err
	}
	if result.ToURL == "" {
		return nil, coreerrors.New(coreerrors.ErrCodeInvalidState, "auth: 登录缺少跳转地址")
	}
	// 2. 访问 ToUrl 获取 COOKIE_LOGIN_USER
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, result.ToURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := l.client.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	cookies := l.client.Cookies(resp.Request.URL)
	cookieLoginUser := findCookieValue(cookies, "COOKIE_LOGIN_USER")
	sson := pickNonEmpty(result.SSON, findCookieValue(cookies, "SSON"))

	// 3. APP 登录获取 SessionKey/SessionSecret
	appResult, _, err := l.passwordLogin(ctx, l.endpoints.AppLoginURL, l.beforeLoginParams(), creds)
	if err != nil {
		return nil, err
	}
	session, err := l.exchangeSession(ctx, appResult.ToURL)
	if err != nil {
		return nil, err
	}
	// 合并 Web Cookie 和 APP Session
	session.SSON = pickNonEmpty(session.SSON, sson, appResult.SSON)
	session.CookieLoginUser = cookieLoginUser
	return session, nil
}

func (l *LoginClient) webLogin(ctx context.Context, creds Credentials, _ bool) (*Session, error) {
	if err := l.validateCreds(creds); err != nil {
		return nil, err
	}
	result, _, err := l.passwordLogin(ctx, l.endpoints.WebLoginURL, nil, creds)
	if err != nil {
		return nil, err
	}
	if result.ToURL == "" {
		return nil, coreerrors.New(coreerrors.ErrCodeInvalidState, "auth: 登录缺少跳转地址")
	}

	session := &Session{}

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
	session.SSON = pickNonEmpty(result.SSON, findCookieValue(cookies, "SSON"))
	session.CookieLoginUser = findCookieValue(cookies, "COOKIE_LOGIN_USER")
	if session.CookieLoginUser == "" {
		return nil, coreerrors.New(coreerrors.ErrCodeInvalidState, "auth: 登录后未返回 COOKIE_LOGIN_USER")
	}
	return session, nil
}
