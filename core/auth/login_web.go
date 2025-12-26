package auth

import (
	"context"
	"fmt"
	"net/http"

	coreerrors "github.com/dnslin/cloud189-desktop/core/errors"
)

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
		return nil, coreerrors.New(coreerrors.ErrCodeInvalidState, "auth: 登录缺少跳转地址")
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
		return nil, coreerrors.New(coreerrors.ErrCodeInvalidState, "auth: 登录后未返回 COOKIE_LOGIN_USER")
	}
	return session, nil
}
