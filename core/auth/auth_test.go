package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gowsp/cloud189-desktop/core/httpclient"
)

type memoryStore struct {
	session  *Session
	loadErr  error
	saveErr  error
	clearErr error
}

func (m *memoryStore) SaveSession(session *Session) error {
	if m.saveErr != nil {
		return m.saveErr
	}
	m.session = session.Clone()
	return nil
}

func (m *memoryStore) LoadSession() (*Session, error) {
	if m.loadErr != nil {
		return nil, m.loadErr
	}
	if m.session == nil {
		return nil, ErrSessionNotFound
	}
	return m.session.Clone(), nil
}

func (m *memoryStore) ClearSession() error {
	if m.clearErr != nil {
		return m.clearErr
	}
	m.session = nil
	return nil
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := f(req)
	if resp != nil && resp.Request == nil {
		resp.Request = req
	}
	return resp, err
}

func TestAppLoginFlow(t *testing.T) {
	pubKey, privKey := generateRSAKey(t)
	base := "https://mock.local"
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/unifyLoginForPC.action":
			return redirectResponse(base + "/page?reqId=req-1&lt=lt-1&appId=appid-1"), nil
		case "/page":
			return jsonResponse(http.StatusOK, ``), nil
		case "/api/logbox/oauth2/appConf.do":
			return jsonResponse(http.StatusOK, `{"data":{"accountType":"01","appKey":"9317140619","clientType":10020,"mailSuffix":"","isOauth2":true,"paramId":"pid"}}`), nil
		case "/api/logbox/config/encryptConf.do":
			return jsonResponse(http.StatusOK, `{"result":0,"data":{"pre":"pre-","pubKey":"`+pubKey+`"}}`), nil
		case "/api/logbox/oauth2/loginSubmit.do":
			if err := r.ParseForm(); err != nil {
				t.Fatalf("解析表单失败: %v", err)
			}
			checkEncrypted(t, privKey, strings.TrimPrefix(r.Form.Get("userName"), "pre-"), "user-app")
			checkEncrypted(t, privKey, strings.TrimPrefix(r.Form.Get("epd"), "pre-"), "pass-app")
			return jsonResponse(http.StatusOK, `{"result":0,"toUrl":"`+base+`/redirect"}`, &http.Cookie{Name: "SSON", Value: "sson-cookie"}), nil
		case "/redirect":
			return jsonResponse(http.StatusOK, ``), nil
		case "/getSessionForPC.action":
			return jsonResponse(http.StatusOK, `{"sessionKey":"new-key","sessionSecret":"new-secret","accessToken":"token-1","keepAlive":60}`), nil
		default:
			return jsonResponse(http.StatusNotFound, `{"error":"not found"}`), nil
		}
	})
	client := httpclient.NewClient(httpclient.WithHTTPClient(&http.Client{Transport: transport}))
	login := NewLoginClient(client, WithLoginEndpoints(serverEndpoints(base)))

	session, err := login.AppLogin(context.Background(), Credentials{Username: "user-app", Password: "pass-app"})
	if err != nil {
		t.Fatalf("App 登录失败: %v", err)
	}
	if session.SessionKey != "new-key" || session.SessionSecret != "new-secret" {
		t.Fatalf("会话字段解析错误: %+v", session)
	}
	if session.AccessToken != "token-1" {
		t.Fatalf("accessToken 未写入: %+v", session)
	}
	if session.SSON != "sson-cookie" {
		t.Fatalf("SSON 未抓取: %+v", session)
	}
}

