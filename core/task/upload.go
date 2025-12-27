package task

import (
	"context"
	"io"
	"time"

	"github.com/dnslin/cloud189-desktop/core/store"
)

// DefaultSliceSize 默认分片大小（10MB）。
const DefaultSliceSize = 10 * 1024 * 1024

// UploadMode 上传模式。
type UploadMode int

const (
	// UploadModeApp 使用 App 接口上传（支持断点续传）。
	UploadModeApp UploadMode = iota
	// UploadModeWeb 使用 Web 接口上传（不支持断点续传）。
	UploadModeWeb
)

// ResumeState 断点续传恢复状态。
type ResumeState struct {
	UploadFileID string   // 之前的上传会话 ID
	UploadedSize int64    // 已上传字节数
	PartHashes   []string // 已上传分片的 MD5 列表
}

// Uploader 上传器接口，由上层实现。
type Uploader interface {
	// InitUpload 初始化或恢复分片上传。
	// resumeState 为 nil 时新建上传，否则尝试恢复。
	// 返回 uploadFileID、是否秒传、已上传字节数。
	InitUpload(ctx context.Context, parentID, filename string, size int64, resumeState *ResumeState) (uploadFileID string, exists bool, uploadedSize int64, err error)
	// UploadPart 上传分片。
	UploadPart(ctx context.Context, uploadFileID string, partNum int, data io.Reader) error
	// CommitUpload 提交上传。
	CommitUpload(ctx context.Context, uploadFileID string, fileMD5, sliceMD5 string) (fileID string, err error)
	// Mode 返回上传模式（App/Web）。
	Mode() UploadMode
	// GetPartHashes 获取已上传分片的 MD5 列表（用于断点续传状态保存）。
	GetPartHashes() []string
}

// UploadReader 上传读取器接口。
type UploadReader interface {
	io.Reader
	io.Seeker
	io.Closer
	// Size 返回文件大小。
	Size() int64
}

// UploadConfig 上传配置。
type UploadConfig struct {
	LocalPath string // 本地文件路径
	FileName  string // 文件名
	ParentID  string // 云端父目录 ID
	FileMD5   string // 文件 MD5（用于断点续传校验，可选）
	// 注意：分片大小固定为 10MB（天翼云服务端要求）
}

// AddUpload 添加上传任务。
func (m *Manager) AddUpload(cfg UploadConfig, uploader Uploader, reader UploadReader) (string, error) {
	task := m.CreateTask(TaskTypeUpload)
	task.LocalPath = cfg.LocalPath
	task.FileName = cfg.FileName
	task.ParentID = cfg.ParentID
	task.Total = reader.Size()

	go m.runUpload(task, uploader, reader, cfg.FileMD5)
	return task.ID, nil
}

