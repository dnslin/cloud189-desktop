package task

import (
	"context"
	"errors"
	"sync"

	"github.com/dnslin/cloud189-desktop/core/store"
	"github.com/google/uuid"
)

// 错误定义。
var (
	ErrTaskNotFound  = errors.New("task: 任务不存在")
	ErrTaskCanceled  = errors.New("task: 任务已取消")
	ErrInvalidStatus = errors.New("task: 无效的任务状态")
)

// Manager 任务管理器，负责任务调度和生命周期管理。
type Manager struct {
	mu        sync.RWMutex
	tasks     map[string]*Task              // 任务映射
	callbacks []ProgressCallback            // 进度回调列表
	cancels   map[string]context.CancelFunc // 任务取消函数

	maxConcurrent    int                    // 最大并发数
	semaphore        chan struct{}          // 并发控制信号量
	uploadStateStore store.UploadStateStore // 上传状态存储（可选，用于断点续传）
}

// ManagerOption 管理器配置选项。
type ManagerOption func(*Manager)

// WithMaxConcurrent 设置最大并发数。
func WithMaxConcurrent(n int) ManagerOption {
	return func(m *Manager) {
		if n > 0 {
			m.maxConcurrent = n
		}
	}
}

// WithUploadStateStore 设置上传状态存储（启用断点续传）。
func WithUploadStateStore(s store.UploadStateStore) ManagerOption {
	return func(m *Manager) {
		m.uploadStateStore = s
	}
}

// NewManager 创建任务管理器。
func NewManager(opts ...ManagerOption) *Manager {
	m := &Manager{
		tasks:         make(map[string]*Task),
		callbacks:     make([]ProgressCallback, 0),
		cancels:       make(map[string]context.CancelFunc),
		maxConcurrent: 3, // 默认最大并发数
	}
	for _, opt := range opts {
		opt(m)
	}
	m.semaphore = make(chan struct{}, m.maxConcurrent)
	return m
}

// generateID 生成任务 ID。
func generateID() string {
	return uuid.New().String()
}

// CreateTask 创建任务（内部使用）。
func (m *Manager) CreateTask(taskType TaskType) *Task {
	task := NewTask(generateID(), taskType)
	m.mu.Lock()
	m.tasks[task.ID] = task
	m.mu.Unlock()
	return task
}

// GetTask 获取任务。
func (m *Manager) GetTask(taskID string) (*Task, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	task, ok := m.tasks[taskID]
	if !ok {
		return nil, ErrTaskNotFound
	}
	return task.Clone(), nil
}

// ListTasks 列出所有任务。
func (m *Manager) ListTasks() []*Task {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]*Task, 0, len(m.tasks))
	for _, task := range m.tasks {
		result = append(result, task.Clone())
	}
	return result
}

// ListTasksByStatus 按状态列出任务。
func (m *Manager) ListTasksByStatus(status TaskStatus) []*Task {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]*Task, 0)
	for _, task := range m.tasks {
		if task.GetStatus() == status {
			result = append(result, task.Clone())
		}
	}
	return result
}

// RemoveTask 移除任务。
func (m *Manager) RemoveTask(taskID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	task, ok := m.tasks[taskID]
	if !ok {
		return ErrTaskNotFound
	}
	// 只能移除已完成、失败或取消的任务
	status := task.GetStatus()
	if status != TaskStatusCompleted && status != TaskStatusFailed && status != TaskStatusCanceled {
		return ErrInvalidStatus
	}
	delete(m.tasks, taskID)
	delete(m.cancels, taskID)
	return nil
}

// Cancel 取消任务。
func (m *Manager) Cancel(taskID string) error {
	m.mu.Lock()
	task, ok := m.tasks[taskID]
	cancel, hasCancel := m.cancels[taskID]
	m.mu.Unlock()

	if !ok {
		return ErrTaskNotFound
	}

	status := task.GetStatus()
	if status == TaskStatusCompleted || status == TaskStatusFailed || status == TaskStatusCanceled {
		return ErrInvalidStatus
	}

	if hasCancel {
		cancel()
	}
	task.SetStatus(TaskStatusCanceled)
	m.notifyProgress(task)
	return nil
}

// Pause 暂停任务。
func (m *Manager) Pause(taskID string) error {
	m.mu.RLock()
	task, ok := m.tasks[taskID]
	m.mu.RUnlock()

	if !ok {
		return ErrTaskNotFound
	}

	status := task.GetStatus()
	if status != TaskStatusRunning && status != TaskStatusPending {
		return ErrInvalidStatus
	}

	task.SetStatus(TaskStatusPaused)
	m.notifyProgress(task)
	return nil
}

// Resume 恢复任务。
func (m *Manager) Resume(taskID string) error {
	m.mu.RLock()
	task, ok := m.tasks[taskID]
	m.mu.RUnlock()

	if !ok {
		return ErrTaskNotFound
	}

	if task.GetStatus() != TaskStatusPaused {
		return ErrInvalidStatus
	}

	task.SetStatus(TaskStatusPending)
	m.notifyProgress(task)
	return nil
}

// Subscribe 订阅进度更新。
func (m *Manager) Subscribe(callback ProgressCallback) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.callbacks = append(m.callbacks, callback)
}

// notifyProgress 通知进度更新。
func (m *Manager) notifyProgress(task *Task) {
	m.mu.RLock()
	callbacks := make([]ProgressCallback, len(m.callbacks))
	copy(callbacks, m.callbacks)
	m.mu.RUnlock()

	clone := task.Clone()
	for _, cb := range callbacks {
		cb(clone)
	}
}

// acquireSemaphore 获取信号量。
func (m *Manager) acquireSemaphore(ctx context.Context) error {
	select {
	case m.semaphore <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// releaseSemaphore 释放信号量。
func (m *Manager) releaseSemaphore() {
	<-m.semaphore
}

// registerCancel 注册取消函数。
func (m *Manager) registerCancel(taskID string, cancel context.CancelFunc) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cancels[taskID] = cancel
}

// unregisterCancel 注销取消函数。
func (m *Manager) unregisterCancel(taskID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.cancels, taskID)
}
