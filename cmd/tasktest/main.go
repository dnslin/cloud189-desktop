// Task 模块集成测试
// 运行: go run ./cmd/tasktest
package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/dnslin/cloud189-desktop/core/auth"
	"github.com/dnslin/cloud189-desktop/core/cloud189"
	"github.com/dnslin/cloud189-desktop/core/httpclient"
	"github.com/dnslin/cloud189-desktop/core/store"
	"github.com/dnslin/cloud189-desktop/core/task"
)

const testRootFolderID = "-11"

type taskLogger struct{}

func (taskLogger) Debugf(f string, a ...any) { fmt.Printf("[DEBUG] "+f+"\n", a...) }
func (taskLogger) Errorf(f string, a ...any) { fmt.Printf("[ERROR] "+f+"\n", a...) }

// taskMemStore 内存会话存储
type taskMemStore struct {
	mu      sync.RWMutex
	session *auth.Session
}

func (m *taskMemStore) SaveSession(s any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s == nil {
		m.session = nil
		return nil
	}
	session, ok := s.(*auth.Session)
	if !ok {
		return fmt.Errorf("不支持的 Session 类型: %T", s)
	}
	m.session = session.Clone()
	return nil
}

func (m *taskMemStore) LoadSession() (any, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.session == nil {
		return nil, auth.ErrSessionNotFound
	}
	return m.session.Clone(), nil
}

func (m *taskMemStore) ClearSession() error {
	m.mu.Lock()
	m.session = nil
	m.mu.Unlock()
	return nil
}

// FileUploadStateStore 文件存储实现 UploadStateStore 接口（用于断点续传测试）
type FileUploadStateStore struct {
	mu       sync.RWMutex
	filePath string
	states   map[string]*store.UploadState
}

func NewFileUploadStateStore(filePath string) *FileUploadStateStore {
	s := &FileUploadStateStore{
		filePath: filePath,
		states:   make(map[string]*store.UploadState),
	}
	// 尝试从文件加载
	if data, err := os.ReadFile(filePath); err == nil {
		_ = json.Unmarshal(data, &s.states)
	}
	return s
}

func (s *FileUploadStateStore) SaveState(localPath string, state *store.UploadState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.states[localPath] = state
	return s.persist()
}

func (s *FileUploadStateStore) LoadState(localPath string) (*store.UploadState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if state, ok := s.states[localPath]; ok {
		return state, nil
	}
	return nil, fmt.Errorf("状态不存在")
}

func (s *FileUploadStateStore) DeleteState(localPath string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.states, localPath)
	return s.persist()
}

func (s *FileUploadStateStore) persist() error {
	data, err := json.MarshalIndent(s.states, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.filePath, data, 0644)
}

// AppUploader 实现 task.Uploader 接口（App 模式）
type AppUploader struct {
	client  *cloud189.Client
	mu      sync.Mutex
	session *cloud189.UploadSession // 保存完整的 session 状态
}

func (u *AppUploader) Mode() task.UploadMode {
	return task.UploadModeApp
}

func (u *AppUploader) InitUpload(ctx context.Context, parentID, filename string, size int64, resumeState *task.ResumeState) (string, bool, int64, error) {
	// 尝试恢复上传
	if resumeState != nil && resumeState.UploadFileID != "" {
		// 恢复上传会话（包含已上传分片的 hash）
		session := u.client.ResumeUploadSession(parentID, filename, size, resumeState.UploadFileID, resumeState.UploadedSize, resumeState.PartHashes)
		u.mu.Lock()
		u.session = session
		u.mu.Unlock()
		return resumeState.UploadFileID, false, resumeState.UploadedSize, nil
	}

	// 新建上传
	session, err := u.client.InitUpload(ctx, parentID, filename, size)
	if err != nil {
		return "", false, 0, err
	}
	u.mu.Lock()
	u.session = session
	u.mu.Unlock()
	return session.UploadFileID, session.Exists(), 0, nil
}

func (u *AppUploader) UploadPart(ctx context.Context, uploadFileID string, partNum int, data io.Reader) error {
	u.mu.Lock()
	session := u.session
	u.mu.Unlock()
	if session == nil {
		return fmt.Errorf("session 未初始化")
	}
	return u.client.UploadPart(ctx, session, partNum, data)
}

