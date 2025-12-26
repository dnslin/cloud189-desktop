package auth

import (
	coreerrors "github.com/dnslin/cloud189-desktop/core/errors"
	"github.com/dnslin/cloud189-desktop/core/store"
)

var (
	// ErrSessionNotFound 用于标记存储中不存在会话。
	ErrSessionNotFound = coreerrors.New(coreerrors.ErrCodeNotFound, "auth: 未找到会话")
	// ErrSessionStoreNil 在未注入存储时返回。
	ErrSessionStoreNil = coreerrors.New(coreerrors.ErrCodeInvalidConfig, "auth: SessionStore 未设置")
)

// loadSession 将存储中的会话转换为 auth.Session。
func loadSession(store store.SessionStore) (*Session, error) {
	if store == nil {
		return nil, ErrSessionStoreNil
	}
	raw, err := store.LoadSession()
	if err != nil {
		return nil, err
	}
	return castSession(raw)
}

func castSession(raw any) (*Session, error) {
	if raw == nil {
		return nil, nil
	}
	session, ok := raw.(*Session)
	if !ok {
		return nil, coreerrors.New(coreerrors.ErrCodeInvalidState, "auth: SessionStore 返回非 Session 类型")
	}
	return session, nil
}
