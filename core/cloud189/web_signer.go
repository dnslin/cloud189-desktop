package cloud189

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/dnslin/cloud189-desktop/core/auth"
	"github.com/dnslin/cloud189-desktop/core/crypto"
	"github.com/dnslin/cloud189-desktop/core/httpclient"
)

// WebRSA 表示上传场景所需的公钥信息。
type WebRSA struct {
	ResCode    int    `json:"res_code,omitempty"`
	ResMessage string `json:"res_message,omitempty"`
	PkId       string `json:"pkId,omitempty"`
	PubKey     string `json:"pubKey,omitempty"`
	Expire     int64  `json:"expire,omitempty"`
}

func (r *WebRSA) Error() string {
	if r == nil {
		return ""
	}
	if r.ResMessage != "" {
		return r.ResMessage
	}
	if r.ResCode != 0 {
		return "rsa 获取失败"
	}
	return ""
}

// IsSuccess 实现 httpclient.OkRsp。
func (r *WebRSA) IsSuccess() bool {
	if r == nil {
		return true
	}
	return r.ResCode == 0
}

// Code 提供业务码给 ErrCode。
func (r *WebRSA) Code() string {
	if r == nil || r.ResCode == 0 {
		return ""
	}
	return strconv.Itoa(r.ResCode)
}

// Message 返回服务端消息。
func (r *WebRSA) Message() string {
	if r == nil {
		return ""
	}
	if r.ResMessage != "" {
		return r.ResMessage
	}
	return r.Error()
}

// WebSigner 负责 Web 端上传签名，复刻官方 AES+RSA 方案。
type WebSigner struct {
	session   auth.SessionProvider
	now       func() time.Time
	requestID func() string
	keyGen    func() (string, error)
}

// WebSignerOption 自定义签名器行为。
type WebSignerOption func(*WebSigner)

// WithWebSignerNow 替换时间来源，便于测试。
func WithWebSignerNow(now func() time.Time) WebSignerOption {
	return func(s *WebSigner) {
		s.now = now
	}
}

// WithWebSignerRequestID 替换请求 ID 生成逻辑。
func WithWebSignerRequestID(fn func() string) WebSignerOption {
	return func(s *WebSigner) {
		s.requestID = fn
	}
}

// WithWebSignerKeyGen 替换随机密钥生成逻辑。
func WithWebSignerKeyGen(fn func() (string, error)) WebSignerOption {
	return func(s *WebSigner) {
		s.keyGen = fn
	}
}

// NewWebSigner 创建 Web 签名器。
func NewWebSigner(session auth.SessionProvider, opts ...WebSignerOption) *WebSigner {
	signer := &WebSigner{
		session:   session,
		now:       time.Now,
		requestID: crypto.UUID,
		keyGen:    randomWebSecret,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(signer)
		}
	}
	if signer.now == nil {
		signer.now = time.Now
	}
	if signer.requestID == nil {
		signer.requestID = crypto.UUID
	}
	if signer.keyGen == nil {
		signer.keyGen = randomWebSecret
	}
	return signer
}

// Sign 填充上传参数、加密密钥与签名头。
func (s *WebSigner) Sign(req *http.Request, params url.Values, rsaKey *WebRSA) error {
	if s == nil {
		return errors.New("cloud189: Web 签名器未初始化")
	}
	if s.session == nil {
		return errors.New("cloud189: SessionProvider 未设置")
	}
	if rsaKey == nil || rsaKey.PkId == "" || rsaKey.PubKey == "" {
		return errors.New("cloud189: RSA 公钥缺失")
	}
	sessionKey := s.session.GetSessionKey()
	if sessionKey == "" {
		return errors.New("cloud189: 会话缺少 SessionKey")
	}

	secret, err := s.keyGen()
	if err != nil {
		return fmt.Errorf("cloud189: 生成加密密钥失败: %w", err)
	}
	if len(secret) < 16 {
		return errors.New("cloud189: 生成加密密钥失败")
	}
	aesKey := []byte(secret[:16])

	encodedParams := crypto.EncodeURLValues(params)
	encryptedParams, err := crypto.EncryptECB(aesKey, []byte(encodedParams))
	if err != nil {
		return err
	}
	hexParams := hex.EncodeToString(encryptedParams)

	q := req.URL.Query()
	q.Set("params", hexParams)
	req.URL.RawQuery = q.Encode()

	requestDate := strconv.FormatInt(s.now().UnixMilli(), 10)
	reqID := s.requestID()

	// 签名字符串必须按固定顺序拼接（与参考实现一致）
	signStr := fmt.Sprintf("SessionKey=%s&Operate=%s&RequestURI=%s&Date=%s&params=%s",
		sessionKey, strings.ToUpper(req.Method), req.URL.Path, requestDate, hexParams)
	signature := crypto.Sign(signStr, secret)

	pubKey := crypto.WrapRSAPubKey(rsaKey.PubKey)
	encryptedKey, err := crypto.Encrypt(pubKey, []byte(secret))
	if err != nil {
		return err
	}

	req.Header.Set("accept", "application/json;charset=UTF-8")
	req.Header.Set("SessionKey", sessionKey)
	req.Header.Set("Signature", signature)
	req.Header.Set("X-Request-Date", requestDate)
	req.Header.Set("X-Request-ID", reqID)
	req.Header.Set("EncryptionText", base64.StdEncoding.EncodeToString(encryptedKey))
	req.Header.Set("PkId", rsaKey.PkId)
	return nil
}

// randomWebSecret 复刻官方随机串生成方式。
func randomWebSecret() (string, error) {
	tmpl := []byte("xxxxxxxxxxxx4xxxyxxxxxxxxxxxxxxx")
	for i, b := range tmpl {
		switch b {
		case 'x':
			v, err := randomNibble()
			if err != nil {
				return "", err
			}
			tmpl[i] = hexChar(v)
		case 'y':
			v, err := randomNibble()
			if err != nil {
				return "", err
			}
			tmpl[i] = hexChar(v&0x3 | 0x8)
		}
	}
	secret := string(tmpl)
	// 16~31 位随机截断
	lenByte := make([]byte, 1)
	if _, err := rand.Read(lenByte); err != nil {
		return "", err
	}
	max := 16 + int(lenByte[0])%16
	if max > len(secret) {
		max = len(secret)
	}
	return secret[:max], nil
}

func randomNibble() (int, error) {
	b := make([]byte, 1)
	if _, err := rand.Read(b); err != nil {
		return 0, err
	}
	return int(b[0]) & 0x0f, nil
}

func hexChar(v int) byte {
	if v < 10 {
		return byte('0' + v)
	}
	return byte('a' + v - 10)
}

// WithWebCookies 为 Web API 补齐必需 Cookie。
func WithWebCookies(session auth.SessionProvider) httpclient.Middleware {
	return func(req *http.Request) error {
		if session == nil {
			return errors.New("cloud189: SessionProvider 未设置")
		}
		user := session.GetCookieLoginUser()
		sson := session.GetSSSON()
		if user == "" && sson == "" {
			return errors.New("cloud189: 缺少 Web Cookie")
		}
		if user != "" {
			req.AddCookie(&http.Cookie{Name: "COOKIE_LOGIN_USER", Value: user})
		}
		if sson != "" {
			req.AddCookie(&http.Cookie{Name: "SSON", Value: sson})
		}
		return nil
	}
}