// runUpload 执行上传任务。
func (m *Manager) runUpload(task *Task, uploader Uploader, reader UploadReader, fileMD5 string) {
	ctx, cancel := context.WithCancel(context.Background())
	m.registerCancel(task.ID, cancel)
	defer m.unregisterCancel(task.ID)
	defer reader.Close()

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

	fileSize := reader.Size()

	// 检查是否有可恢复的状态（断点续传）
	var resumeState *ResumeState
	if m.uploadStateStore != nil && uploader.Mode() == UploadModeApp {
		if state, err := m.uploadStateStore.LoadState(task.LocalPath); err == nil && state != nil {
			// 验证文件未修改（大小和 MD5 一致）
			if state.FileSize == fileSize && (fileMD5 == "" || state.FileMD5 == fileMD5) && state.UploadFileID != "" {
				resumeState = &ResumeState{
					UploadFileID: state.UploadFileID,
					UploadedSize: state.UploadedSize,
					PartHashes:   state.PartHashes,
				}
			}
		}
	}

	// 初始化或恢复上传
	uploadFileID, exists, uploadedSize, err := uploader.InitUpload(ctx, task.ParentID, task.FileName, fileSize, resumeState)
	if err != nil {
		task.SetError(err)
		m.notifyProgress(task)
		return
	}

	// 秒传：文件已存在
	if exists {
		if m.uploadStateStore != nil {
			_ = m.uploadStateStore.DeleteState(task.LocalPath)
		}
		task.SetProgress(fileSize)
		task.SetStatus(TaskStatusCompleted)
		m.notifyProgress(task)
		return
	}

	// 保存上传状态（用于断点续传）
	if m.uploadStateStore != nil && uploader.Mode() == UploadModeApp {
		_ = m.uploadStateStore.SaveState(task.LocalPath, &store.UploadState{
			LocalPath:    task.LocalPath,
			ParentID:     task.ParentID,
			FileName:     task.FileName,
			FileSize:     fileSize,
			FileMD5:      fileMD5,
			UploadFileID: uploadFileID,
			UploadedSize: uploadedSize,
			CreatedAt:    time.Now().Unix(),
		})
	}

	// 计算分片数（固定 10MB 分片）
	sliceSize := int64(DefaultSliceSize)
	totalParts := (fileSize + sliceSize - 1) / sliceSize
	if totalParts == 0 {
		totalParts = 1
	}

	// 计算起始分片（基于已上传字节数）
	startPart := int64(1)
	uploaded := uploadedSize
	if uploadedSize > 0 && uploader.Mode() == UploadModeApp {
		startPart = uploadedSize/sliceSize + 1
		task.SetProgress(uploaded)
		m.notifyProgress(task)
	}

	// 上传分片
	for partNum := startPart; partNum <= totalParts; partNum++ {
		// 检查任务状态
		status := task.GetStatus()
		if status == TaskStatusCanceled {
			return
		}
		for status == TaskStatusPaused {
			// 暂停时等待恢复
			time.Sleep(100 * time.Millisecond)
			status = task.GetStatus()
		}

		// 定位到分片起始位置
		_, err := reader.Seek((partNum-1)*sliceSize, io.SeekStart)
		if err != nil {
			task.SetError(err)
			m.notifyProgress(task)
			return
		}

		// 计算当前分片大小
		partSize := sliceSize
		if partNum == totalParts {
			partSize = fileSize - (partNum-1)*sliceSize
		}

		// 读取分片数据
		partData := make([]byte, partSize)
		n, err := io.ReadFull(reader, partData)
		if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
			task.SetError(err)
			m.notifyProgress(task)
			return
		}
		if n == 0 {
			break
		}

		// 上传分片
		partReader := &bytesReader{data: partData[:n]}
		if err := uploader.UploadPart(ctx, uploadFileID, int(partNum), partReader); err != nil {
			task.SetError(err)
			m.notifyProgress(task)
			return
		}

		uploaded += int64(n)
		task.SetProgress(uploaded)
		m.notifyProgress(task)

		// 更新上传状态（每个分片上传成功后）
		if m.uploadStateStore != nil && uploader.Mode() == UploadModeApp {
			_ = m.uploadStateStore.SaveState(task.LocalPath, &store.UploadState{
				LocalPath:    task.LocalPath,
				ParentID:     task.ParentID,
				FileName:     task.FileName,
				FileSize:     fileSize,
				FileMD5:      fileMD5,
				UploadFileID: uploadFileID,
				UploadedSize: uploaded,
				PartHashes:   uploader.GetPartHashes(),
				CreatedAt:    time.Now().Unix(),
			})
		}
	}

	// 提交上传
	// 注意：MD5 计算由 Uploader 实现负责
	_, err = uploader.CommitUpload(ctx, uploadFileID, "", "")
	if err != nil {
		task.SetError(err)
		m.notifyProgress(task)
		return
	}

	// 上传成功，删除状态
	if m.uploadStateStore != nil {
		_ = m.uploadStateStore.DeleteState(task.LocalPath)
	}

	task.SetStatus(TaskStatusCompleted)
	m.notifyProgress(task)
}

// bytesReader 简单的字节读取器。
type bytesReader struct {
	data []byte
	pos  int
}

func (r *bytesReader) Read(p []byte) (n int, err error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n = copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}