func (u *AppUploader) CommitUpload(ctx context.Context, uploadFileID string, fileMD5, sliceMD5 string) (string, error) {
	u.mu.Lock()
	session := u.session
	u.mu.Unlock()
	if session == nil {
		return "", fmt.Errorf("session 未初始化")
	}
	// 如果外部传入了 MD5，使用外部的；否则使用 session 内部计算的
	if fileMD5 != "" {
		session.FileMD5 = fileMD5
	}
	if sliceMD5 != "" {
		session.SliceMD5 = sliceMD5
	}
	info, err := u.client.CommitUpload(ctx, session)
	if err != nil {
		return "", err
	}
	return info.ID.String(), nil
}

func (u *AppUploader) GetPartHashes() []string {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.session == nil {
		return nil
	}
	return u.session.GetPartHashes()
}

// AppDownloader 实现 task.Downloader 接口（App 模式）
type AppDownloader struct {
	client     *cloud189.Client
	httpClient *http.Client
}

func (d *AppDownloader) Mode() task.DownloadMode {
	return task.DownloadModeApp
}

func (d *AppDownloader) GetDownloadURL(ctx context.Context, fileID string) (string, error) {
	return d.client.GetDownloadURL(ctx, fileID)
}

func (d *AppDownloader) GetFileInfo(ctx context.Context, fileID string) (string, int64, error) {
	info, err := d.client.GetFileInfo(ctx, fileID)
	if err != nil {
		return "", 0, err
	}
	return info.FileName, info.FileSize, nil
}

func (d *AppDownloader) HTTPClient() *http.Client {
	return d.httpClient
}

// FileWriter 实现 task.DownloadWriter 接口
type FileWriter struct {
	file *os.File
}

func NewFileWriter(path string) (*FileWriter, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, err
	}
	return &FileWriter{file: f}, nil
}

func (w *FileWriter) Write(p []byte) (int, error) { return w.file.Write(p) }
func (w *FileWriter) Seek(offset int64, whence int) (int64, error) {
	return w.file.Seek(offset, whence)
}
func (w *FileWriter) Close() error { return w.file.Close() }

// FileReader 实现 task.UploadReader 接口
type FileReader struct {
	file *os.File
	size int64
}

func NewFileReader(path string) (*FileReader, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	return &FileReader{file: f, size: info.Size()}, nil
}

func (r *FileReader) Read(p []byte) (int, error)                   { return r.file.Read(p) }
func (r *FileReader) Seek(offset int64, whence int) (int64, error) { return r.file.Seek(offset, whence) }
func (r *FileReader) Close() error                                 { return r.file.Close() }
func (r *FileReader) Size() int64                                  { return r.size }

