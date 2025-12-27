// Web API 测试入口
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
	"time"

	"github.com/dnslin/cloud189-desktop/core/auth"
	"github.com/dnslin/cloud189-desktop/core/cloud189"
	"github.com/dnslin/cloud189-desktop/core/httpclient"
)

const webRootFolderID = "-11"

// webDebugTransport 打印请求响应便于调试
type webDebugTransport struct{ base http.RoundTripper }

func (t *webDebugTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(req)
	if err != nil {
		fmt.Printf("<<< ERROR: %v\n", err)
		return nil, err
	}
	return resp, nil
}

type webLogger struct{}

func (webLogger) Debugf(f string, a ...any) { fmt.Printf("[DEBUG] "+f+"\n", a...) }
func (webLogger) Errorf(f string, a ...any) { fmt.Printf("[ERROR] "+f+"\n", a...) }

// webMemStore 复用泛型内存存储
type webMemStore = MemoryStore[*auth.Session]

func main() {
	reader := bufio.NewReader(os.Stdin)
	log := webLogger{}

	fmt.Println("=== 天翼云盘 Web API 测试 ===")
	fmt.Println()

	// 1. 获取凭证
	fmt.Print("用户名: ")
	username := strings.TrimSpace(webReadLine(reader))
	fmt.Print("密码: ")
	password := strings.TrimSpace(webReadLine(reader))
	if username == "" || password == "" {
		fmt.Println("用户名密码不能为空")
		return
	}

	// 2. 初始化 HTTP 客户端
	jar, _ := cookiejar.New(nil)
	rawHTTP := &http.Client{
		Transport: &webDebugTransport{base: http.DefaultTransport},
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

	// 3. 混合登录（一次拿到 Web Cookie + APP Session）
	fmt.Println("\n--- 混合登录 ---")
	creds := auth.Credentials{Username: username, Password: password}
	loginClient := auth.NewLoginClient(httpClient, auth.WithLoginLogger(log))
	session, err := loginClient.HybridLogin(ctx, creds)
	if err != nil {
		fmt.Printf("登录失败: %v\n", err)
		return
	}
	fmt.Println("登录成功!")
	fmt.Printf("CookieLoginUser: %s...\n", truncate(session.CookieLoginUser, 20))
	fmt.Printf("SSON: %s...\n", truncate(session.SSON, 20))
	fmt.Printf("SessionKey: %s...\n", truncate(session.SessionKey, 20))

	// 4. 设置 AuthManager（使用 WebRefresher）
	store := &webMemStore{}
	_ = store.SaveSession(session)
	authMgr := auth.NewAuthManager()
	refresher := auth.NewWebRefresher(httpClient, store, loginClient, creds, auth.WithWebLogger(log))
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

	// 开始测试所有 Web API
	runWebTests(ctx, client, rawHTTP)
}

func webReadLine(r *bufio.Reader) string {
	line, _ := r.ReadString('\n')
	return line
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func runWebTests(ctx context.Context, client *cloud189.Client, rawHTTP *http.Client) {
	fmt.Println("\n========== 开始测试 Web API ==========")

	// 用于清理的 ID 列表
	var cleanupIDs []string
	var testFolderID string

	// 1. 获取用户简要信息（Web API）
	fmt.Println("\n--- 1. WebGet: getUserBriefInfo 获取用户简要信息 ---")
	var userBrief struct {
		SessionKey string `json:"sessionKey"`
		UserID     int64  `json:"userId"`
		NickName   string `json:"nickName"`
		cloud189.CodeResponse
	}
	err := client.WebGet(ctx, "/portal/v2/getUserBriefInfo.action", nil, &userBrief)
	if webCheckErr("getUserBriefInfo", err) {
		return
	}
	webPrintJSON(userBrief)

	// 2. FetchWebRSA 获取上传 RSA 公钥
	fmt.Println("\n--- 2. FetchWebRSA 获取上传 RSA 公钥 ---")
	rsaKey, err := client.FetchWebRSA(ctx)
	if !webCheckErr("FetchWebRSA", err) {
		fmt.Printf("PkId: %s\n", rsaKey.PkId)
		fmt.Printf("PubKey: %s...\n", truncate(rsaKey.PubKey, 50))
	}

	// 3. WebSession 获取上传 SessionKey
	fmt.Println("\n--- 3. WebSession 获取上传 SessionKey ---")
	sessionKey, err := client.WebSession(ctx)
	if !webCheckErr("WebSession", err) {
		fmt.Printf("SessionKey: %s...\n", truncate(sessionKey, 30))
	}

	// 4. 创建文件夹（使用 APP API，因为 Web API 可能没有对应端点）
	fmt.Println("\n--- 4. CreateFolder 创建测试文件夹 ---")
	folderName := fmt.Sprintf("web_test_%d", time.Now().Unix())
	folder, err := client.CreateFolder(ctx, webRootFolderID, folderName)
	if webCheckErr("CreateFolder", err) {
		fmt.Println("创建文件夹失败，后续测试可能受影响")
	} else {
		testFolderID = folder.ID.String()
		cleanupIDs = append(cleanupIDs, folder.ID.String())
		fmt.Printf("创建成功: %s (ID: %s)\n", folder.FileName, folder.ID)
	}

	// 5. Web 分片上传测试
	fmt.Println("\n--- 5. WebSimpleUpload Web 分片上传测试 ---")
	var uploadedFileID string
	if testFolderID != "" && rsaKey != nil {
		uploadName := fmt.Sprintf("web_upload_%d.txt", time.Now().Unix())
		payload := make([]byte, 512)
		_, _ = rand.Read(payload)

		// 使用 WebSimpleUpload 测试完整 Web 分片上传流程
		uploaded, err := client.WebSimpleUpload(ctx, testFolderID, uploadName, bytes.NewReader(payload), rsaKey)
		if !webCheckErr("WebSimpleUpload", err) {
			uploadedFileID = uploaded.ID.String()
			cleanupIDs = append(cleanupIDs, uploaded.ID.String())
			fmt.Printf("Web 上传成功: %s (ID: %s, 大小: %d)\n", uploaded.FileName, uploaded.ID, uploaded.FileSize)
		}
	} else {
		fmt.Println("跳过: 没有测试文件夹或 RSA 公钥")
	}

	// 6. 获取文件信息
	fmt.Println("\n--- 6. GetFileInfo 获取文件信息 ---")
	if uploadedFileID != "" {
		fileInfo, err := client.GetFileInfo(ctx, uploadedFileID)
		if !webCheckErr("GetFileInfo", err) {
			fmt.Printf("文件名: %s, 大小: %d, MD5: %s\n", fileInfo.FileName, fileInfo.FileSize, fileInfo.MD5)
		}
	} else {
		fmt.Println("跳过: 没有上传文件")
	}

	// 7. 获取下载地址
	fmt.Println("\n--- 7. GetDownloadURL 获取下载地址 ---")
	if uploadedFileID != "" {
		dlURL, err := client.GetDownloadURL(ctx, uploadedFileID)
		if !webCheckErr("GetDownloadURL", err) {
			fmt.Printf("下载地址: %s\n", dlURL)
			if rawHTTP != nil && dlURL != "" {
				webProbeDownload(ctx, rawHTTP, dlURL)
			}
		}
	} else {
		fmt.Println("跳过: 没有上传文件")
	}

	// 8. 列出文件
	fmt.Println("\n--- 8. ListFiles 列出根目录文件 ---")
	fileList, err := client.ListFiles(ctx, webRootFolderID,
		cloud189.WithListPagination(1, 10),
		cloud189.WithListOrder("lastOpTime", true),
	)
	if !webCheckErr("ListFiles", err) {
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

	// 9. 搜索文件
	fmt.Println("\n--- 9. SearchFiles 搜索文件 ---")
	if testFolderID != "" {
		searchResult, err := client.SearchFiles(ctx, "web_",
			cloud189.WithSearchFolder(testFolderID),
			cloud189.WithSearchRecursive(true),
			cloud189.WithSearchPagination(1, 10),
		)
		if !webCheckErr("SearchFiles", err) {
			fmt.Printf("搜索到 %d 个结果\n", searchResult.Count)
			for _, item := range searchResult.Items() {
				fmt.Printf("  - %s (ID: %s)\n", item.FileName, item.ID)
			}
		}
	} else {
		fmt.Println("跳过: 没有测试文件夹")
	}

	// 10. 删除测试数据
	fmt.Println("\n--- 10. DeleteFiles 删除测试数据 ---")
	if len(cleanupIDs) > 0 {
		fmt.Printf("准备删除 %d 个测试文件/文件夹...\n", len(cleanupIDs))
		err := client.DeleteFiles(ctx, cleanupIDs)
		if !webCheckErr("DeleteFiles", err) {
			fmt.Println("删除成功!")
		}
	} else {
		fmt.Println("没有需要清理的数据")
	}

	fmt.Println("\n========== Web API 测试完成 ==========")
}

func webCheckErr(api string, err error) bool {
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

func webPrintJSON(v any) {
	data, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(data))
}

func webProbeDownload(ctx context.Context, httpClient *http.Client, url string) {
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
