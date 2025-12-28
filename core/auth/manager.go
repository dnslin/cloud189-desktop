package auth

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
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
	Store       store.SessionStore[*Session]
	Refresher   Refresher
}

// AuthManager 负责多账号的会话管理与自动刷新。
type AuthManager struct {
	mu              sync.RWMutex
	accounts        map[string]*AccountSession
	sessionVersions map[string]*atomic.Int64
	current         string
	now             func() time.Time
}

// NewAuthManager 创建 AuthManager。
func NewAuthManager() *AuthManager {
	return &AuthManager{
		accounts:        make(map[string]*AccountSession),
		sessionVersions: make(map[string]*atomic.Int64),
		now:             time.Now,
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
	if m.sessionVersions == nil {
		m.sessionVersions = make(map[string]*atomic.Int64)
	}
	cp := session
	cp.AccountID = accountID
	m.accounts[accountID] = &cp
	if _, ok := m.sessionVersions[accountID]; !ok {
		m.sessionVersions[accountID] = &atomic.Int64{}
	}
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
	if m.sessionVersions != nil {
		delete(m.sessionVersions, accountID)
	}
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
	accID, acc, err := m.resolveAccount(accountID)
	if err != nil {
		return nil, err
	}
	session, err := m.ensureSession(ctx, accID, acc)
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
	m.bumpSessionVersion(accID)
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
	return &storeProvider{manager: m, accountID: accID, cachedVersion: -1}, nil
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

func (m *AuthManager) ensureSession(ctx context.Context, accountID string, acc *AccountSession) (*Session, error) {
	if acc.Store == nil {
		return nil, ErrSessionStoreNil
	}
	session, err := loadSession(acc.Store)
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
		m.bumpSessionVersion(accountID)
		session, err = loadSession(acc.Store)
		if err != nil {
			return nil, err
		}
	}
	if session == nil {
		return nil, ErrSessionNotFound
	}
	return session, nil
}

func (m *AuthManager) getSessionVersion(accountID string) int64 {
	if m == nil {
		return 0
	}
	m.mu.RLock()
	version := m.sessionVersions[accountID]
	m.mu.RUnlock()
	if version == nil {
		return 0
	}
	return version.Load()
}

func (m *AuthManager) bumpSessionVersion(accountID string) int64 {
	if m == nil {
		return 0
	}
	m.mu.RLock()
	version := m.sessionVersions[accountID]
	m.mu.RUnlock()
	if version == nil {
		m.mu.Lock()
		if m.sessionVersions == nil {
			m.sessionVersions = make(map[string]*atomic.Int64)
		}
		version = m.sessionVersions[accountID]
		if version == nil {
			version = &atomic.Int64{}
			m.sessionVersions[accountID] = version
		}
		m.mu.Unlock()
	}
	return version.Add(1)
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
	return loadSession(acc.Store)
}

func (m *AuthManager) saveSnapshot(accountID string, session *Session) (int64, error) {
	m.mu.RLock()
	acc := m.accounts[accountID]
	m.mu.RUnlock()
	if acc == nil {
		return 0, ErrAccountNotFound
	}
	if acc.Store == nil {
		return 0, ErrSessionStoreNil
	}
	if err := acc.Store.SaveSession(session); err != nil {
		return 0, err
	}
	return m.bumpSessionVersion(accountID), nil
}

type storeProvider struct {
	manager       *AuthManager
	accountID     string
	cached        *Session
	cachedVersion int64
	cacheMu       sync.RWMutex
}

func (p *storeProvider) session() *Session {
	if p == nil {
		return nil
	}
	currentVersion := int64(0)
	if p.manager != nil {
		currentVersion = p.manager.getSessionVersion(p.accountID)
	}
	p.cacheMu.RLock()
	if p.cachedVersion == currentVersion {
		cached := p.cached
		p.cacheMu.RUnlock()
		return cached
	}
	p.cacheMu.RUnlock()
	if p.manager == nil {
		return nil
	}
	cached, _ := p.manager.snapshot(p.accountID)
	p.cacheMu.Lock()
	p.cached = cached
	p.cachedVersion = currentVersion
	p.cacheMu.Unlock()
	return cached
}

func (p *storeProvider) save(session *Session) error {
	if p == nil || p.manager == nil {
		return coreerrors.Wrap(coreerrors.ErrCodeInvalidConfig, "auth: 会话存储未初始化", ErrSessionStoreNil)
	}
	version, err := p.manager.saveSnapshot(p.accountID, session)
	if err != nil {
		return err
	}
	p.cacheMu.Lock()
	p.cached = session
	p.cachedVersion = version
	p.cacheMu.Unlock()
	return nil
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
