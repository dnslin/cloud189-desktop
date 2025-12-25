package auth

import "time"

// SessionProvider 提供签名、鉴权所需的会话字段。
type SessionProvider interface {
	GetSessionKey() string
	GetSessionSecret() string
	GetAccessToken() string
	GetSSSON() string
	GetCookieLoginUser() string
}

// Session 记录当前的会话凭证。
type Session struct {
	SessionKey      string    `json:"sessionKey,omitempty"`
	SessionSecret   string    `json:"sessionSecret,omitempty"`
	AccessToken     string    `json:"accessToken,omitempty"`
	SSON            string    `json:"sson,omitempty"`
	CookieLoginUser string    `json:"cookieLoginUser,omitempty"`
	ExpiresAt       time.Time `json:"expiresAt,omitempty"`
}

// GetSessionKey 实现 SessionProvider。
func (s *Session) GetSessionKey() string {
	if s == nil {
		return ""
	}
	return s.SessionKey
}

// SetSessionKey 设置 SessionKey（Web 上传时动态获取）。
func (s *Session) SetSessionKey(key string) {
	if s != nil {
		s.SessionKey = key
	}
}

// GetSessionSecret 实现 SessionProvider。
func (s *Session) GetSessionSecret() string {
	if s == nil {
		return ""
	}
	return s.SessionSecret
}

// GetAccessToken 实现 SessionProvider。
func (s *Session) GetAccessToken() string {
	if s == nil {
		return ""
	}
	return s.AccessToken
}

// GetSSSON 实现 SessionProvider。
func (s *Session) GetSSSON() string {
	if s == nil {
		return ""
	}
	return s.SSON
}

// GetCookieLoginUser 实现 SessionProvider。
func (s *Session) GetCookieLoginUser() string {
	if s == nil {
		return ""
	}
	return s.CookieLoginUser
}

// Expired 判断会话是否过期。
func (s *Session) Expired(now time.Time) bool {
	if s == nil {
		return true
	}
	if s.ExpiresAt.IsZero() {
		return false
	}
	return !now.Before(s.ExpiresAt)
}

// Clone 返回会话的浅拷贝，避免直接暴露内部指针。
func (s *Session) Clone() *Session {
	if s == nil {
		return nil
	}
	cp := *s
	return &cp
}
