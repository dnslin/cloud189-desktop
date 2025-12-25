package cloud189

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gowsp/cloud189-desktop/core/auth"
	"github.com/gowsp/cloud189-desktop/core/crypto"
	"github.com/gowsp/cloud189-desktop/core/httpclient"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := f(req)
	if resp != nil && resp.Request == nil {
		resp.Request = req
	}
	return resp, err
}

func TestAppSignerMiddleware(t *testing.T) {
	fixed := time.Unix(0, 0)
	session := &auth.Session{SessionKey: "app-key", SessionSecret: "app-secret"}
	signer := NewAppSigner(session, WithAppSignerNow(func() time.Time { return fixed }), WithAppSignerRequestID(func() string { return "req-id" }))

	req, _ := http.NewRequest(http.MethodPost, "https://api.cloud.189.cn/some", nil)
	q := req.URL.Query()
	q.Set("foo", "bar")
	req.URL.RawQuery = q.Encode()

	if err := signer.Middleware()(req); err != nil {
		t.Fatalf("签名失败: %v", err)
	}

	got := req.URL.Query()
	if got.Get("rand") != "0" {
		t.Fatalf("rand 参数错误: %s", got.Get("rand"))
	}
	if got.Get("clientType") != AppClientType || got.Get("version") != AppVersion || got.Get("channelId") != AppChannelID {
		t.Fatalf("客户端参数缺失: %v", got)
	}
	date := fixed.Format(time.RFC1123)
	expectSign := crypto.Sign("SessionKey=app-key&Operate=POST&RequestURI=/some&Date="+date, "app-secret")
	if req.Header.Get("Signature") != expectSign {
		t.Fatalf("签名不匹配，得到 %s，期望 %s", req.Header.Get("Signature"), expectSign)
	}
	if req.Header.Get("X-Request-ID") != "req-id" {
		t.Fatalf("X-Request-ID 缺失")
	}
	if req.Header.Get("SessionKey") != "app-key" {
		t.Fatalf("SessionKey 头缺失")
	}
	if req.Header.Get("user-agent") != UserAgent {
		t.Fatalf("UA 未设置")
	}
}

func TestAppSignerUploadParams(t *testing.T) {
	fixed := time.Unix(0, 0)
	session := &auth.Session{SessionKey: "app-key", SessionSecret: "app-secret"}
	signer := NewAppSigner(session, WithAppSignerNow(func() time.Time { return fixed }), WithAppSignerRequestID(func() string { return "req-id" }))

	req, _ := http.NewRequest(http.MethodGet, "https://upload.cloud.189.cn/upload", nil)
	q := req.URL.Query()
	q.Set("params", "abc")
	req.URL.RawQuery = q.Encode()

	if err := signer.Middleware()(req); err != nil {
		t.Fatalf("签名失败: %v", err)
	}
	date := fixed.Format(time.RFC1123)
	expect := crypto.Sign("SessionKey=app-key&Operate=GET&RequestURI=/upload&Date="+date+"&params=abc", "app-secret")
	if req.Header.Get("Signature") != expect {
		t.Fatalf("上传签名不匹配，得到 %s，期望 %s", req.Header.Get("Signature"), expect)
	}
}

func TestWebSignerSign(t *testing.T) {
	pub, priv := generateRSAPair(t)
	session := &auth.Session{SessionKey: "web-key"}
	signer := NewWebSigner(session,
		WithWebSignerKeyGen(func() string { return "0123456789abcdef" }),
		WithWebSignerNow(func() time.Time { return time.UnixMilli(1234) }),
		WithWebSignerRequestID(func() string { return "req-1" }),
	)

	params := url.Values{}
	params.Set("foo", "bar")
	params.Set("hello", "world")

	req, _ := http.NewRequest(http.MethodGet, "https://upload.cloud.189.cn/web", nil)
	rsaKey := &WebRSA{PkId: "pk-1", PubKey: pub}
	if err := signer.Sign(req, params, rsaKey); err != nil {
		t.Fatalf("Web 签名失败: %v", err)
	}

	secret := decryptSecret(t, priv, req.Header.Get("EncryptionText"))
	if secret != "0123456789abcdef" {
		t.Fatalf("随机密钥不匹配: %s", secret)
	}

	cipherParams := req.URL.Query().Get("params")
	raw, err := hex.DecodeString(cipherParams)
	if err != nil {
		t.Fatalf("params 解析失败: %v", err)
	}
	plain, err := crypto.DecryptECB([]byte(secret[:16]), raw)
	if err != nil {
		t.Fatalf("params 解密失败: %v", err)
	}
	parsed := parseKV(string(plain))
	for k, v := range map[string]string{"foo": "bar", "hello": "world"} {
		if parsed[k] != v {
			t.Fatalf("参数不匹配 %s: %s", k, parsed[k])
		}
	}

	if req.Header.Get("SessionKey") != "web-key" {
		t.Fatalf("SessionKey 头缺失")
	}
	if req.Header.Get("PkId") != "pk-1" {
		t.Fatalf("PkId 头缺失")
	}
	// 验证签名符合任意排列的参数编码
	fields := map[string]string{
		"SessionKey": req.Header.Get("SessionKey"),
		"Operate":    req.Method,
		"RequestURI": req.URL.Path,
		"Date":       req.Header.Get("X-Request-Date"),
		"params":     req.URL.Query().Get("params"),
	}
	if !matchSignature(req.Header.Get("Signature"), secret, fields) {
		t.Fatalf("Signature 校验失败: %s", req.Header.Get("Signature"))
	}
}

