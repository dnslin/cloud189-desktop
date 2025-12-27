// Web API Task 模块集成测试
// 运行: go run ./cmd/webtasktest
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

type webLogger struct{}

func (webLogger) Debugf(f string, a ...any) { fmt.Printf("[DEBUG] "+f+"\n", a...) }
func (webLogger) Errorf(f string, a ...any) { fmt.Printf("[ERROR] "+f+"\n", a...) }

// webMemStore 内存会话存储
type webMemStore struct {
	mu      sync.RWMutex
	session *auth.Session
}

func (m *webMemStore) SaveSession(s any) error {
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

func (m *webMemStore) LoadSession() (any, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.session == nil {
		return nil, auth.ErrSessionNotFound
	}
	return m.session.Clone(), nil
}

func (m *webMemStore) ClearSession() error {
	m.mu.Lock()
	m.session = nil
	m.mu.Unlock()
	return nil
}

// WebUploader 实现 task.Uploader 接口（Web 模式）
type WebUploader struct {
	client  *cloud189.Client
	rsaKey  *cloud189.WebRSA
	mu      sync.Mutex
	session *cloud189.UploadSession
}

func (u *WebUploader) Mode() task.UploadMode {
	return task.UploadModeWeb
}

func (u *WebUploader) InitUpload(ctx context.Context, parentID, filename string, size int64, resumeState *task.ResumeState) (string, bool, int64, error) {
	// Web 模式不支持断点续传，忽略 resumeState
	session, err := u.client.WebInitUpload(ctx, parentID, filename, size, u.rsaKey)
	if err != nil {
		return "", false, 0, err
	}
	u.mu.Lock()
	u.session = session
	u.mu.Unlock()
	return session.UploadFileID, session.Exists(), 0, nil
}

func (u *WebUploader) UploadPart(ctx context.Context, uploadFileID string, partNum int, data io.Reader) error {
	u.mu.Lock()
	session := u.session
	u.mu.Unlock()
	if session == nil {
		return fmt.Errorf("session 未初始化")
	}
	return u.client.WebUploadPart(ctx, session, partNum, data, u.rsaKey)
}

func (u *WebUploader) CommitUpload(ctx context.Context, uploadFileID string, fileMD5, sliceMD5 string) (string, error) {
	u.mu.Lock()
	session := u.session
	u.mu.Unlock()
	if session == nil {
		return "", fmt.Errorf("session 未初始化")
	}
	if fileMD5 != "" {
		session.FileMD5 = fileMD5
	}
	if sliceMD5 != "" {
		session.SliceMD5 = sliceMD5
	}
	info, err := u.client.WebCommitUpload(ctx, session, u.rsaKey)
	if err != nil {
		return "", err
	}
	return info.ID.String(), nil
}

func (u *WebUploader) GetPartHashes() []string {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.session == nil {
		return nil
	}
	return u.session.GetPartHashes()
}

// WebDownloader 实现 task.Downloader 接口（Web 模式）
type WebDownloader struct {
	client     *cloud189.Client
	httpClient *http.Client
}

func (d *WebDownloader) Mode() task.DownloadMode {
	return task.DownloadModeWeb
}

func (d *WebDownloader) GetDownloadURL(ctx context.Context, fileID string) (string, error) {
	// Web 模式也使用 App 接口获取下载链接（通用）
	return d.client.GetDownloadURL(ctx, fileID)
}

func (d *WebDownloader) GetFileInfo(ctx context.Context, fileID string) (string, int64, error) {
	info, err := d.client.GetFileInfo(ctx, fileID)
	if err != nil {
		return "", 0, err
	}
	return info.FileName, info.FileSize, nil
}

func (d *WebDownloader) HTTPClient() *http.Client {
	return d.httpClient
}

// FileReader 文件读取器
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

func (r *FileReader) Read(p []byte) (int, error)         { return r.file.Read(p) }
func (r *FileReader) Seek(offset int64, whence int) (int64, error) { return r.file.Seek(offset, whence) }
func (r *FileReader) Close() error                       { return r.file.Close() }
func (r *FileReader) Size() int64                        { return r.size }

// FileWriter 文件写入器
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

func (w *FileWriter) Write(p []byte) (int, error)                  { return w.file.Write(p) }
func (w *FileWriter) Seek(offset int64, whence int) (int64, error) { return w.file.Seek(offset, whence) }
func (w *FileWriter) Close() error                                 { return w.file.Close() }

func main() {
	fmt.Println("=== Web API Task 模块集成测试 ===")

	reader := bufio.NewReader(os.Stdin)
	fmt.Print("\n用户名: ")
	username := strings.TrimSpace(readLine(reader))
	fmt.Print("密码: ")
	password := strings.TrimSpace(readLine(reader))

	if username == "" || password == "" {
		fmt.Println("用户名密码不能为空")
		return
	}

	// 初始化 HTTP 客户端
	jar, _ := cookiejar.New(nil)
	rawHTTP := &http.Client{
		Jar:     jar,
		Timeout: 60 * time.Second,
	}
	log := webLogger{}
	httpClient := httpclient.NewClient(
		httpclient.WithHTTPClient(rawHTTP),
		httpclient.WithCookieJar(jar),
		httpclient.WithLogger(log),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// 登录（使用 HybridLogin 同时获取 Web Cookie 和 App 会话）
	fmt.Println("\n--- 登录 (HybridLogin) ---")
	creds := auth.Credentials{Username: username, Password: password}
	loginClient := auth.NewLoginClient(httpClient, auth.WithLoginLogger(log))
	session, err := loginClient.HybridLogin(ctx, creds)
	if err != nil {
		fmt.Printf("登录失败: %v\n", err)
		return
	}
	fmt.Println("登录成功!")

	// 创建 AuthManager
	store := &webMemStore{}
	_ = store.SaveSession(session)
	authMgr := auth.NewAuthManager()
	refresher := auth.NewAppRefresher(httpClient, store, loginClient, creds, auth.WithAppLogger(log))
	_ = authMgr.AddAccount("main", auth.AccountSession{
		DisplayName: username,
		Store:       store,
		Refresher:   refresher,
	})

	// 创建 Client
	client := cloud189.NewClient(authMgr,
		cloud189.WithHTTPClient(httpClient),
		cloud189.WithLogger(log),
	).WithAccount("main")

	// 运行 Web Task 测试
	runWebTaskTests(ctx, client, rawHTTP)
}

func readLine(r *bufio.Reader) string {
	line, _ := r.ReadString('\n')
	return strings.TrimSpace(line)
}

func runWebTaskTests(ctx context.Context, client *cloud189.Client, rawHTTP *http.Client) {
	fmt.Println("\n========== Web API Task 测试 ==========")

	// 获取 RSA 公钥（Web 上传需要）
	fmt.Println("\n--- 获取 RSA 公钥 ---")
	rsaKey, err := client.FetchWebRSA(ctx)
	if err != nil {
		fmt.Printf("获取 RSA 公钥失败: %v\n", err)
		return
	}
	fmt.Println("RSA 公钥获取成功")

	// 创建测试文件夹
	fmt.Println("\n--- 创建测试文件夹 ---")
	testFolderName := fmt.Sprintf("web_task_test_%d", time.Now().Unix())
	folder, err := client.CreateFolder(ctx, testRootFolderID, testFolderName)
	if err != nil {
		fmt.Printf("创建文件夹失败: %v\n", err)
		return
	}
	testFolderID := folder.ID.String()
	fmt.Printf("创建成功: %s (ID: %s)\n", testFolderName, testFolderID)

	// 清理函数
	defer func() {
		fmt.Println("\n--- 清理测试数据 ---")
		if err := client.DeleteFiles(ctx, []string{testFolderID}); err != nil {
			fmt.Printf("清理失败: %v\n", err)
		} else {
			fmt.Println("清理成功!")
		}
	}()

	// 创建 Task Manager
	manager := task.NewManager(task.WithMaxConcurrent(2))
	var lastPercent float64
	manager.Subscribe(func(t *task.Task) {
		percent := t.Percent()
		if percent-lastPercent >= 10 || percent == 100 || t.Status != task.TaskStatusRunning {
			speed := float64(t.Speed) / 1024 / 1024
			fmt.Printf("[%s] %s: %.1f%% (%.2f MB/s) - %s\n",
				t.Type.String(), t.FileName, percent, speed, t.Status.String())
			lastPercent = percent
		}
	})

	// ===== Web 上传测试 =====
	fmt.Println("\n--- Web 上传测试 ---")
	testFilePath := "/tmp/web_task_test_upload.bin"
	testFileSize := int64(25 * 1024 * 1024) // 25MB（2.5 个分片）
	if err := createTestFile(testFilePath, testFileSize); err != nil {
		fmt.Printf("创建测试文件失败: %v\n", err)
		return
	}
	defer os.Remove(testFilePath)
	fmt.Printf("创建测试文件: %s (%d bytes)\n", testFilePath, testFileSize)

	uploadFileName := fmt.Sprintf("web_upload_test_%d.bin", time.Now().Unix())
	uploader := &WebUploader{client: client, rsaKey: rsaKey}
	uploadReader, err := NewFileReader(testFilePath)
	if err != nil {
		fmt.Printf("打开文件失败: %v\n", err)
		return
	}

	uploadCfg := task.UploadConfig{
		LocalPath: testFilePath,
		FileName:  uploadFileName,
		ParentID:  testFolderID,
	}

	lastPercent = 0
	taskID, err := manager.AddUpload(uploadCfg, uploader, uploadReader)
	if err != nil {
		fmt.Printf("添加上传任务失败: %v\n", err)
		return
	}
	fmt.Printf("上传任务已添加: %s\n", taskID)

	// 等待上传完成
	waitForTask(manager, taskID, 5*time.Minute)

	t, _ := manager.GetTask(taskID)
	if t.Status != task.TaskStatusCompleted {
		fmt.Printf("✗ Web 上传失败: %v\n", t.Error)
		return
	}
	fmt.Println("✓ Web 上传成功!")

	// 获取上传的文件 ID
	files, err := client.ListFiles(ctx, testFolderID)
	if err != nil {
		fmt.Printf("列出文件失败: %v\n", err)
		return
	}
	var uploadedFileID string
	for _, f := range files.Items() {
		if f.FileName == uploadFileName {
			uploadedFileID = f.ID.String()
			fmt.Printf("找到上传的文件: %s (ID: %s, 大小: %d bytes)\n", f.FileName, uploadedFileID, f.FileSize)
			break
		}
	}
	if uploadedFileID == "" {
		fmt.Println("✗ 未找到上传的文件")
		return
	}

	// ===== Web 下载测试 =====
	fmt.Println("\n--- Web 下载测试 ---")
	downloadPath := "/tmp/web_task_test_download.bin"
	defer os.Remove(downloadPath)

	downloader := &WebDownloader{client: client, httpClient: rawHTTP}
	downloadWriter, err := NewFileWriter(downloadPath)
	if err != nil {
		fmt.Printf("创建下载文件失败: %v\n", err)
		return
	}

	downloadCfg := task.DownloadConfig{
		FileID:    uploadedFileID,
		LocalPath: downloadPath,
		Resume:    false,
	}

	lastPercent = 0
	downloadTaskID, err := manager.AddDownload(downloadCfg, downloader, downloadWriter)
	if err != nil {
		fmt.Printf("添加下载任务失败: %v\n", err)
		return
	}
	fmt.Printf("下载任务已添加: %s\n", downloadTaskID)

	// 等待下载完成
	waitForTask(manager, downloadTaskID, 5*time.Minute)

	dt, _ := manager.GetTask(downloadTaskID)
	if dt.Status != task.TaskStatusCompleted {
		fmt.Printf("✗ Web 下载失败: %v\n", dt.Error)
		return
	}
	fmt.Println("✓ Web 下载成功!")

	// 验证下载的文件大小
	downloadInfo, err := os.Stat(downloadPath)
	if err != nil {
		fmt.Printf("获取下载文件信息失败: %v\n", err)
		return
	}
	if downloadInfo.Size() == testFileSize {
		fmt.Printf("✓ 文件大小匹配: %d bytes\n", downloadInfo.Size())
	} else {
		fmt.Printf("✗ 文件大小不匹配: 期望 %d, 实际 %d\n", testFileSize, downloadInfo.Size())
	}

	// ===== 下载断点续传测试 =====
	fmt.Println("\n--- 下载断点续传测试 ---")
	resumeDownloadPath := "/tmp/web_task_test_resume_download.bin"
	defer os.Remove(resumeDownloadPath)

	// 第一次下载：下载一部分后取消
	fmt.Println("\n[第一次下载] 下载一部分后取消...")
	resumeWriter1, err := NewFileWriter(resumeDownloadPath)
	if err != nil {
		fmt.Printf("创建下载文件失败: %v\n", err)
		return
	}

	resumeDownloadCfg := task.DownloadConfig{
		FileID:    uploadedFileID,
		LocalPath: resumeDownloadPath,
		Resume:    false,
	}

	lastPercent = 0
	resumeTaskID1, err := manager.AddDownload(resumeDownloadCfg, downloader, resumeWriter1)
	if err != nil {
		fmt.Printf("添加下载任务失败: %v\n", err)
		return
	}

	// 等待下载进度达到 30% 后取消
	targetPercent := 30.0
	fmt.Printf("等待下载进度达到 %.0f%% 后取消...\n", targetPercent)
	for {
		rt1, _ := manager.GetTask(resumeTaskID1)
		if rt1.Status == task.TaskStatusCompleted || rt1.Status == task.TaskStatusFailed || rt1.Status == task.TaskStatusCanceled {
			fmt.Printf("任务已结束: %s\n", rt1.Status.String())
			break
		}
		if rt1.Percent() >= targetPercent {
			fmt.Printf("已下载: %d bytes (%.1f%%), 取消任务...\n", rt1.Progress, rt1.Percent())
			manager.Cancel(resumeTaskID1)
			time.Sleep(500 * time.Millisecond)
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	// 检查已下载的文件大小
	partialInfo, err := os.Stat(resumeDownloadPath)
	if err != nil {
		fmt.Printf("获取部分下载文件信息失败: %v\n", err)
		return
	}
	fmt.Printf("已下载文件大小: %d bytes (%.1f%%)\n", partialInfo.Size(), float64(partialInfo.Size())/float64(testFileSize)*100)

	// 第二次下载：断点续传
	fmt.Println("\n[第二次下载] 断点续传...")
	resumeWriter2, err := NewFileWriter(resumeDownloadPath)
	if err != nil {
		fmt.Printf("创建下载文件失败: %v\n", err)
		return
	}

	resumeDownloadCfg2 := task.DownloadConfig{
		FileID:    uploadedFileID,
		LocalPath: resumeDownloadPath,
		Resume:    true, // 启用断点续传
	}

	lastPercent = 0
	resumeTaskID2, err := manager.AddDownload(resumeDownloadCfg2, downloader, resumeWriter2)
	if err != nil {
		fmt.Printf("添加下载任务失败: %v\n", err)
		return
	}
	fmt.Printf("下载任务已添加: %s\n", resumeTaskID2)

	// 等待下载完成
	waitForTask(manager, resumeTaskID2, 5*time.Minute)

	rt2, _ := manager.GetTask(resumeTaskID2)
	if rt2.Status != task.TaskStatusCompleted {
		fmt.Printf("✗ 断点续传下载失败: %v\n", rt2.Error)
		return
	}

	// 验证最终文件大小
	finalInfo, err := os.Stat(resumeDownloadPath)
	if err != nil {
		fmt.Printf("获取最终文件信息失败: %v\n", err)
		return
	}
	if finalInfo.Size() == testFileSize {
		fmt.Printf("✓ 下载断点续传成功! 文件大小: %d bytes\n", finalInfo.Size())
	} else {
		fmt.Printf("✗ 文件大小不匹配: 期望 %d, 实际 %d\n", testFileSize, finalInfo.Size())
	}

	fmt.Println("\n========== Web API Task 测试完成 ==========")
}

func waitForTask(manager *task.Manager, taskID string, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		t, err := manager.GetTask(taskID)
		if err != nil {
			return
		}
		if t.Status == task.TaskStatusCompleted || t.Status == task.TaskStatusFailed || t.Status == task.TaskStatusCanceled {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func createTestFile(path string, size int64) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	buf := make([]byte, 1024*1024) // 1MB 缓冲区
	remaining := size
	for remaining > 0 {
		n := int64(len(buf))
		if n > remaining {
			n = remaining
		}
		rand.Read(buf[:n])
		if _, err := f.Write(buf[:n]); err != nil {
			return err
		}
		remaining -= n
	}
	return nil
}
