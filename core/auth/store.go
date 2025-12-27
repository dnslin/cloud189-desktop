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
func loadSession(store store.SessionStore[*Session]) (*Session, error) {
	if store == nil {
		return nil, ErrSessionStoreNil
	}
	return store.LoadSession()
}