func TestAppUploadEncryptsParams(t *testing.T) {
	secret := "1234567890abcdefX"
	session := &auth.Session{SessionKey: "app-key", SessionSecret: secret}
	handler := func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "upload.cloud.189.cn" {
			t.Fatalf("上传地址错误: %s", r.URL.Host)
		}
		val := r.URL.Query().Get("params")
		raw, err := hex.DecodeString(val)
		if err != nil {
			t.Fatalf("params 十六进制解析失败: %v", err)
		}
		plain, err := crypto.DecryptECB([]byte(secret[:16]), raw)
		if err != nil {
			t.Fatalf("params 解密失败: %v", err)
		}
		parsed := parseKV(string(plain))
		if parsed["fileName"] != "demo.txt" {
			t.Fatalf("参数解密错误: %+v", parsed)
		}
		return jsonResponse(http.StatusOK, `{"code":"SUCCESS"}`), nil
	}
	cli := httpclient.NewClient(httpclient.WithHTTPClient(&http.Client{Transport: roundTripFunc(handler)}))
	client := NewClient(session, WithHTTPClient(cli))

	form := url.Values{}
	form.Set("fileName", "demo.txt")
	var rsp CodeResponse
	if err := client.AppUpload(context.Background(), "/person/init", form, &rsp); err != nil {
		t.Fatalf("上传请求失败: %v", err)
	}
}

func TestAppGetBusinessError(t *testing.T) {
	session := &auth.Session{SessionKey: "app-key", SessionSecret: "secret"}
	handler := func(r *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, `{"code":"InvalidSessionKey","msg":"expired"}`), nil
	}
	cli := httpclient.NewClient(httpclient.WithHTTPClient(&http.Client{Transport: roundTripFunc(handler)}))
	client := NewClient(session, WithHTTPClient(cli))

	var rsp CodeResponse
	err := client.AppGet(context.Background(), "/demo", nil, &rsp)
	if err == nil {
		t.Fatalf("预期出现业务错误")
	}
	var ec *httpclient.ErrCode
	if !errors.As(err, &ec) {
		t.Fatalf("应返回 ErrCode，得到 %T", err)
	}
	if ec.Code != "InvalidSessionKey" {
		t.Fatalf("错误码不匹配: %+v", ec)
	}
}

// 解析 k=v&k2=v2 字符串。
func parseKV(s string) map[string]string {
	res := make(map[string]string)
	parts := strings.Split(s, "&")
	for _, p := range parts {
		if p == "" {
			continue
		}
		kv := strings.SplitN(p, "=", 2)
		if len(kv) == 1 {
			res[kv[0]] = ""
			continue
		}
		res[kv[0]] = kv[1]
	}
	return res
}

func matchSignature(sig, secret string, fields map[string]string) bool {
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	return permute(keys, func(order []string) bool {
		var buf strings.Builder
		for i, k := range order {
			if i > 0 {
				buf.WriteByte('&')
			}
			buf.WriteString(k)
			buf.WriteByte('=')
			buf.WriteString(fields[k])
		}
		expect := crypto.Sign(buf.String(), secret)
		return expect == sig
	})
}

func permute(keys []string, cb func([]string) bool) bool {
	var dfs func(int)
	arr := append([]string{}, keys...)
	matched := false
	dfs = func(i int) {
		if matched {
			return
		}
		if i == len(arr) {
			if cb(arr) {
				matched = true
			}
			return
		}
		for j := i; j < len(arr); j++ {
			arr[i], arr[j] = arr[j], arr[i]
			dfs(i + 1)
			arr[i], arr[j] = arr[j], arr[i]
			if matched {
				return
			}
		}
	}
	dfs(0)
	return matched
}

func generateRSAPair(t *testing.T) (string, *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatalf("生成 RSA 失败: %v", err)
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

func decryptSecret(t *testing.T, priv *rsa.PrivateKey, enc string) string {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		t.Fatalf("解码加密密钥失败: %v", err)
	}
	plain, err := rsa.DecryptPKCS1v15(rand.Reader, priv, data)
	if err != nil {
		t.Fatalf("解密密钥失败: %v", err)
	}
	return string(plain)
}

func jsonResponse(status int, body string) *http.Response {
	rec := httptest.NewRecorder()
	rec.Header().Set("Content-Type", "application/json")
	rec.WriteHeader(status)
	rec.Body.WriteString(body)
	return rec.Result()
}
