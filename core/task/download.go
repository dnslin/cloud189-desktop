package task

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"time"
)

// DownloadMode 下载模式。
type DownloadMode int

const (
	// DownloadModeApp 使用 App 接口下载。
	DownloadModeApp DownloadMode = iota
	// DownloadModeWeb 使用 Web 接口下载。
	DownloadModeWeb
)

// Downloader 下载器接口，由上层实现。
type Downloader interface {
	// GetDownloadURL 获取下载链接。
	GetDownloadURL(ctx context.Context, fileID string) (string, error)
	// GetFileInfo 获取文件信息（用于获取文件大小）。
	GetFileInfo(ctx context.Context, fileID string) (fileName string, fileSize int64, err error)
	// HTTPClient 返回 HTTP 客户端（用于下载）。
	HTTPClient() *http.Client
	// Mode 返回下载模式（App/Web）。
	Mode() DownloadMode
}

// DownloadWriter 下载写入器接口。
type DownloadWriter interface {
	io.Writer
	io.Seeker
	io.Closer
}

// DownloadConfig 下载配置。
type DownloadConfig struct {
	FileID    string // 云端文件 ID
	LocalPath string // 本地保存路径
	Resume    bool   // 是否断点续传
}

// AddDownload 添加下载任务。
func (m *Manager) AddDownload(cfg DownloadConfig, downloader Downloader, writer DownloadWriter) (string, error) {
	task := m.CreateTask(TaskTypeDownload)
	task.FileID = cfg.FileID
	task.LocalPath = cfg.LocalPath

	go m.runDownload(task, cfg, downloader, writer)
	return task.ID, nil
}

// runDownload 执行下载任务。
func (m *Manager) runDownload(task *Task, cfg DownloadConfig, downloader Downloader, writer DownloadWriter) {
	ctx, cancel := context.WithCancel(context.Background())
	m.registerCancel(task.ID, cancel)
	defer m.unregisterCancel(task.ID)
	defer writer.Close()

	// 获取信号量
	if err := m.acquireSemaphore(ctx); err != nil {
		task.SetError(err)
		m.notifyProgress(task)
		return
	}
	defer m.releaseSemaphore()

	// 检查任务状态
	if task.GetStatus() == TaskStatusCanceled {
		return
	}

	task.SetStatus(TaskStatusRunning)
	m.notifyProgress(task)

	// 获取文件信息
	fileName, fileSize, err := downloader.GetFileInfo(ctx, cfg.FileID)
	if err != nil {
		task.SetError(err)
		m.notifyProgress(task)
		return
	}
	task.FileName = fileName
	task.Total = fileSize

	// 获取下载链接
	downloadURL, err := downloader.GetDownloadURL(ctx, cfg.FileID)
	if err != nil {
		task.SetError(err)
		m.notifyProgress(task)
		return
	}

	// 断点续传：获取已下载大小
	var startOffset int64
	if cfg.Resume {
		startOffset, _ = writer.Seek(0, io.SeekEnd)
		if startOffset >= fileSize {
			// 已下载完成
			task.SetProgress(fileSize)
			task.SetStatus(TaskStatusCompleted)
			m.notifyProgress(task)
			return
		}
		task.SetProgress(startOffset)
	}

	// 创建下载请求
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		task.SetError(err)
		m.notifyProgress(task)
		return
	}

	// 设置 Range 头（断点续传）
	if startOffset > 0 {
		req.Header.Set("Range", "bytes="+strconv.FormatInt(startOffset, 10)+"-")
	}

	// 执行下载
	client := downloader.HTTPClient()
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		task.SetError(err)
		m.notifyProgress(task)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		task.SetError(&DownloadError{StatusCode: resp.StatusCode, Status: resp.Status})
		m.notifyProgress(task)
		return
	}

	// 写入数据
	buf := make([]byte, 32*1024) // 32KB 缓冲区
	downloaded := startOffset

	for {
		// 检查任务状态
		status := task.GetStatus()
		if status == TaskStatusCanceled {
			return
		}
		for status == TaskStatusPaused {
			time.Sleep(100 * time.Millisecond)
			status = task.GetStatus()
		}

		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			_, writeErr := writer.Write(buf[:n])
			if writeErr != nil {
				task.SetError(writeErr)
				m.notifyProgress(task)
				return
			}
			downloaded += int64(n)
			task.SetProgress(downloaded)
			m.notifyProgress(task)
		}

		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			task.SetError(readErr)
			m.notifyProgress(task)
			return
		}
	}

	task.SetStatus(TaskStatusCompleted)
	m.notifyProgress(task)
}

// DownloadError 下载错误。
type DownloadError struct {
	StatusCode int
	Status     string
}

func (e *DownloadError) Error() string {
	return "下载失败: " + e.Status
}