func TestWebLoginFlow(t *testing.T) {
	pubKey, privKey := generateRSAKey(t)
	base := "https://mock.local"
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/api/portal/loginUrl.action":
			return redirectResponse(base + "/page?reqId=web-req&lt=web-lt&appId=web-app"), nil
		case "/page":
			return jsonResponse(http.StatusOK, ``), nil
		case "/api/logbox/oauth2/appConf.do":
			return jsonResponse(http.StatusOK, `{"data":{"accountType":"01","appKey":"web-app","clientType":10020,"mailSuffix":"","isOauth2":true,"paramId":"pid"}}`), nil
		case "/api/logbox/config/encryptConf.do":
			return jsonResponse(http.StatusOK, `{"result":0,"data":{"pre":"pre-","pubKey":"`+pubKey+`"}}`), nil
		case "/api/logbox/oauth2/loginSubmit.do":
			if err := r.ParseForm(); err != nil {
				t.Fatalf("解析表单失败: %v", err)
			}
			checkEncrypted(t, privKey, strings.TrimPrefix(r.Form.Get("userName"), "pre-"), "user-web")
			checkEncrypted(t, privKey, strings.TrimPrefix(r.Form.Get("epd"), "pre-"), "pass-web")
			return jsonResponse(http.StatusOK, `{"result":0,"toUrl":"`+base+`/web_redirect"}`, &http.Cookie{Name: "SSON", Value: "web-sson"}), nil
		case "/web_redirect":
			return jsonResponse(http.StatusOK, ``, &http.Cookie{Name: "COOKIE_LOGIN_USER", Value: "cookie-web"}), nil
		default:
			return jsonResponse(http.StatusNotFound, `{"error":"not found"}`), nil
		}
	})
	client := httpclient.NewClient(httpclient.WithHTTPClient(&http.Client{Transport: transport}))
	login := NewLoginClient(client, WithLoginEndpoints(serverEndpoints(base)))
	session, err := login.WebLogin(context.Background(), Credentials{Username: "user-web", Password: "pass-web"})
	if err != nil {
		t.Fatalf("Web 登录失败: %v", err)
	}
	if session.CookieLoginUser != "cookie-web" {
		t.Fatalf("COOKIE_LOGIN_USER 未写入: %+v", session)
	}
	if session.SSON == "" {
		t.Fatalf("SSON 未写入: %+v", session)
	}
}

func TestAppRefresherRefreshWithAccessToken(t *testing.T) {
	base := "https://mock.local"
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/getSessionForPC.action":
			if err := r.ParseForm(); err != nil {
				t.Fatalf("解析表单失败: %v", err)
			}
			if r.Form.Get("accessToken") != "token-refresh" {
				t.Fatalf("未携带 accessToken 刷新: %v", r.Form)
			}
			return jsonResponse(http.StatusOK, `{"sessionKey":"refreshed-key","sessionSecret":"refreshed-secret","accessToken":"token-refresh","keepAlive":30}`), nil
		default:
			return jsonResponse(http.StatusNotFound, `{"error":"not found"}`), nil
		}
	})
	client := httpclient.NewClient(httpclient.WithHTTPClient(&http.Client{Transport: transport}))
	login := NewLoginClient(client, WithLoginEndpoints(serverEndpoints(base)))
	store := &memoryStore{session: &Session{AccessToken: "token-refresh"}}
	refresher := NewAppRefresher(client, store, login, Credentials{}, WithAppRefreshURL(base+"/getSessionForPC.action"), WithAppNow(func() time.Time {
		return time.Unix(0, 0)
	}))

	if err := refresher.Refresh(context.Background()); err != nil {
		t.Fatalf("刷新失败: %v", err)
	}
	if store.session.SessionKey != "refreshed-key" || store.session.SessionSecret != "refreshed-secret" {
		t.Fatalf("刷新结果不正确: %+v", store.session)
	}
	if store.session.AccessToken != "token-refresh" {
		t.Fatalf("AccessToken 应保留: %+v", store.session)
	}
	if store.session.ExpiresAt.IsZero() {
		t.Fatalf("应写入过期时间: %+v", store.session)
	}
}

