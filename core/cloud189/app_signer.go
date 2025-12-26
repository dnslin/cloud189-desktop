package cloud189

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gowsp/cloud189-desktop/core/auth"
	"github.com/gowsp/cloud189-desktop/core/crypto"
	"github.com/gowsp/cloud189-desktop/core/httpclient"
)

// AppSigner 负责 App 端签名，复刻官方 HMAC-SHA1 逻辑。
type AppSigner struct {
	session   auth.SessionProvider
	now       func() time.Time
	requestID func() string
}

// AppSignerOption 自定义签名器行为。
type AppSignerOption func(*AppSigner)

// WithAppSignerNow 替换时间来源，便于测试。
func WithAppSignerNow(now func() time.Time) AppSignerOption {
	return func(s *AppSigner) {
		s.now = now
	}
}

// WithAppSignerRequestID 替换请求 ID 生成逻辑。
func WithAppSignerRequestID(fn func() string) AppSignerOption {
	return func(s *AppSigner) {
		s.requestID = fn
	}
}

// NewAppSigner 创建 App 签名器。
func NewAppSigner(session auth.SessionProvider, opts ...AppSignerOption) *AppSigner {
	signer := &AppSigner{
		session:   session,
		now:       time.Now,
		requestID: crypto.UUID,
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
	return signer
}

// Middleware 返回 httpclient 可用的中间件。
func (s *AppSigner) Middleware() httpclient.Middleware {
	return func(req *http.Request) error {
		if s == nil {
			return errors.New("cloud189: App 签名器未初始化")
		}
		if s.session == nil {
			return errors.New("cloud189: SessionProvider 未设置")
		}
		sessionKey := s.session.GetSessionKey()
		sessionSecret := s.session.GetSessionSecret()
		if sessionKey == "" || sessionSecret == "" {
			return errors.New("cloud189: 会话密钥缺失")
		}

		now := s.now()
		q := req.URL.Query()
		q.Set("rand", strconv.FormatInt(now.UnixMilli(), 10))
		q.Set("clientType", AppClientType)
		q.Set("version", AppVersion)
		q.Set("channelId", AppChannelID)
		req.URL.RawQuery = q.Encode()

		date := now.Format(time.RFC1123)
		signStr := fmt.Sprintf("SessionKey=%s&Operate=%s&RequestURI=%s&Date=%s",
			sessionKey, strings.ToUpper(req.Method), req.URL.Path, date)
		if strings.EqualFold(req.URL.Host, UploadHost) {
			if val := q.Get("params"); val != "" {
				signStr += "&params=" + val
			}
		}
		signature := crypto.Sign(signStr, sessionSecret)

		req.Header.Set("Date", date)
		req.Header.Set("SessionKey", sessionKey)
		req.Header.Set("Signature", signature)
		req.Header.Set("user-agent", UserAgent)
		req.Header.Set("X-Request-ID", s.requestID())
		return nil
	}
}
