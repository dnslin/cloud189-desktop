package auth

import "errors"

// SessionStore 抽象会话存储，具体实现由上层注入。
type SessionStore interface {
	SaveSession(session *Session) error
	LoadSession() (*Session, error)
	ClearSession() error
}

var (
	// ErrSessionNotFound 用于标记存储中不存在会话。
	ErrSessionNotFound = errors.New("auth: 未找到会话")
	// ErrSessionStoreNil 在未注入存储时返回。
	ErrSessionStoreNil = errors.New("auth: SessionStore 未设置")
)