func main() {
	reader := bufio.NewReader(os.Stdin)
	log := taskLogger{}

	fmt.Println("=== Task 模块集成测试 ===")
	fmt.Println()

	// 1. 获取凭证
	fmt.Print("用户名: ")
	username := strings.TrimSpace(taskReadLine(reader))
	fmt.Print("密码: ")
	password := strings.TrimSpace(taskReadLine(reader))
	if username == "" || password == "" {
		fmt.Println("用户名密码不能为空")
		return
	}

	// 2. 初始化 HTTP 客户端
	jar, _ := cookiejar.New(nil)
	rawHTTP := &http.Client{
		Jar:     jar,
		Timeout: 60 * time.Second,
	}
	httpClient := httpclient.NewClient(
		httpclient.WithHTTPClient(rawHTTP),
		httpclient.WithCookieJar(jar),
		httpclient.WithLogger(log),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// 3. 登录
	fmt.Println("\n--- 登录 ---")
	creds := auth.Credentials{Username: username, Password: password}
	loginClient := auth.NewLoginClient(httpClient, auth.WithLoginLogger(log))
	session, err := loginClient.AppLogin(ctx, creds)
	if err != nil {
		fmt.Printf("登录失败: %v\n", err)
		return
	}
	fmt.Println("登录成功!")

	// 4. 设置 AuthManager
	store := &taskMemStore{}
	_ = store.SaveSession(session)
	authMgr := auth.NewAuthManager()
	refresher := auth.NewAppRefresher(httpClient, store, loginClient, creds, auth.WithAppLogger(log))
	_ = authMgr.AddAccount("main", auth.AccountSession{
		DisplayName: username,
		Store:       store,
		Refresher:   refresher,
	})

	// 5. 创建 Client
	client := cloud189.NewClient(authMgr,
		cloud189.WithHTTPClient(httpClient),
		cloud189.WithLogger(log),
	).WithAccount("main")

	// 6. 运行 Task 测试
	runTaskTests(ctx, client, rawHTTP)
}

func taskReadLine(r *bufio.Reader) string {
	line, _ := r.ReadString('\n')
	return line
}

func runTaskTests(ctx context.Context, client *cloud189.Client, _ *http.Client) {
	fmt.Println("\n========== Task 模块测试 ==========")

	// 创建上传状态存储（用于断点续传）
	uploadStateStore := NewFileUploadStateStore("/tmp/upload_state.json")

	// 创建 TaskManager（启用断点续传）
	manager := task.NewManager(
		task.WithMaxConcurrent(2),
		task.WithUploadStateStore(uploadStateStore),
	)

	// 订阅进度更新（每 5% 更新一次）
	var lastPercent float64
	manager.Subscribe(func(t *task.Task) {
		percent := t.Percent()
		// 状态变化或进度每 5% 更新一次
		if percent-lastPercent >= 5 || percent == 100 || t.Status != task.TaskStatusRunning {
			speed := float64(t.Speed) / 1024 / 1024 // MB/s
			fmt.Printf("[%s] %s: %.1f%% (%.2f MB/s) - %s\n",
				t.Type.String(), t.FileName, percent, speed, t.Status.String())
			lastPercent = percent
		}
	})

	// ========== 上传断点续传测试 ==========
	fmt.Println("\n========== 上传断点续传测试 ==========")

	// 创建测试文件夹
	fmt.Println("\n--- 创建测试文件夹 ---")
	folderName := fmt.Sprintf("task_test_%d", time.Now().Unix())
	folder, err := client.CreateFolder(ctx, testRootFolderID, folderName)
	if err != nil {
		fmt.Printf("创建文件夹失败: %v\n", err)
		return
	}
	testFolderID := folder.ID.String()
	fmt.Printf("创建成功: %s (ID: %s)\n", folderName, testFolderID)

	defer func() {
		// 清理测试数据
		fmt.Println("\n--- 清理测试数据 ---")
		if err := client.DeleteFiles(ctx, []string{testFolderID}); err != nil {
			fmt.Printf("清理失败: %v\n", err)
		} else {
			fmt.Println("清理成功!")
		}
		// 清理状态文件
		os.Remove("/tmp/upload_state.json")
	}()

	// 创建测试文件（50MB，5个分片）
	fmt.Println("\n--- 创建测试文件 ---")
	testFilePath := "/tmp/task_test_upload.bin"
	testFileSize := int64(50 * 1024 * 1024) // 50MB
	if err := createTestFile(testFilePath, testFileSize); err != nil {
		fmt.Printf("创建测试文件失败: %v\n", err)
		return
	}
	defer os.Remove(testFilePath)
	fmt.Printf("创建测试文件: %s (%d bytes, %d 个分片)\n", testFilePath, testFileSize, testFileSize/(10*1024*1024))

	uploadFileName := fmt.Sprintf("upload_resume_test_%d.bin", time.Now().Unix())

	// ===== 第一次上传：上传一部分后取消 =====
	fmt.Println("\n--- [第一次上传] 上传一部分后取消 ---")
	uploader1 := &AppUploader{client: client}
	uploadReader1, err := NewFileReader(testFilePath)
	if err != nil {
		fmt.Printf("打开文件失败: %v\n", err)
		return
	}

	uploadCfg1 := task.UploadConfig{
		LocalPath: testFilePath,
		FileName:  uploadFileName,
		ParentID:  testFolderID,
	}

	taskID1, err := manager.AddUpload(uploadCfg1, uploader1, uploadReader1)
	if err != nil {
		fmt.Printf("添加上传任务失败: %v\n", err)
		return
	}
	fmt.Printf("上传任务已添加: %s\n", taskID1)

	// 等待上传进度达到 30% 后取消
	targetPercent := 30.0
	fmt.Printf("等待上传进度达到 %.0f%% 后取消...\n", targetPercent)
	for {
		t1, _ := manager.GetTask(taskID1)
		if t1.Status == task.TaskStatusCompleted || t1.Status == task.TaskStatusFailed || t1.Status == task.TaskStatusCanceled {
			fmt.Printf("任务已结束: %s\n", t1.Status.String())
			break
		}
		if t1.Percent() >= targetPercent {
			fmt.Printf("已上传: %d bytes (%.1f%%), 取消任务...\n", t1.Progress, t1.Percent())
			manager.Cancel(taskID1)
			time.Sleep(500 * time.Millisecond)
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	// 检查状态文件
	fmt.Println("\n--- 检查上传状态文件 ---")
	if stateData, err := os.ReadFile("/tmp/upload_state.json"); err == nil {
		fmt.Printf("状态文件内容:\n%s\n", string(stateData))
	}

	// ===== 第二次上传：断点续传 =====
	fmt.Println("\n--- [第二次上传] 断点续传 ---")

	// 重新创建 Manager（模拟进程重启）
	uploadStateStore2 := NewFileUploadStateStore("/tmp/upload_state.json")
	manager2 := task.NewManager(
		task.WithMaxConcurrent(2),
		task.WithUploadStateStore(uploadStateStore2),
	)
	manager2.Subscribe(func(t *task.Task) {
		percent := t.Percent()
		if percent-lastPercent >= 5 || percent == 100 || t.Status != task.TaskStatusRunning {
			speed := float64(t.Speed) / 1024 / 1024
			fmt.Printf("[%s] %s: %.1f%% (%.2f MB/s) - %s\n",
				t.Type.String(), t.FileName, percent, speed, t.Status.String())
			lastPercent = percent
		}
	})

	uploader2 := &AppUploader{client: client}
	uploadReader2, err := NewFileReader(testFilePath)
	if err != nil {
		fmt.Printf("打开文件失败: %v\n", err)
		return
	}

	uploadCfg2 := task.UploadConfig{
		LocalPath: testFilePath,
		FileName:  uploadFileName,
		ParentID:  testFolderID,
	}

	taskID2, err := manager2.AddUpload(uploadCfg2, uploader2, uploadReader2)
	if err != nil {
		fmt.Printf("添加上传任务失败: %v\n", err)
		return
	}
	fmt.Printf("上传任务已添加: %s\n", taskID2)

	// 等待上传完成
	waitForTask(manager2, taskID2, 5*time.Minute)

	t2, _ := manager2.GetTask(taskID2)
	if t2.Status == task.TaskStatusCompleted {
		fmt.Println("\n✓ 上传断点续传测试成功!")
	} else {
		fmt.Printf("\n✗ 上传失败: %v\n", t2.Error)
	}

	// 验证文件已上传
	fmt.Println("\n--- 验证上传的文件 ---")
	fileList, err := client.ListFiles(ctx, testFolderID)
	if err != nil {
		fmt.Printf("列出文件失败: %v\n", err)
		return
	}
	for _, item := range fileList.Items() {
		if item.FileName == uploadFileName {
			fmt.Printf("找到上传的文件: %s (大小: %d bytes)\n", item.FileName, item.FileSize)
			if item.FileSize == testFileSize {
				fmt.Println("✓ 文件大小匹配!")
			} else {
				fmt.Printf("✗ 文件大小不匹配! 预期: %d, 实际: %d\n", testFileSize, item.FileSize)
			}
			break
		}
	}

	fmt.Println("\n========== Task 测试完成 ==========")
}

func createTestFile(path string, size int64) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	buf := make([]byte, 1024*1024) // 1MB buffer
	var written int64
	for written < size {
		toWrite := size - written
		if toWrite > int64(len(buf)) {
			toWrite = int64(len(buf))
		}
		rand.Read(buf[:toWrite])
		n, err := f.Write(buf[:toWrite])
		if err != nil {
			return err
		}
		written += int64(n)
	}
	return nil
}

func waitForTask(manager *task.Manager, taskID string, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		t, err := manager.GetTask(taskID)
		if err != nil {
			fmt.Printf("获取任务状态失败: %v\n", err)
			return
		}
		if t.Status == task.TaskStatusCompleted ||
			t.Status == task.TaskStatusFailed ||
			t.Status == task.TaskStatusCanceled {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	fmt.Println("任务超时")
}
