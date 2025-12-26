package auth

import coreerrors "github.com/dnslin/cloud189-desktop/core/errors"

var (
	// ErrSessionNotFound 用于标记存储中不存在会话。
	ErrSessionNotFound = coreerrors.New(coreerrors.ErrCodeNotFound, "auth: 未找到会话")
	// ErrSessionStoreNil 在未注入存储时返回。
	ErrSessionStoreNil = coreerrors.New(coreerrors.ErrCodeInvalidConfig, "auth: SessionStore 未设置")
)
