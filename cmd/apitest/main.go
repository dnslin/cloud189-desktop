// 扁平 Client API 测试入口
package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
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
)

const rootFolderID = "-11"

// debugTransport 打印请求响应便于调试
type debugTransport struct{ base http.RoundTripper }

func (t *debugTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(req)
	if err != nil {
		fmt.Printf("<<< ERROR: %v\n", err)
		return nil, err
	}
	return resp, nil
}

type logger struct{}

func (logger) Debugf(f string, a ...any) { fmt.Printf("[DEBUG] "+f+"\n", a...) }
func (logger) Errorf(f string, a ...any) { fmt.Printf("[ERROR] "+f+"\n", a...) }

// MemoryStore 内存会话存储
type MemoryStore[T any] struct {
	mu         sync.RWMutex
	session    T
	hasSession bool
}

func (m *MemoryStore[T]) SaveSession(session T) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if any(session) == nil {
		m.reset()
		return nil
	}
	m.session = cloneSession(session)
	m.hasSession = true
	return nil
}

func (m *MemoryStore[T]) LoadSession() (T, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if !m.hasSession {
		var zero T
		return zero, auth.ErrSessionNotFound
	}
	return cloneSession(m.session), nil
}

func (m *MemoryStore[T]) ClearSession() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reset()
	return nil
}

func (m *MemoryStore[T]) reset() {
	var zero T
	m.session = zero
	m.hasSession = false
}

func cloneSession[T any](session T) T {
	s, ok := any(session).(*auth.Session)
	if !ok || s == nil {
		return session
	}
	return any(s.Clone()).(T)
}

