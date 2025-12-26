package auth

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

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