func TestAppRefresherFallbackToLogin(t *testing.T) {
	pubKey, privKey := generateRSAKey(t)
	base := "https://mock.local"
	refreshShouldFail := true
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/unifyLoginForPC.action":
			return redirectResponse(base + "/page?reqId=req-1&lt=lt-1&appId=appid-1"), nil
		case "/page":
			return jsonResponse(http.StatusOK, ``), nil
		case "/api/logbox/oauth2/appConf.do":
			return jsonResponse(http.StatusOK, `{"data":{"accountType":"01","appKey":"9317140619","clientType":10020,"mailSuffix":"","isOauth2":true,"paramId":"pid"}}`), nil
		case "/api/logbox/config/encryptConf.do":
			return jsonResponse(http.StatusOK, `{"result":0,"data":{"pre":"pre-","pubKey":"`+pubKey+`"}}`), nil
		case "/api/logbox/oauth2/loginSubmit.do":
			if err := r.ParseForm(); err != nil {
				t.Fatalf("解析表单失败: %v", err)
			}
			checkEncrypted(t, privKey, strings.TrimPrefix(r.Form.Get("userName"), "pre-"), "user-app")
			checkEncrypted(t, privKey, strings.TrimPrefix(r.Form.Get("epd"), "pre-"), "pass-app")
			return jsonResponse(http.StatusOK, `{"result":0,"toUrl":"`+base+`/redirect"}`, &http.Cookie{Name: "SSON", Value: "sson-login"}), nil
		case "/redirect":
			return jsonResponse(http.StatusOK, ``), nil
		case "/getSessionForPC.action":
			if err := r.ParseForm(); err != nil {
				t.Fatalf("解析表单失败: %v", err)
			}
			if r.Form.Get("accessToken") != "" {
				if refreshShouldFail {
					return jsonResponse(http.StatusInternalServerError, `{"code":500}`), nil
				}
				return jsonResponse(http.StatusOK, `{"sessionKey":"ref-key","sessionSecret":"ref-secret","accessToken":"token-refresh","keepAlive":20}`), nil
			}
			return jsonResponse(http.StatusOK, `{"sessionKey":"login-key","sessionSecret":"login-secret","accessToken":"token-login"}`), nil
		default:
			return jsonResponse(http.StatusNotFound, `{"error":"not found"}`), nil
		}
	})
	client := httpclient.NewClient(httpclient.WithHTTPClient(&http.Client{Transport: transport}))
	login := NewLoginClient(client, WithLoginEndpoints(serverEndpoints(base)))
	store := &memoryStore{session: &Session{AccessToken: "token-refresh"}}
	refresher := NewAppRefresher(client, store, login, Credentials{Username: "user-app", Password: "pass-app"},
		WithAppRefreshURL(base+"/getSessionForPC.action"))

	if err := refresher.Refresh(context.Background()); err != nil {
		t.Fatalf("回退登录失败: %v", err)
	}
	if store.session.SessionKey != "login-key" || store.session.SessionSecret != "login-secret" {
		t.Fatalf("未使用登录结果更新会话: %+v", store.session)
	}
	if store.session.AccessToken != "token-login" {
		t.Fatalf("AccessToken 应来自登录结果: %+v", store.session)
	}
	if store.session.SSON != "sson-login" {
		t.Fatalf("SSON 应来自登录流程: %+v", store.session)
	}
}

