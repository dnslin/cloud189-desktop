package store

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/dnslin/cloud189-desktop/core/auth"
)

const (
	tuiDataDirName      = ".cloud189-tui"
	sessionStorageFile  = "sessions.json"
	defaultSessionPerm  = 0o600
	defaultStoragePerms = 0o700
)

// JSONSessionStore 使用 JSON 文件保存会话信息。
type JSONSessionStore struct {
	filePath string
	mu       sync.RWMutex
}

// DefaultSessionFilePath 返回默认的会话文件路径：~/.cloud189-tui/sessions.json。
func DefaultSessionFilePath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("获取用户目录失败: %w", err)
	}
	return filepath.Join(homeDir, tuiDataDirName, sessionStorageFile), nil
}

// NewJSONSessionStore 创建 JSONSessionStore，路径留空时使用默认路径。
func NewJSONSessionStore(filePath string) *JSONSessionStore {
	if filePath == "" {
		if defaultPath, err := DefaultSessionFilePath(); err == nil {
			filePath = defaultPath
		}
	}
	return &JSONSessionStore{filePath: filePath}
}

// SaveSession 保存会话到 JSON 文件，传入 nil 时清空文件。
func (s *JSONSessionStore) SaveSession(session *auth.Session) error {
	if s == nil {
		return fmt.Errorf("会话存储未初始化")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if session == nil {
		return s.clearLocked()
	}
	if s.filePath == "" {
		return fmt.Errorf("会话文件路径为空")
	}
	if err := os.MkdirAll(filepath.Dir(s.filePath), defaultStoragePerms); err != nil {
		return fmt.Errorf("创建会话目录失败: %w", err)
	}
	if err := os.Chmod(s.filePath, defaultSessionPerm); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("收紧会话文件权限失败: %w", err)
	}
	clone := session.Clone()
	data, err := json.MarshalIndent(clone, "", "  ")
	if err != nil {
		return fmt.Errorf("编码会话失败: %w", err)
	}
	tmpPath := s.filePath + ".tmp"
	tmpFile, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, defaultSessionPerm)
	if err != nil {
		return fmt.Errorf("创建临时会话文件失败: %w", err)
	}
	if n, err := tmpFile.Write(data); err != nil || n < len(data) {
		if err == nil {
			err = io.ErrShortWrite
		}
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("写入临时会话文件失败: %w", err)
	}
	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("同步临时会话文件失败: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("关闭临时会话文件失败: %w", err)
	}
	if err := os.Rename(tmpPath, s.filePath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("替换会话文件失败: %w", err)
	}
	if err := os.Chmod(s.filePath, defaultSessionPerm); err != nil {
		return fmt.Errorf("设置会话文件权限失败: %w", err)
	}
	return nil
}

// LoadSession 从 JSON 文件读取会话，不存在时返回 ErrSessionNotFound。
func (s *JSONSessionStore) LoadSession() (*auth.Session, error) {
	if s == nil {
		return nil, fmt.Errorf("会话存储未初始化")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.filePath == "" {
		return nil, auth.ErrSessionNotFound
	}
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, auth.ErrSessionNotFound
		}
		return nil, fmt.Errorf("读取会话文件失败: %w", err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, auth.ErrSessionNotFound
	}
	var session auth.Session
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("解析会话失败: %w", err)
	}
	return session.Clone(), nil
}

// ClearSession 删除会话文件。
func (s *JSONSessionStore) ClearSession() error {
	if s == nil {
		return fmt.Errorf("会话存储未初始化")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.clearLocked()
}

func (s *JSONSessionStore) clearLocked() error {
	if s.filePath == "" {
		return nil
	}
	if err := os.Remove(s.filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("删除会话文件失败: %w", err)
	}
	return nil
}
