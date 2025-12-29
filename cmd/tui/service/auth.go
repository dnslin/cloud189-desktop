package service

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	tuistore "github.com/dnslin/cloud189-desktop/cmd/tui/store"
	"github.com/dnslin/cloud189-desktop/core/auth"
	"github.com/dnslin/cloud189-desktop/core/cloud189"
	"github.com/dnslin/cloud189-desktop/core/httpclient"
	"github.com/dnslin/cloud189-desktop/core/model"
)

const defaultAccountID = "default"

// AuthService 封装登录与会话管理。
type AuthService struct {
	manager      *auth.AuthManager
	loginClient  *auth.LoginClient
	httpClient   *httpclient.Client
	sessionStore *tuistore.JSONSessionStore
	accountID    string
	client       *cloud189.Client // API 客户端
}

// NewAuthService 组装 AuthService，dataDir 为空时使用默认会话路径。
func NewAuthService(manager *auth.AuthManager, loginClient *auth.LoginClient, httpClient *httpclient.Client, dataDir string) *AuthService {
	if httpClient == nil {
		httpClient = httpclient.NewClient()
	}
	if loginClient == nil {
		loginClient = auth.NewLoginClient(httpClient)
	}
	if manager == nil {
		manager = auth.NewAuthManager()
	}

	sessionPath := ""
	if dataDir != "" {
		sessionPath = filepath.Join(dataDir, "sessions.json")
	} else if path, err := tuistore.DefaultSessionFilePath(); err == nil {
		sessionPath = path
	}
	sessionStore := tuistore.NewJSONSessionStore(sessionPath)
	// 预先注册账号，便于直接读取已有会话。
	_ = manager.AddAccount(defaultAccountID, auth.AccountSession{Store: sessionStore})

	// 创建 API 客户端
	adapter := cloud189.NewAuthManagerAdapter(manager)
	client := cloud189.NewClient(adapter,
		cloud189.WithHTTPClient(httpClient),
		cloud189.WithLogger(httpClient.Logger),
	)

	return &AuthService{
		manager:      manager,
		loginClient:  loginClient,
		httpClient:   httpClient,
		sessionStore: sessionStore,
		accountID:    defaultAccountID,
		client:       client,
	}
}

// Login 执行账号密码登录，并更新会话存储与刷新器。
func (s *AuthService) Login(ctx context.Context, username, password string) (*auth.Session, error) {
	creds := auth.Credentials{
		Username: strings.TrimSpace(username),
		Password: password,
	}
	if creds.Username == "" || creds.Password == "" {
		return nil, auth.ErrMissingCredentials
	}
	session, err := s.loginClient.AppLogin(ctx, creds)
	if err != nil {
		return nil, err
	}
	if err := s.sessionStore.SaveSession(session); err != nil {
		return nil, err
	}

	refresher := auth.NewAppRefresher(s.httpClient, s.sessionStore, s.loginClient, creds, auth.WithAppLogger(s.httpClient.Logger))
	if err := s.manager.AddAccount(s.accountID, auth.AccountSession{
		DisplayName: creds.Username,
		Store:       s.sessionStore,
		Refresher:   refresher,
	}); err != nil {
		return nil, err
	}
	return session, nil
}

// Logout 清除当前会话并移除账号。
func (s *AuthService) Logout() error {
	if err := s.sessionStore.ClearSession(); err != nil {
		return err
	}
	if s.manager != nil {
		s.manager.RemoveAccount(s.accountID)
	}
	return nil
}

// GetCurrentSession 返回当前会话，缺失时返回 ErrSessionNotFound。
func (s *AuthService) GetCurrentSession(ctx context.Context) (*auth.Session, error) {
	if s.manager == nil {
		return nil, auth.ErrSessionNotFound
	}
	session, err := s.manager.GetAccount(ctx, s.accountID)
	if err == nil {
		return session, nil
	}
	if errors.Is(err, auth.ErrAccountNotFound) || errors.Is(err, auth.ErrRefresherNil) {
		return nil, auth.ErrSessionNotFound
	}
	return nil, err
}

// GetUserInfo 获取当前用户信息。
func (s *AuthService) GetUserInfo(ctx context.Context) (*model.User, error) {
	if s.client == nil {
		return nil, errors.New("API 客户端未初始化")
	}
	info, err := s.client.GetUserInfo(ctx)
	if err != nil {
		return nil, err
	}
	user := info.ToModel()
	return &user, nil
}

// Client 返回 API 客户端。
func (s *AuthService) Client() *cloud189.Client {
	return s.client
}