func TestWebRefresherRefreshAndFallback(t *testing.T) {
	pubKey, privKey := generateRSAKey(t)
	base := "https://mock.local"
	refreshWithCookie := true
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/api/portal/loginUrl.action":
			if refreshWithCookie {
				return jsonResponse(http.StatusOK, ``, &http.Cookie{Name: "COOKIE_LOGIN_USER", Value: "refreshed-cookie"}, &http.Cookie{Name: "SSON", Value: "ref-sson"}), nil
			}
			return redirectResponse(base + "/page?reqId=req-1&lt=lt-1&appId=appid-1"), nil
		case "/page":
			return jsonResponse(http.StatusOK, ``), nil
		case "/api/logbox/oauth2/appConf.do":
			return jsonResponse(http.StatusOK, `{"data":{"accountType":"01","appKey":"9317140619","clientType":10020,"mailSuffix":"","isOauth2":true,"paramId":"pid"}}`), nil
		case "/api/logbox/config/encryptConf.do":
			return jsonResponse(http.StatusOK, `{"result":0,"data":{"pre":"pre-","pubKey":"`+pubKey+`"}}`), nil
		case "/api/logbox/oauth2/loginSubmit.do":
			if err := r.ParseForm(); err != nil {
				t.Fatalf("解析表单失败: %v", err)
			}
			checkEncrypted(t, privKey, strings.TrimPrefix(r.Form.Get("userName"), "pre-"), "user-web")
			checkEncrypted(t, privKey, strings.TrimPrefix(r.Form.Get("epd"), "pre-"), "pass-web")
			return jsonResponse(http.StatusOK, `{"result":0,"toUrl":"`+base+`/web_redirect"}`, &http.Cookie{Name: "SSON", Value: "fallback-sson"}), nil
		case "/web_redirect":
			return jsonResponse(http.StatusOK, ``, &http.Cookie{Name: "COOKIE_LOGIN_USER", Value: "fallback-cookie"}), nil
		default:
			return jsonResponse(http.StatusNotFound, `{"error":"not found"}`), nil
		}
	})
	client := httpclient.NewClient(httpclient.WithHTTPClient(&http.Client{Transport: transport}))
	login := NewLoginClient(client, WithLoginEndpoints(serverEndpoints(base)))
	store := &memoryStore{session: &Session{}}
	refresher := NewWebRefresher(client, store, login, Credentials{Username: "user-web", Password: "pass-web"},
		WithWebLoginURL(base+"/api/portal/loginUrl.action"))

	// 先走 Cookie 刷新
	if err := refresher.Refresh(context.Background()); err != nil {
		t.Fatalf("Cookie 刷新失败: %v", err)
	}
	if store.session.CookieLoginUser != "refreshed-cookie" || store.session.SSON != "ref-sson" {
		t.Fatalf("Cookie 刷新结果不正确: %+v", store.session)
	}

	// 触发回退登录
	refreshWithCookie = false
	if err := refresher.Refresh(context.Background()); err != nil {
		t.Fatalf("回退登录失败: %v", err)
	}
	if store.session.CookieLoginUser != "fallback-cookie" {
		t.Fatalf("回退未写入 COOKIE_LOGIN_USER: %+v", store.session)
	}
	if store.session.SSON == "" {
		t.Fatalf("回退未写入 SSON: %+v", store.session)
	}
}

func generateRSAKey(t *testing.T) (string, *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatalf("生成 RSA 密钥失败: %v", err)
	}
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatalf("序列化公钥失败: %v", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})
	body := string(pemBytes)
	body = strings.ReplaceAll(body, "-----BEGIN PUBLIC KEY-----", "")
	body = strings.ReplaceAll(body, "-----END PUBLIC KEY-----", "")
	body = strings.ReplaceAll(body, "\n", "")
	return body, key
}

func checkEncrypted(t *testing.T, priv *rsa.PrivateKey, hexData, expect string) {
	t.Helper()
	raw, err := hex.DecodeString(hexData)
	if err != nil {
		t.Fatalf("解析十六进制失败: %v", err)
	}
	plain, err := rsa.DecryptPKCS1v15(rand.Reader, priv, raw)
	if err != nil {
		t.Fatalf("解密失败: %v", err)
	}
	if string(plain) != expect {
		t.Fatalf("明文不匹配，得到 %s，期望 %s", string(plain), expect)
	}
}

func serverEndpoints(base string) LoginEndpoints {
	return LoginEndpoints{
		AppLoginURL:    base + "/unifyLoginForPC.action",
		WebLoginURL:    base + "/api/portal/loginUrl.action",
		AppConfURL:     base + "/api/logbox/oauth2/appConf.do",
		EncryptConfURL: base + "/api/logbox/config/encryptConf.do",
		LoginSubmitURL: base + "/api/logbox/oauth2/loginSubmit.do",
		SessionURL:     base + "/getSessionForPC.action",
	}
}

func jsonResponse(status int, body string, cookies ...*http.Cookie) *http.Response {
	rec := httptest.NewRecorder()
	for _, c := range cookies {
		rec.Header().Add("Set-Cookie", c.String())
	}
	rec.Header().Set("Content-Type", "application/json")
	rec.WriteHeader(status)
	rec.Body.WriteString(body)
	return rec.Result()
}

func redirectResponse(location string) *http.Response {
	rec := httptest.NewRecorder()
	rec.Header().Set("Location", location)
	rec.WriteHeader(http.StatusFound)
	return rec.Result()
}
