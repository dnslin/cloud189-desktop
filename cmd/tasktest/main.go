// Task 模块集成测试
// 运行: go run ./cmd/tasktest
package main

import (
	"bufio"
	"context"
	"crypto/rand"
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

// AppUploader 实现 task.Uploader 接口（App 模式）
type AppUploader struct {
	client  *cloud189.Client
	mu      sync.Mutex
	session *cloud189.UploadSession // 保存完整的 session 状态
}

func (u *AppUploader) Mode() task.UploadMode {
	return task.UploadModeApp
}

func (u *AppUploader) InitUpload(ctx context.Context, parentID, filename string, size int64) (string, bool, []int, error) {
	session, err := u.client.InitUpload(ctx, parentID, filename, size)
	if err != nil {
		return "", false, nil, err
	}
	u.mu.Lock()
	u.session = session
	u.mu.Unlock()
	return session.UploadFileID, session.Exists(), nil, nil
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

func runTaskTests(ctx context.Context, client *cloud189.Client, rawHTTP *http.Client) {
	fmt.Println("\n========== Task 模块测试 ==========")

	// 创建 TaskManager
	manager := task.NewManager(task.WithMaxConcurrent(2))

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

	/* 上传测试暂时注释
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
	}()

	// 创建测试文件
	fmt.Println("\n--- 创建测试文件 ---")
	testFilePath := "/tmp/task_test_upload.bin"
	testFileSize := int64(50 * 1024 * 1024) // 50MB（测试多分片，便于观察暂停/恢复）
	if err := createTestFile(testFilePath, testFileSize); err != nil {
		fmt.Printf("创建测试文件失败: %v\n", err)
		return
	}
	defer os.Remove(testFilePath)
	fmt.Printf("创建测试文件: %s (%d bytes)\n", testFilePath, testFileSize)

	// 测试上传
	fmt.Println("\n--- 测试上传任务 ---")
	uploader := &AppUploader{client: client}
	uploadReader, err := NewFileReader(testFilePath)
	if err != nil {
		fmt.Printf("打开文件失败: %v\n", err)
		return
	}

	uploadCfg := task.UploadConfig{
		LocalPath: testFilePath,
		FileName:  fmt.Sprintf("upload_test_%d.bin", time.Now().Unix()),
		ParentID:  testFolderID,
	}

	taskID, err := manager.AddUpload(uploadCfg, uploader, uploadReader)
	if err != nil {
		fmt.Printf("添加上传任务失败: %v\n", err)
		return
	}
	fmt.Printf("上传任务已添加: %s\n", taskID)

	// 测试暂停/恢复（断点续传）
	go func() {
		time.Sleep(1 * time.Second) // 等待上传开始
		t, _ := manager.GetTask(taskID)
		if t.Status == task.TaskStatusRunning && t.Progress > 0 {
			fmt.Println("\n--- 测试暂停/恢复 ---")
			fmt.Printf("当前进度: %d bytes, 暂停任务...\n", t.Progress)
			if err := manager.Pause(taskID); err != nil {
				fmt.Printf("暂停失败: %v\n", err)
				return
			}
			time.Sleep(2 * time.Second)
			fmt.Println("恢复任务...")
			if err := manager.Resume(taskID); err != nil {
				fmt.Printf("恢复失败: %v\n", err)
			}
		}
	}()

	// 等待上传完成
	waitForTask(manager, taskID, 5*time.Minute)

	// 获取上传的文件 ID
	uploadTask, _ := manager.GetTask(taskID)
	if uploadTask.Status != task.TaskStatusCompleted {
		fmt.Printf("上传失败: %v\n", uploadTask.Error)
		return
	}
	上传测试暂时注释 */

	// 测试下载（使用已存在的文件）
	fmt.Println("\n--- 查找测试文件 ---")
	testFolderID := "524821226301804505"
	fileList, err := client.ListFiles(ctx, testFolderID)
	if err != nil {
		fmt.Printf("列出文件失败: %v\n", err)
		return
	}
	var downloadFileID string
	var expectedSize int64
	for _, item := range fileList.Items() {
		if item.FileName == "kmssDevPluginIdea-1.0.16.v20240109.zip" {
			downloadFileID = item.ID.String()
			expectedSize = item.FileSize
			fmt.Printf("找到测试文件: %s (ID: %s, 大小: %d bytes)\n", item.FileName, downloadFileID, expectedSize)
			break
		}
	}
	if downloadFileID == "" {
		fmt.Println("未找到测试文件 kmssDevPluginIdea-1.0.16.v20240109.zip")
		return
	}

	// 测试下载
	fmt.Println("\n--- 测试下载任务（断点续传）---")
	downloader := &AppDownloader{client: client, httpClient: rawHTTP}
	downloadPath := "/tmp/task_test_download.zip"

	// 第一次下载：下载一部分后取消
	fmt.Println("\n[第一次下载] 下载一部分后取消...")
	downloadWriter1, err := NewFileWriter(downloadPath)
	if err != nil {
		fmt.Printf("创建下载文件失败: %v\n", err)
		return
	}

	downloadCfg1 := task.DownloadConfig{
		FileID:    downloadFileID,
		LocalPath: downloadPath,
		Resume:    false, // 第一次不需要续传
	}

	downloadTaskID1, err := manager.AddDownload(downloadCfg1, downloader, downloadWriter1)
	if err != nil {
		fmt.Printf("添加下载任务失败: %v\n", err)
		return
	}

	// 等待下载一部分后取消
	time.Sleep(2 * time.Second)
	t1, _ := manager.GetTask(downloadTaskID1)
	fmt.Printf("已下载: %d bytes (%.1f%%), 取消任务...\n", t1.Progress, t1.Percent())
	manager.Cancel(downloadTaskID1)
	time.Sleep(500 * time.Millisecond) // 等待取消完成

	// 检查已下载的文件大小
	partialInfo, _ := os.Stat(downloadPath)
	fmt.Printf("部分下载文件大小: %d bytes\n", partialInfo.Size())

	// 第二次下载：断点续传
	fmt.Println("\n[第二次下载] 断点续传...")
	downloadWriter2, err := NewFileWriter(downloadPath)
	if err != nil {
		fmt.Printf("创建下载文件失败: %v\n", err)
		return
	}

	downloadCfg2 := task.DownloadConfig{
		FileID:    downloadFileID,
		LocalPath: downloadPath,
		Resume:    true, // 启用断点续传
	}

	downloadTaskID2, err := manager.AddDownload(downloadCfg2, downloader, downloadWriter2)
	if err != nil {
		fmt.Printf("添加下载任务失败: %v\n", err)
		return
	}
	fmt.Printf("下载任务已添加: %s\n", downloadTaskID2)

	// 等待下载完成
	waitForTask(manager, downloadTaskID2, 5*time.Minute)

	downloadTask, _ := manager.GetTask(downloadTaskID2)
	if downloadTask.Status != task.TaskStatusCompleted {
		fmt.Printf("下载失败: %v\n", downloadTask.Error)
		return
	}
	defer os.Remove(downloadPath)

	// 验证下载的文件
	fmt.Println("\n--- 验证下载的文件 ---")
	downloadInfo, err := os.Stat(downloadPath)
	if err != nil {
		fmt.Printf("获取下载文件信息失败: %v\n", err)
		return
	}
	fmt.Printf("下载文件大小: %d bytes (预期: %d bytes)\n", downloadInfo.Size(), expectedSize)
	if downloadInfo.Size() == expectedSize {
		fmt.Println("✓ 文件大小匹配!")
	} else {
		fmt.Println("✗ 文件大小不匹配!")
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
