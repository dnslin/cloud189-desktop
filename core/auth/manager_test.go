package auth

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/dnslin/cloud189-desktop/core/store"
)

// 内存实现的 SessionStore，便于测试。
type memorySessionStore struct {
	mu      sync.Mutex
	session *Session
}

func (s *memorySessionStore) SaveSession(session *Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if session == nil {
		s.session = nil
		return nil
	}
	s.session = session.Clone()
	return nil
}

func (s *memorySessionStore) LoadSession() (*Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.session == nil {
		return nil, ErrSessionNotFound
	}
	return s.session.Clone(), nil
}

func (s *memorySessionStore) ClearSession() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.session = nil
	return nil
}

type fakeRefresher struct {
	store         store.SessionStore[*Session]
	next          *Session
	err           error
	needsRefresh  bool
	refreshCalled int
}

func (r *fakeRefresher) Refresh(ctx context.Context) error {
	r.refreshCalled++
	if r.err != nil {
		return r.err
	}
	if r.store != nil && r.next != nil {
		return r.store.SaveSession(r.next)
	}
	return nil
}

func (r *fakeRefresher) NeedsRefresh() bool {
	return r.needsRefresh
}

// TestAuthManager_LoginWithRealAccount 使用真实账号验证登录与 Session 获取。
func TestAuthManager_LoginWithRealAccount(t *testing.T) {
	username := os.Getenv("CLOUD189_USERNAME")
	password := os.Getenv("CLOUD189_PASSWORD")
	if username == "" || password == "" {
		t.Skip("未配置 CLOUD189_USERNAME/CLOUD189_PASSWORD，跳过真实登录测试")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	store := &memorySessionStore{}
	manager := NewAuthManager()
	refresher := NewAppRefresher(nil, store, nil, Credentials{Username: username, Password: password})

	if err := manager.AddAccount("real", AccountSession{
		AccountID:   "real",
		DisplayName: "真实账号",
		Store:       store,
		Refresher:   refresher,
	}); err != nil {
		t.Fatalf("添加账号失败: %v", err)
	}

	session, err := manager.GetAccount(ctx, "real")
	if err != nil {
		t.Fatalf("真实登录失败: %v", err)
	}
	if session.SessionKey == "" || session.SessionSecret == "" {
		t.Fatalf("登录未返回有效会话，结果: %+v", session)
	}

	accounts := manager.ListAccounts()
	if len(accounts) != 1 || accounts[0].AccountID != "real" {
		t.Fatalf("账号列表异常，期望包含 real，实际: %+v", accounts)
	}
}

// TestAuthManager_MultiAccountSwitch 验证多账号添加与切换。
func TestAuthManager_MultiAccountSwitch(t *testing.T) {
	manager := NewAuthManager()

	store1 := &memorySessionStore{}
	_ = store1.SaveSession(&Session{SessionKey: "k1", SessionSecret: "s1"})
	if err := manager.AddAccount("acc1", AccountSession{
		AccountID:   "acc1",
		DisplayName: "账号1",
		Store:       store1,
		Refresher:   &fakeRefresher{store: store1},
	}); err != nil {
		t.Fatalf("添加第一个账号失败: %v", err)
	}

	store2 := &memorySessionStore{}
	_ = store2.SaveSession(&Session{SessionKey: "k2", SessionSecret: "s2"})
	if err := manager.AddAccount("acc2", AccountSession{
		AccountID:   "acc2",
		DisplayName: "账号2",
		Store:       store2,
		Refresher:   &fakeRefresher{store: store2},
	}); err != nil {
		t.Fatalf("添加第二个账号失败: %v", err)
	}

	accounts := manager.ListAccounts()
	if len(accounts) != 2 {
		t.Fatalf("应有两个账号，实际 %d", len(accounts))
	}

	if err := manager.SetCurrentAccount("acc2"); err != nil {
		t.Fatalf("切换当前账号失败: %v", err)
	}
	current, err := manager.GetAccount(context.Background(), "")
	if err != nil {
		t.Fatalf("获取当前账号失败: %v", err)
	}
	if current.SessionKey != "k2" {
		t.Fatalf("切换后应返回 acc2 会话，实际 SessionKey %s", current.SessionKey)
	}
}

// TestAuthManager_RefreshExpiredSession 过期会话应触发刷新并写回存储。
func TestAuthManager_RefreshExpiredSession(t *testing.T) {
	store := &memorySessionStore{}
	expired := &Session{
		SessionKey:    "old",
		SessionSecret: "old",
		ExpiresAt:     time.Now().Add(-time.Hour),
	}
	_ = store.SaveSession(expired)

	newSession := &Session{
		SessionKey:    "new",
		SessionSecret: "new",
		ExpiresAt:     time.Now().Add(time.Hour),
	}
	refresher := &fakeRefresher{
		store: store,
		next:  newSession,
	}

	manager := NewAuthManager()
	if err := manager.AddAccount("refresh", AccountSession{
		AccountID:   "refresh",
		DisplayName: "刷新账号",
		Store:       store,
		Refresher:   refresher,
	}); err != nil {
		t.Fatalf("添加账号失败: %v", err)
	}

	session, err := manager.GetAccount(context.Background(), "refresh")
	if err != nil {
		t.Fatalf("获取会话失败: %v", err)
	}
	if refresher.refreshCalled != 1 {
		t.Fatalf("过期会话应触发一次刷新，实际 %d 次", refresher.refreshCalled)
	}
	if session.SessionKey != "new" || session.SessionSecret != "new" {
		t.Fatalf("刷新后会话应更新为新值，实际: %+v", session)
	}
}