func mains() {
	reader := bufio.NewReader(os.Stdin)
	log := logger{}

	fmt.Println("=== 天翼云盘扁平 Client API 测试 ===")
	fmt.Println()

	// 1. 获取凭证
	fmt.Print("用户名: ")
	username := strings.TrimSpace(readLine(reader))
	fmt.Print("密码: ")
	password := strings.TrimSpace(readLine(reader))
	if username == "" || password == "" {
		fmt.Println("用户名密码不能为空")
		return
	}

	// 2. 初始化 HTTP 客户端
	jar, _ := cookiejar.New(nil)
	rawHTTP := &http.Client{
		Transport: &debugTransport{base: http.DefaultTransport},
		Jar:       jar,
		Timeout:   60 * time.Second,
	}
	httpClient := httpclient.NewClient(
		httpclient.WithHTTPClient(rawHTTP),
		httpclient.WithCookieJar(jar),
		httpclient.WithLogger(log),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
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
	store := &MemoryStore[*auth.Session]{}
	_ = store.SaveSession(session)
	authMgr := auth.NewAuthManager()
	refresher := auth.NewAppRefresher(httpClient, store, loginClient, creds, auth.WithAppLogger(log))
	_ = authMgr.AddAccount("main", auth.AccountSession{
		DisplayName: username,
		Store:       store,
		Refresher:   refresher,
	})

	// 5. 创建扁平 Client
	client := cloud189.NewClient(authMgr,
		cloud189.WithHTTPClient(httpClient),
		cloud189.WithLogger(log),
	).WithAccount("main")

	// 开始测试所有 API
	runAllTests(ctx, client, rawHTTP)
}

func readLine(r *bufio.Reader) string {
	line, _ := r.ReadString('\n')
	return line
}

func runAllTests(ctx context.Context, client *cloud189.Client, rawHTTP *http.Client) {
	fmt.Println("\n========== 开始测试所有 API ==========")

	// 用于清理的 ID 列表
	var cleanupIDs []string
	var testFolderID string

	// 1. GetUserInfo
	fmt.Println("\n--- 1. GetUserInfo 获取用户信息 ---")
	userInfo, err := client.GetUserInfo(ctx)
	if checkErr("GetUserInfo", err) {
		return
	}
	printJSON(userInfo)

	// 2. GetCapacity
	fmt.Println("\n--- 2. GetCapacity 获取容量 ---")
	capacity, err := client.GetCapacity(ctx)
	if !checkErr("GetCapacity", err) {
		fmt.Printf("总容量: %.2f GB, 可用: %.2f GB\n",
			float64(capacity.Capacity)/1024/1024/1024,
			float64(capacity.Available)/1024/1024/1024)
	}

	// 3. SignIn
	fmt.Println("\n--- 3. SignIn 签到 ---")
	signResult, err := client.SignIn(ctx)
	if !checkErr("SignIn", err) {
		fmt.Printf("签到结果: %s (code=%d)\n", signResult.ResultTip, signResult.Result)
	}

	// 4. ListFiles
	fmt.Println("\n--- 4. ListFiles 列出根目录文件 ---")
	fileList, err := client.ListFiles(ctx, rootFolderID,
		cloud189.WithListPagination(1, 10),
		cloud189.WithListOrder("lastOpTime", true),
	)
	if !checkErr("ListFiles", err) {
		items := fileList.Items()
		fmt.Printf("共 %d 个条目:\n", len(items))
		for i, item := range items {
			if i >= 5 {
				fmt.Printf("  ... 还有 %d 个\n", len(items)-5)
				break
			}
			kind := "文件"
			if item.IsFolder {
				kind = "目录"
			}
			fmt.Printf("  [%s] %s (ID: %s)\n", kind, item.FileName, item.ID)
		}
	}

	// 5. CreateFolder
	fmt.Println("\n--- 5. CreateFolder 创建文件夹 ---")
	folderName := fmt.Sprintf("test_folder_%d", time.Now().Unix())
	folder, err := client.CreateFolder(ctx, rootFolderID, folderName)
	if checkErr("CreateFolder", err) {
		fmt.Println("创建文件夹失败，后续测试可能受影响")
	} else {
		testFolderID = folder.ID.String()
		cleanupIDs = append(cleanupIDs, folder.ID.String())
		fmt.Printf("创建成功: %s (ID: %s)\n", folder.FileName, folder.ID)
	}

	// 6. SimpleUpload
	fmt.Println("\n--- 6. SimpleUpload 上传文件 ---")
	var uploadedFileID string
	if testFolderID != "" {
		uploadName := fmt.Sprintf("test_file_%d.txt", time.Now().Unix())
		payload := make([]byte, 1024)
		_, _ = rand.Read(payload)

		uploaded, err := client.SimpleUpload(ctx, testFolderID, uploadName, bytes.NewReader(payload))
		if !checkErr("SimpleUpload", err) {
			uploadedFileID = uploaded.ID.String()
			cleanupIDs = append(cleanupIDs, uploaded.ID.String())
			fmt.Printf("上传成功: %s (ID: %s, 大小: %d)\n", uploaded.FileName, uploaded.ID, uploaded.FileSize)
		}
	} else {
		fmt.Println("跳过: 没有测试文件夹")
	}

	// 7. GetFileInfo
	fmt.Println("\n--- 7. GetFileInfo 获取文件信息 ---")
	if uploadedFileID != "" {
		fileInfo, err := client.GetFileInfo(ctx, uploadedFileID)
		if !checkErr("GetFileInfo", err) {
			fmt.Printf("文件名: %s, 大小: %d, MD5: %s\n", fileInfo.FileName, fileInfo.FileSize, fileInfo.MD5)
		}
	} else {
		fmt.Println("跳过: 没有上传文件")
	}

	// 8. GetDownloadURL
	fmt.Println("\n--- 8. GetDownloadURL 获取下载地址 ---")
	if uploadedFileID != "" {
		dlURL, err := client.GetDownloadURL(ctx, uploadedFileID)
		if !checkErr("GetDownloadURL", err) {
			fmt.Printf("下载地址: %s\n", dlURL)
			// 探测下载
			if rawHTTP != nil && dlURL != "" {
				probeDownload(ctx, rawHTTP, dlURL)
			}
		}
	} else {
		fmt.Println("跳过: 没有上传文件")
	}

	// 9. SearchFiles
	fmt.Println("\n--- 9. SearchFiles 搜索文件 ---")
	if uploadedFileID != "" {
		searchResult, err := client.SearchFiles(ctx, "test_file",
			cloud189.WithSearchFolder(testFolderID),
			cloud189.WithSearchRecursive(true),
			cloud189.WithSearchPagination(1, 10),
		)
		if !checkErr("SearchFiles", err) {
			fmt.Printf("搜索到 %d 个结果\n", searchResult.Count)
			for _, item := range searchResult.Items() {
				fmt.Printf("  - %s (ID: %s)\n", item.FileName, item.ID)
			}
		}
	} else {
		fmt.Println("跳过: 没有上传文件")
	}

	// 10. RenameFile
	fmt.Println("\n--- 10. RenameFile 重命名文件 ---")
	if uploadedFileID != "" {
		newName := fmt.Sprintf("renamed_%d.txt", time.Now().Unix())
		err := client.RenameFile(ctx, uploadedFileID, newName)
		if !checkErr("RenameFile", err) {
			fmt.Printf("重命名成功: -> %s\n", newName)
		}
	} else {
		fmt.Println("跳过: 没有上传文件")
	}

	// 11. CopyFiles
	fmt.Println("\n--- 11. CopyFiles 复制文件 ---")
	if uploadedFileID != "" && testFolderID != "" {
		// 先创建一个目标文件夹
		copyDestName := fmt.Sprintf("copy_dest_%d", time.Now().Unix())
		copyDest, err := client.CreateFolder(ctx, rootFolderID, copyDestName)
		if !checkErr("CreateFolder(复制目标)", err) {
			cleanupIDs = append(cleanupIDs, copyDest.ID.String())
			err = client.CopyFiles(ctx, []string{uploadedFileID}, copyDest.ID.String())
			if !checkErr("CopyFiles", err) {
				fmt.Printf("复制成功: 文件已复制到 %s\n", copyDestName)
			}
		}
	} else {
		fmt.Println("跳过: 没有上传文件或测试文件夹")
	}

	// 12. MoveFiles
	fmt.Println("\n--- 12. MoveFiles 移动文件 ---")
	if uploadedFileID != "" && testFolderID != "" {
		// 创建移动目标文件夹
		moveDestName := fmt.Sprintf("move_dest_%d", time.Now().Unix())
		moveDest, err := client.CreateFolder(ctx, rootFolderID, moveDestName)
		if !checkErr("CreateFolder(移动目标)", err) {
			cleanupIDs = append(cleanupIDs, moveDest.ID.String())
			err = client.MoveFiles(ctx, []string{uploadedFileID}, moveDest.ID.String())
			if !checkErr("MoveFiles", err) {
				fmt.Printf("移动成功: 文件已移动到 %s\n", moveDestName)
				// 移动后文件不在原文件夹了，从 cleanupIDs 移除（会随目标文件夹一起删除）
				for i, id := range cleanupIDs {
					if id == uploadedFileID {
						cleanupIDs = append(cleanupIDs[:i], cleanupIDs[i+1:]...)
						break
					}
				}
			}
		}
	} else {
		fmt.Println("跳过: 没有上传文件或测试文件夹")
	}

	// 13. DeleteFiles
	fmt.Println("\n--- 13. DeleteFiles 删除测试数据 ---")
	if len(cleanupIDs) > 0 {
		fmt.Printf("准备删除 %d 个测试文件/文件夹...\n", len(cleanupIDs))
		err := client.DeleteFiles(ctx, cleanupIDs)
		if !checkErr("DeleteFiles", err) {
			fmt.Println("删除成功!")
		}
	} else {
		fmt.Println("没有需要清理的数据")
	}

	fmt.Println("\n========== 测试完成 ==========")
}

func checkErr(api string, err error) bool {
	if err == nil {
		return false
	}
	var ce *cloud189.CloudError
	if errors.As(err, &ce) {
		fmt.Printf("[%s] 失败: code=%d, msg=%s, http=%d\n", api, ce.Code, ce.Message, ce.HTTPStatus)
	} else {
		fmt.Printf("[%s] 失败: %v\n", api, err)
	}
	return true
}

func printJSON(v any) {
	data, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(data))
}

func probeDownload(ctx context.Context, httpClient *http.Client, url string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		fmt.Printf("下载探测失败: %v\n", err)
		return
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 32))
	fmt.Printf("下载探测: %d, Content-Length=%s\n", resp.StatusCode, resp.Header.Get("Content-Length"))
}
