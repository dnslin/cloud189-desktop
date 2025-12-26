package auth

import (
	"context"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/dnslin/cloud189-desktop/core/crypto"
)

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
