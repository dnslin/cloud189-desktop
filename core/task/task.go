// Package task 提供上传/下载任务管理能力。
package task

import (
	"sync"
	"time"
)

// TaskType 任务类型。
type TaskType int

const (
	// TaskTypeDownload 下载任务。
	TaskTypeDownload TaskType = iota
	// TaskTypeUpload 上传任务。
	TaskTypeUpload
)

// String 返回任务类型的字符串表示。
func (t TaskType) String() string {
	switch t {
	case TaskTypeDownload:
		return "download"
	case TaskTypeUpload:
		return "upload"
	default:
		return "unknown"
	}
}

// TaskStatus 任务状态。
type TaskStatus int

const (
	// TaskStatusPending 等待中。
	TaskStatusPending TaskStatus = iota
	// TaskStatusRunning 运行中。
	TaskStatusRunning
	// TaskStatusPaused 已暂停。
	TaskStatusPaused
	// TaskStatusCompleted 已完成。
	TaskStatusCompleted
	// TaskStatusFailed 失败。
	TaskStatusFailed
	// TaskStatusCanceled 已取消。
	TaskStatusCanceled
)

// String 返回任务状态的字符串表示。
func (s TaskStatus) String() string {
	switch s {
	case TaskStatusPending:
		return "pending"
	case TaskStatusRunning:
		return "running"
	case TaskStatusPaused:
		return "paused"
	case TaskStatusCompleted:
		return "completed"
	case TaskStatusFailed:
		return "failed"
	case TaskStatusCanceled:
		return "canceled"
	default:
		return "unknown"
	}
}

// Task 表示一个上传或下载任务。
type Task struct {
	mu sync.RWMutex

	// 基本信息
	ID        string     // 任务唯一标识
	Type      TaskType   // 任务类型
	Status    TaskStatus // 任务状态
	CreatedAt time.Time  // 创建时间
	UpdatedAt time.Time  // 更新时间

	// 进度信息
	Progress int64 // 已完成字节数
	Total    int64 // 总字节数
	Speed    int64 // 当前速度（字节/秒）

	// 文件信息
	FileID    string // 云端文件 ID（下载时使用）
	FileName  string // 文件名
	LocalPath string // 本地路径
	ParentID  string // 云端父目录 ID（上传时使用）

	// 错误信息
	Error error // 任务错误

	// 内部状态
	lastProgress int64     // 上次进度（用于计算速度）
	lastTime     time.Time // 上次更新时间
}

// NewTask 创建新任务。
func NewTask(id string, taskType TaskType) *Task {
	now := time.Now()
	return &Task{
		ID:        id,
		Type:      taskType,
		Status:    TaskStatusPending,
		CreatedAt: now,
		UpdatedAt: now,
		lastTime:  now,
	}
}

// SetStatus 设置任务状态。
func (t *Task) SetStatus(status TaskStatus) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.Status = status
	t.UpdatedAt = time.Now()
}

// GetStatus 获取任务状态。
func (t *Task) GetStatus() TaskStatus {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.Status
}

// SetProgress 设置任务进度并计算速度。
func (t *Task) SetProgress(progress int64) {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(t.lastTime).Seconds()
	if elapsed > 0 {
		t.Speed = int64(float64(progress-t.lastProgress) / elapsed)
	}

	t.Progress = progress
	t.lastProgress = progress
	t.lastTime = now
	t.UpdatedAt = now
}

// GetProgress 获取任务进度。
func (t *Task) GetProgress() (progress, total int64) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.Progress, t.Total
}

// GetSpeed 获取当前速度。
func (t *Task) GetSpeed() int64 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.Speed
}

// Percent 返回完成百分比（0-100）。
func (t *Task) Percent() float64 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.Total <= 0 {
		return 0
	}
	return float64(t.Progress) / float64(t.Total) * 100
}

// SetError 设置任务错误。
func (t *Task) SetError(err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.Error = err
	t.Status = TaskStatusFailed
	t.UpdatedAt = time.Now()
}

// GetError 获取任务错误。
func (t *Task) GetError() error {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.Error
}

// Clone 返回任务的副本（用于安全传递给回调）。
func (t *Task) Clone() *Task {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return &Task{
		ID:        t.ID,
		Type:      t.Type,
		Status:    t.Status,
		CreatedAt: t.CreatedAt,
		UpdatedAt: t.UpdatedAt,
		Progress:  t.Progress,
		Total:     t.Total,
		Speed:     t.Speed,
		FileID:    t.FileID,
		FileName:  t.FileName,
		LocalPath: t.LocalPath,
		ParentID:  t.ParentID,
		Error:     t.Error,
	}
}

// ProgressCallback 进度回调函数类型。
type ProgressCallback func(task *Task)
