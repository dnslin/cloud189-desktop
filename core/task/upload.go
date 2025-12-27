package task

import (
	"context"
	"io"
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

// Uploader 上传器接口，由上层实现。
type Uploader interface {
	// InitUpload 初始化分片上传。
	// 返回 uploadFileID、是否秒传、已上传的分片列表（用于断点续传）。
	InitUpload(ctx context.Context, parentID, filename string, size int64) (uploadFileID string, exists bool, uploadedParts []int, err error)
	// UploadPart 上传分片。
	UploadPart(ctx context.Context, uploadFileID string, partNum int, data io.Reader) error
	// CommitUpload 提交上传。
	CommitUpload(ctx context.Context, uploadFileID string, fileMD5, sliceMD5 string) (fileID string, err error)
	// Mode 返回上传模式（App/Web）。
	Mode() UploadMode
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
	// 注意：分片大小固定为 10MB（天翼云服务端要求）
}

// AddUpload 添加上传任务。
func (m *Manager) AddUpload(cfg UploadConfig, uploader Uploader, reader UploadReader) (string, error) {
	task := m.CreateTask(TaskTypeUpload)
	task.LocalPath = cfg.LocalPath
	task.FileName = cfg.FileName
	task.ParentID = cfg.ParentID
	task.Total = reader.Size()

	go m.runUpload(task, uploader, reader)
	return task.ID, nil
}

// runUpload 执行上传任务。
func (m *Manager) runUpload(task *Task, uploader Uploader, reader UploadReader) {
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

	// 初始化上传
	uploadFileID, exists, uploadedParts, err := uploader.InitUpload(ctx, task.ParentID, task.FileName, fileSize)
	if err != nil {
		task.SetError(err)
		m.notifyProgress(task)
		return
	}

	// 秒传：文件已存在
	if exists {
		task.SetProgress(fileSize)
		task.SetStatus(TaskStatusCompleted)
		m.notifyProgress(task)
		return
	}

	// 构建已上传分片集合（用于断点续传）
	uploadedSet := make(map[int]bool)
	for _, partNum := range uploadedParts {
		uploadedSet[partNum] = true
	}

	// 计算分片数（固定 10MB 分片）
	sliceSize := int64(DefaultSliceSize)
	totalParts := (fileSize + sliceSize - 1) / sliceSize
	if totalParts == 0 {
		totalParts = 1
	}

	// 计算已上传的字节数（断点续传）
	var uploaded int64
	if len(uploadedParts) > 0 && uploader.Mode() == UploadModeApp {
		for partNum := range uploadedSet {
			if int64(partNum) == totalParts {
				// 最后一个分片
				uploaded += fileSize - (int64(partNum)-1)*sliceSize
			} else {
				uploaded += sliceSize
			}
		}
		task.SetProgress(uploaded)
		m.notifyProgress(task)
	}

	// 上传分片
	for partNum := int64(1); partNum <= totalParts; partNum++ {
		// 检查任务状态
		status := task.GetStatus()
		if status == TaskStatusCanceled {
			return
		}
		for status == TaskStatusPaused {
			// 暂停时等待恢复
			status = task.GetStatus()
		}

		// 跳过已上传的分片（断点续传，仅 App 模式支持）
		if uploadedSet[int(partNum)] && uploader.Mode() == UploadModeApp {
			// 跳过已上传的分片，但需要移动文件指针
			_, _ = reader.Seek(int64(partNum)*sliceSize, io.SeekStart)
			continue
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
	}

	// 提交上传
	// 注意：MD5 计算由 Uploader 实现负责
	_, err = uploader.CommitUpload(ctx, uploadFileID, "", "")
	if err != nil {
		task.SetError(err)
		m.notifyProgress(task)
		return
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
