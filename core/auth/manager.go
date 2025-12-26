package auth

import (
	"context"
	"errors"
	"sync"
	"time"

	coreerrors "github.com/dnslin/cloud189-desktop/core/errors"
	"github.com/dnslin/cloud189-desktop/core/store"
)

var (
	// ErrAccountNotFound 在账号不存在或未选择时返回。
	ErrAccountNotFound = coreerrors.New(coreerrors.ErrCodeNotFound, "auth: 未找到账号")
	// ErrAccountIDEmpty 在新增账号时未提供 ID 返回。
	ErrAccountIDEmpty = coreerrors.New(coreerrors.ErrCodeInvalidArgument, "auth: 账号 ID 不能为空")
	// ErrRefresherNil 需要刷新但未配置刷新器时返回。
	ErrRefresherNil = coreerrors.New(coreerrors.ErrCodeInvalidConfig, "auth: 未配置刷新器")
)

// AccountSession 记录账号关联的会话存储、刷新器与元信息。
type AccountSession struct {
	AccountID   string
	DisplayName string
	Store       store.SessionStore[Session]
	Refresher   Refresher
}

// AuthManager 负责多账号的会话管理与自动刷新。
type AuthManager struct {
	mu       sync.RWMutex
	accounts map[string]*AccountSession
	current  string
	now      func() time.Time
}

// NewAuthManager 创建 AuthManager。
func NewAuthManager() *AuthManager {
	return &AuthManager{
		accounts: make(map[string]*AccountSession),
		now:      time.Now,
	}
}

// AddAccount 注册一个账号，会更新默认当前账号。
func (m *AuthManager) AddAccount(accountID string, session AccountSession) error {
	if accountID == "" {
		return ErrAccountIDEmpty
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.accounts == nil {
		m.accounts = make(map[string]*AccountSession)
	}
	cp := session
	cp.AccountID = accountID
	m.accounts[accountID] = &cp
	if m.current == "" {
		m.current = accountID
	}
	return nil
}

// RemoveAccount 删除账号，若为当前账号则一并清空 current。
func (m *AuthManager) RemoveAccount(accountID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.accounts, accountID)
	if m.current == accountID {
		m.current = ""
	}
}

// SetCurrentAccount 切换当前账号。
func (m *AuthManager) SetCurrentAccount(accountID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.accounts[accountID]; !ok {
		return ErrAccountNotFound
	}
	m.current = accountID
	return nil
}

// ListAccounts 返回账号列表（浅拷贝），包含元信息与当前标记。
func (m *AuthManager) ListAccounts() []AccountSession {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]AccountSession, 0, len(m.accounts))
	for id, acc := range m.accounts {
		item := *acc
		item.AccountID = id
		result = append(result, item)
	}
	return result
}

// GetAccount 返回指定账号（或当前账号）的有效 Session，必要时自动刷新。
func (m *AuthManager) GetAccount(ctx context.Context, accountID string) (*Session, error) {
	_, acc, err := m.resolveAccount(accountID)
	if err != nil {
		return nil, err
	}
	session, err := m.ensureSession(ctx, acc)
	if err != nil {
		return nil, err
	}
	return session.Clone(), nil
}

// RefreshAccount 主动触发账号刷新。
func (m *AuthManager) RefreshAccount(ctx context.Context, accountID string) error {
	accID, acc, err := m.resolveAccount(accountID)
	if err != nil {
		return err
	}
	if acc.Refresher == nil {
		return ErrRefresherNil
	}
	if err := acc.Refresher.Refresh(ctx); err != nil {
		return err
	}
	_, err = m.snapshot(accID)
	return err
}

// SessionProvider 返回面向当前存储的 SessionProvider，便于签名器获取最新凭证。
func (m *AuthManager) SessionProvider(accountID string) (SessionProvider, error) {
	accID, acc, err := m.resolveAccount(accountID)
	if err != nil {
		return nil, err
	}
	if acc.Store == nil {
		return nil, ErrSessionStoreNil
	}
	return &storeProvider{manager: m, accountID: accID}, nil
}

func (m *AuthManager) resolveAccount(accountID string) (string, *AccountSession, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	id := accountID
	if id == "" {
		id = m.current
	}
	if id == "" {
		return "", nil, ErrAccountNotFound
	}
	acc := m.accounts[id]
	if acc == nil {
		return "", nil, ErrAccountNotFound
	}
	return id, acc, nil
}

func (m *AuthManager) ensureSession(ctx context.Context, acc *AccountSession) (*Session, error) {
	if acc.Store == nil {
		return nil, ErrSessionStoreNil
	}
	session, err := acc.Store.LoadSession()
	if err != nil && !errors.Is(err, ErrSessionNotFound) {
		return nil, err
	}
	needRefresh := session == nil || session.Expired(m.now())
	if acc.Refresher != nil && acc.Refresher.NeedsRefresh() {
		needRefresh = true
	}
	if needRefresh {
		if acc.Refresher == nil {
			return nil, ErrRefresherNil
		}
		if err := acc.Refresher.Refresh(ctx); err != nil {
			return nil, err
		}
		session, err = acc.Store.LoadSession()
		if err != nil {
			return nil, err
		}
	}
	if session == nil {
		return nil, ErrSessionNotFound
	}
	return session, nil
}

func (m *AuthManager) snapshot(accountID string) (*Session, error) {
	m.mu.RLock()
	acc := m.accounts[accountID]
	m.mu.RUnlock()
	if acc == nil {
		return nil, ErrAccountNotFound
	}
	if acc.Store == nil {
		return nil, ErrSessionStoreNil
	}
	return acc.Store.LoadSession()
}

func (m *AuthManager) saveSnapshot(accountID string, session *Session) error {
	m.mu.RLock()
	acc := m.accounts[accountID]
	m.mu.RUnlock()
	if acc == nil {
		return ErrAccountNotFound
	}
	if acc.Store == nil {
		return ErrSessionStoreNil
	}
	return acc.Store.SaveSession(session)
}

type storeProvider struct {
	manager   *AuthManager
	accountID string
}

func (p *storeProvider) session() *Session {
	if p == nil || p.manager == nil {
		return nil
	}
	session, err := p.manager.snapshot(p.accountID)
	if err != nil {
		return nil
	}
	return session
}

func (p *storeProvider) save(session *Session) error {
	if p == nil || p.manager == nil {
		return coreerrors.Wrap(coreerrors.ErrCodeInvalidConfig, "auth: 会话存储未初始化", ErrSessionStoreNil)
	}
	return p.manager.saveSnapshot(p.accountID, session)
}

func (p *storeProvider) GetSessionKey() string {
	if s := p.session(); s != nil {
		return s.SessionKey
	}
	return ""
}

func (p *storeProvider) GetSessionSecret() string {
	if s := p.session(); s != nil {
		return s.SessionSecret
	}
	return ""
}

func (p *storeProvider) GetAccessToken() string {
	if s := p.session(); s != nil {
		return s.AccessToken
	}
	return ""
}

func (p *storeProvider) GetSSSON() string {
	if s := p.session(); s != nil {
		return s.SSON
	}
	return ""
}

func (p *storeProvider) GetCookieLoginUser() string {
	if s := p.session(); s != nil {
		return s.CookieLoginUser
	}
	return ""
}

func (p *storeProvider) SetSessionKey(key string) error {
	session := p.session()
	if session == nil {
		session = &Session{}
	}
	if err := session.SetSessionKey(key); err != nil {
		return err
	}
	return p.save(session)
}
