// 手动测试入口，用于验证 core 层 API 集成
package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/md5"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gowsp/cloud189-desktop/core/auth"
	"github.com/gowsp/cloud189-desktop/core/cloud189"
	"github.com/gowsp/cloud189-desktop/core/httpclient"
)

// 调试 Transport
type debugTransport struct {
	base http.RoundTripper
}

func (t *debugTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	fmt.Printf(">>> %s %s\n", req.Method, req.URL.String())
	for k, v := range req.Header {
		if k == "SessionKey" || k == "Signature" || k == "Date" {
			fmt.Printf("    %s: %s\n", k, v)
		}
	}
	if req.Body != nil && req.GetBody != nil {
		body, _ := req.GetBody()
		data, _ := io.ReadAll(body)
		if len(data) > 0 {
			if len(data) < 1000 {
				fmt.Printf("    Body: %s\n", string(data))
			} else {
				fmt.Printf("    Body: (%d bytes)\n", len(data))
			}
		}
	}

	resp, err := t.base.RoundTrip(req)
	if err != nil {
		fmt.Printf("<<< ERROR: %v\n", err)
		return nil, err
	}

	fmt.Printf("<<< %d %s\n", resp.StatusCode, resp.Status)
	if loc := resp.Header.Get("Location"); loc != "" {
		fmt.Printf("    Location: %s\n", loc)
	}
	return resp, nil
}

// 简单日志实现
type consoleLogger struct{}

func (consoleLogger) Debugf(format string, args ...any) { fmt.Printf("[DEBUG] "+format+"\n", args...) }
func (consoleLogger) Errorf(format string, args ...any) { fmt.Printf("[ERROR] "+format+"\n", args...) }

func main() {
	reader := bufio.NewReader(os.Stdin)
	logger := consoleLogger{}

	fmt.Println("=== 天翼云盘 API 测试工具 ===")
	fmt.Println()

	// 选择登录方式
	fmt.Println("选择登录方式:")
	fmt.Println("1. App 端登录 (获取 SessionKey/SessionSecret)")
	fmt.Println("2. Web 端登录 (获取 Cookie)")
	fmt.Print("请输入 (1/2): ")
	choice, _ := reader.ReadString('\n')
	choice = strings.TrimSpace(choice)

	// 输入账号密码
	fmt.Print("用户名 (手机号): ")
	username, _ := reader.ReadString('\n')
	username = strings.TrimSpace(username)

	fmt.Print("密码: ")
	password, _ := reader.ReadString('\n')
	password = strings.TrimSpace(password)

	if username == "" || password == "" {
		fmt.Println("错误: 用户名和密码不能为空")
		return
	}

	// 创建带调试的 HTTP 客户端
	jar, _ := cookiejar.New(nil)
	rawHTTP := &http.Client{
		Transport: &debugTransport{base: http.DefaultTransport},
		Jar:       jar,
	}

	// 创建 HTTP 客户端
	httpClient := httpclient.NewClient(
		httpclient.WithHTTPClient(rawHTTP),
		httpclient.WithCookieJar(jar),
		httpclient.WithLogger(logger),
	)
	loginClient := auth.NewLoginClient(httpClient, auth.WithLoginLogger(logger))

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	var session *auth.Session
	var err error

	fmt.Println()
	fmt.Println("正在登录...")

	creds := auth.Credentials{Username: username, Password: password}

	if choice == "2" {
		session, err = loginClient.WebLogin(ctx, creds)
	} else {
		session, err = loginClient.AppLogin(ctx, creds)
	}

	if err != nil {
		fmt.Printf("登录失败: %v\n", err)
		fmt.Println()
		fmt.Println("提示: 如果是 'Bad Request' 错误，可能是:")
		fmt.Println("1. 网络问题或 API 端点变化")
		fmt.Println("2. 请求参数格式不正确")
		fmt.Println("3. 需要验证码（频繁登录触发）")
		fmt.Println()
		fmt.Println("建议: 设置环境变量 189_MODE=1 查看详细请求日志")
		return
	}

	fmt.Println()
	fmt.Println("=== 登录成功 ===")
	printJSON("Session", session)
	fmt.Printf("SessionKey: %s\n", session.SessionKey)
	fmt.Printf("SessionSecret: %s\n", session.SessionSecret)
	fmt.Printf("AccessToken: %s\n", session.AccessToken)

	// 测试 API 调用
	fmt.Println()
	fmt.Println("=== 测试 API 调用 ===")

	apiClient := cloud189.NewClient(session,
		cloud189.WithHTTPClient(httpClient),
		cloud189.WithLogger(logger),
	)

	// 根据登录方式选择 API 调用方法
	isWebLogin := choice == "2"

	// 获取用户信息
	fmt.Println()
	fmt.Println("1. 获取用户信息...")
	var userInfo map[string]any
	if isWebLogin {
		// Web 端使用不同的 API 路径
		err = apiClient.WebGet(ctx, "/open/user/getUserInfoForPortal.action", nil, &userInfo)
	} else {
		err = apiClient.AppGet(ctx, "/getUserInfo.action", nil, &userInfo)
	}
	if err != nil {
		fmt.Printf("获取用户信息失败: %v\n", err)
	} else {
		printJSON("用户信息", userInfo)
	}

	// 获取根目录文件列表
	fmt.Println()
	fmt.Println("2. 获取根目录文件列表...")
	var fileList map[string]any
	params := map[string]string{
		"folderId":   "-11",
		"pageNum":    "1",
		"pageSize":   "10",
		"mediaType":  "0",
		"orderBy":    "lastOpTime",
		"descending": "true",
	}
	if isWebLogin {
		// Web 端使用不同的 API 路径
		err = apiClient.WebGet(ctx, "/open/file/listFiles.action", params, &fileList)
	} else {
		err = apiClient.AppGet(ctx, "/listFiles.action", params, &fileList)
	}
	if err != nil {
		fmt.Printf("获取文件列表失败: %v\n", err)
	} else {
		printJSON("文件列表", fileList)
	}

	// 获取云盘容量（容量信息已包含在 getUserInfo 响应中）
	fmt.Println()
	fmt.Println("3. 云盘容量信息（来自 getUserInfo）:")
	if userInfo != nil {
		available, _ := userInfo["available"].(float64)
		capacity, _ := userInfo["capacity"].(float64)
		used := capacity - available
		fmt.Printf("   总容量: %.2f GB\n", capacity/1024/1024/1024)
		fmt.Printf("   已使用: %.2f GB\n", used/1024/1024/1024)
		fmt.Printf("   可用: %.2f GB\n", available/1024/1024/1024)
		fmt.Printf("   使用率: %.1f%%\n", used*100/capacity)
	}

	// 签到（仅 App 端支持）
	if !isWebLogin {
		fmt.Println()
		fmt.Println("4. 每日签到...")
		var signResult map[string]any
		err = apiClient.AppGet(ctx, "/mkt/userSign.action", nil, &signResult)
		if err != nil {
			fmt.Printf("签到失败: %v\n", err)
		} else {
			printJSON("签到结果", signResult)
		}
	}

	// 测试上传接口
	fmt.Println()
	fmt.Println("=== 测试上传接口 ===")
	if err := testUpload(ctx, apiClient, isWebLogin, "-11"); err != nil {
		fmt.Printf("上传测试失败: %v\n", err)
	} else {
		fmt.Println("上传测试完成")
	}

	// 搜索文件
	fmt.Println()
	fmt.Println("5. 搜索文件 (关键词: test)...")
	var searchResult map[string]any
	searchParams := map[string]string{
		"folderId":  "-11",
		"filename":  "test",
		"recursive": "1",
		"mediaType": "0",
		"pageNum":   "1",
		"pageSize":  "10",
	}
	if isWebLogin {
		err = apiClient.WebGet(ctx, "/open/file/searchFiles.action", searchParams, &searchResult)
	} else {
		err = apiClient.AppGet(ctx, "/searchFiles.action", searchParams, &searchResult)
	}
	if err != nil {
		fmt.Printf("搜索失败: %v\n", err)
	} else {
		printJSON("搜索结果", searchResult)
	}

	// 创建测试文件夹
	fmt.Println()
	testFolderName := fmt.Sprintf("api_test_%d", time.Now().Unix())
	fmt.Printf("6. 创建文件夹 (%s)...\n", testFolderName)
	var mkdirResult map[string]any
	var createdFolderName, createdFolderId string
	mkdirParams := map[string]string{
		"parentFolderId": "-11",
		"folderName":     testFolderName,
	}
	if isWebLogin {
		err = apiClient.WebPost(ctx, "/open/file/createFolder.action", mkdirParams, &mkdirResult)
	} else {
		err = apiClient.AppPost(ctx, "/createFolder.action", mkdirParams, &mkdirResult)
	}
	if err != nil {
		fmt.Printf("创建文件夹失败: %v\n", err)
	} else {
		printJSON("创建文件夹结果", mkdirResult)
		if name, ok := mkdirResult["name"].(string); ok {
			createdFolderName = name
		}
		if id, ok := mkdirResult["id"].(json.Number); ok {
			createdFolderId = id.String()
		}
	}

	// 创建第二个测试文件夹用于移动/复制测试
	var targetFolderId, targetFolderName string
	if createdFolderId != "" {
		fmt.Println()
		targetFolderName = fmt.Sprintf("api_test_target_%d", time.Now().Unix())
		fmt.Printf("6b. 创建目标文件夹 (%s)...\n", targetFolderName)
		var targetResult map[string]any
		targetParams := map[string]string{
			"parentFolderId": "-11",
			"folderName":     targetFolderName,
		}
		if isWebLogin {
			err = apiClient.WebPost(ctx, "/open/file/createFolder.action", targetParams, &targetResult)
		} else {
			err = apiClient.AppPost(ctx, "/createFolder.action", targetParams, &targetResult)
		}
		if err != nil {
			fmt.Printf("创建目标文件夹失败: %v\n", err)
		} else {
			if id, ok := targetResult["id"].(json.Number); ok {
				targetFolderId = id.String()
			}
			fmt.Printf("   目标文件夹ID: %s\n", targetFolderId)
		}
	}

	// 测试复制（将第一个文件夹复制到目标文件夹）
	if createdFolderId != "" && targetFolderId != "" {
		fmt.Println()
		fmt.Printf("6c. 复制文件夹 (%s -> %s)...\n", createdFolderName, targetFolderName)
		var copyResult map[string]any
		if isWebLogin {
			taskInfo := fmt.Sprintf(`[{"fileId":"%s","fileName":"%s","isFolder":1}]`, createdFolderId, createdFolderName)
			copyParams := map[string]string{
				"type":           "COPY",
				"taskInfos":      taskInfo,
				"targetFolderId": targetFolderId,
			}
			err = apiClient.WebPost(ctx, "/open/batch/createBatchTask.action", copyParams, &copyResult)
		} else {
			copyParams := map[string]string{
				"fileId":             createdFolderId,
				"destFileName":       createdFolderName,
				"destParentFolderId": targetFolderId,
			}
			err = apiClient.AppPost(ctx, "/copyFile.action", copyParams, &copyResult)
		}
		if err != nil {
			fmt.Printf("复制失败: %v\n", err)
		} else {
			fmt.Println("复制成功")
		}
	}

	// 从文件列表中获取第一个文件的下载URL
	fmt.Println()
	fmt.Println("7. 获取文件下载URL...")
	if fileList != nil {
		if fileListData, ok := fileList["fileListAO"].(map[string]any); ok {
			if files, ok := fileListData["fileList"].([]any); ok && len(files) > 0 {
				if firstFile, ok := files[0].(map[string]any); ok {
					var fileId, fileName string
					if id, ok := firstFile["id"].(json.Number); ok {
						fileId = id.String()
					}
					if name, ok := firstFile["name"].(string); ok {
						fileName = name
					}
					fmt.Printf("   文件: %s (ID: %s)\n", fileName, fileId)

					var downloadResult map[string]any
					downloadParams := map[string]string{"fileId": fileId}
					if isWebLogin {
						err = apiClient.WebGet(ctx, "/open/file/getFileDownloadUrl.action", downloadParams, &downloadResult)
					} else {
						err = apiClient.AppGet(ctx, "/getFileDownloadUrl.action", downloadParams, &downloadResult)
					}
					if err != nil {
						fmt.Printf("获取下载URL失败: %v\n", err)
					} else {
						printJSON("下载URL", downloadResult)
						// 测试实际下载
						if dlURL, ok := downloadResult["fileDownloadUrl"].(string); ok {
							fmt.Println("   尝试下载...")
							dlReq, _ := http.NewRequestWithContext(ctx, http.MethodGet, dlURL, nil)
							dlReq.Header.Set("User-Agent", "desktop")
							dlResp, dlErr := rawHTTP.Do(dlReq)
							if dlErr != nil {
								fmt.Printf("   下载失败: %v\n", dlErr)
							} else {
								fmt.Printf("   下载响应: %d, Content-Length: %s\n", dlResp.StatusCode, dlResp.Header.Get("Content-Length"))
								dlResp.Body.Close()
							}
						}
					}
				}
			} else {
				fmt.Println("   根目录没有文件，跳过下载测试")
			}
		}
	}

	// 删除测试文件夹
	foldersToDelete := []struct{ id, name string }{}
	if createdFolderId != "" {
		foldersToDelete = append(foldersToDelete, struct{ id, name string }{createdFolderId, createdFolderName})
	}
	if targetFolderId != "" {
		foldersToDelete = append(foldersToDelete, struct{ id, name string }{targetFolderId, targetFolderName})
	}
	if len(foldersToDelete) > 0 {
		fmt.Println()
		fmt.Println("8. 删除测试文件夹...")
		for _, f := range foldersToDelete {
			fmt.Printf("   删除: %s (ID: %s)...", f.name, f.id)
			var deleteResult map[string]any
			if isWebLogin {
				taskInfo := fmt.Sprintf(`[{"fileId":"%s","fileName":"%s","isFolder":1}]`, f.id, f.name)
				deleteParams := map[string]string{
					"type":           "DELETE",
					"taskInfos":      taskInfo,
					"targetFolderId": "",
				}
				err = apiClient.WebPost(ctx, "/open/batch/createBatchTask.action", deleteParams, &deleteResult)
			} else {
				deleteParams := map[string]string{"fileIdList": f.id}
				err = apiClient.AppPost(ctx, "/batchDeleteFile.action", deleteParams, &deleteResult)
			}
			if err != nil {
				fmt.Printf(" 失败: %v\n", err)
			} else {
				fmt.Println(" 成功")
			}
		}
	}

	fmt.Println()
	fmt.Println("=== 测试完成 ===")
}

func printJSON(title string, v any) {
	data, _ := json.MarshalIndent(v, "", "  ")
	fmt.Printf("%s:\n%s\n", title, string(data))
}

// 上传相关响应结构体
type initUploadResp struct {
	Code string `json:"code,omitempty"`
	Data struct {
		UploadType     int    `json:"uploadType,omitempty"`
		UploadHost     string `json:"uploadHost,omitempty"`
		UploadFileId   string `json:"uploadFileId,omitempty"`
		FileDataExists int    `json:"fileDataExists,omitempty"`
	} `json:"data,omitempty"`
}

type uploadUrlResp struct {
	Code       string `json:"code,omitempty"`
	UploadUrls map[string]struct {
		RequestURL    string `json:"requestURL,omitempty"`
		RequestHeader string `json:"requestHeader,omitempty"`
	} `json:"uploadUrls,omitempty"`
}

type commitResp struct {
	Code string `json:"code,omitempty"`
	File struct {
		Id       string `json:"userFileId,omitempty"`
		FileName string `json:"file_name,omitempty"`
	} `json:"file,omitempty"`
}

// testUpload 测试上传接口
func testUpload(ctx context.Context, apiClient *cloud189.Client, isWebLogin bool, parentFolderId string) error {
	// 1. 创建临时测试文件（1KB 随机数据）
	fileName := fmt.Sprintf("upload_test_%d.txt", time.Now().Unix())
	fileData := make([]byte, 1024)
	if _, err := rand.Read(fileData); err != nil {
		return fmt.Errorf("生成随机数据失败: %w", err)
	}
	fmt.Printf("   创建临时测试文件: %s (%d bytes)\n", fileName, len(fileData))

	// 2. 计算 MD5
	hash := md5.Sum(fileData)
	fileMd5 := hex.EncodeToString(hash[:])
	sliceMd5Base64 := base64.StdEncoding.EncodeToString(hash[:]) // 分片名称使用 Base64 编码
	fmt.Printf("   文件 MD5: %s\n", fileMd5)

	// Web 端需要先获取 RSA 公钥和 SessionKey
	var rsaKey *cloud189.WebRSA
	if isWebLogin {
		fmt.Println("   获取 Web SessionKey...")
		if _, err := apiClient.WebSession(ctx); err != nil {
			return fmt.Errorf("获取 SessionKey 失败: %w", err)
		}
		fmt.Println("   获取 RSA 公钥...")
		var err error
		rsaKey, err = apiClient.FetchWebRSA(ctx)
		if err != nil {
			return fmt.Errorf("获取 RSA 公钥失败: %w", err)
		}
		fmt.Printf("   RSA PkId: %s\n", rsaKey.PkId)
	}

	// 3. 初始化上传
	fmt.Println("   初始化上传...")
	initParams := url.Values{}
	initParams.Set("parentFolderId", parentFolderId)
	initParams.Set("fileName", fileName)
	initParams.Set("fileSize", strconv.Itoa(len(fileData)))
	initParams.Set("sliceSize", "10485760") // 10MB
	initParams.Set("lazyCheck", "1")
	initParams.Set("extend", `{"opScene":"1","relativepath":"","rootfolderid":""}`)

	var initResp initUploadResp
	var err error
	if isWebLogin {
		err = apiClient.WebUpload(ctx, "/person/initMultiUpload", initParams, rsaKey, &initResp)
	} else {
		err = apiClient.AppUpload(ctx, "/person/initMultiUpload", initParams, &initResp)
	}
	if err != nil {
		return fmt.Errorf("初始化上传失败: %w", err)
	}
	if initResp.Data.UploadFileId == "" {
		return fmt.Errorf("初始化上传失败: 未获取到 uploadFileId, code=%s", initResp.Code)
	}
	uploadFileId := initResp.Data.UploadFileId
	fmt.Printf("   uploadFileId: %s\n", uploadFileId)

	var uploadedFileId string

	// 4. 检查是否秒传
	if initResp.Data.FileDataExists == 1 {
		fmt.Println("   文件已存在（秒传）")
	} else {
		// 5. 获取上传 URL
		fmt.Println("   获取上传 URL...")
		partName := fmt.Sprintf("1-%s", sliceMd5Base64)
		urlParams := url.Values{}
		urlParams.Set("uploadFileId", uploadFileId)
		urlParams.Set("partInfo", partName)

		var urlResp uploadUrlResp
		if isWebLogin {
			err = apiClient.WebUpload(ctx, "/person/getMultiUploadUrls", urlParams, rsaKey, &urlResp)
		} else {
			err = apiClient.AppUpload(ctx, "/person/getMultiUploadUrls", urlParams, &urlResp)
		}
		if err != nil {
			return fmt.Errorf("获取上传 URL 失败: %w", err)
		}
		printJSON("   上传URL响应", urlResp)

		partKey := "partNumber_1"
		partInfo, ok := urlResp.UploadUrls[partKey]
		if !ok {
			return fmt.Errorf("未找到分片上传信息: %s", partKey)
		}

		// 6. 上传分片
		fmt.Println("   上传分片 1/1...")
		fmt.Printf("   RequestURL: %s\n", partInfo.RequestURL)
		fmt.Printf("   RequestHeader: %s\n", partInfo.RequestHeader)
		req, err := http.NewRequestWithContext(ctx, http.MethodPut, partInfo.RequestURL, bytes.NewReader(fileData))
		if err != nil {
			return fmt.Errorf("创建上传请求失败: %w", err)
		}

		// 解析并设置请求头（直接使用服务器返回的值）
		headers := strings.Split(partInfo.RequestHeader, "&")
		for _, h := range headers {
			idx := strings.Index(h, "=")
			if idx > 0 {
				req.Header.Set(h[:idx], h[idx+1:])
			}
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return fmt.Errorf("上传分片失败: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("上传分片失败: HTTP %d, body: %s", resp.StatusCode, string(body))
		}
		fmt.Println("   分片上传成功")
	}

	// 7. 提交上传
	fmt.Println("   提交上传...")
	commitParams := url.Values{}
	commitParams.Set("uploadFileId", uploadFileId)
	commitParams.Set("fileMd5", fileMd5)
	commitParams.Set("sliceMd5", fileMd5) // 单分片时 sliceMd5 = fileMd5
	commitParams.Set("lazyCheck", "1")

	var commitResult commitResp
	if isWebLogin {
		err = apiClient.WebUpload(ctx, "/person/commitMultiUploadFile", commitParams, rsaKey, &commitResult)
	} else {
		err = apiClient.AppUpload(ctx, "/person/commitMultiUploadFile", commitParams, &commitResult)
	}
	if err != nil {
		return fmt.Errorf("提交上传失败: %w", err)
	}
	uploadedFileId = commitResult.File.Id
	fmt.Printf("   上传成功，文件ID: %s\n", uploadedFileId)

	// 8. 清理云端测试文件
	if uploadedFileId != "" {
		fmt.Println("   清理云端测试文件...")
		var deleteResult map[string]any
		if isWebLogin {
			taskInfo := fmt.Sprintf(`[{"fileId":"%s","fileName":"%s","isFolder":0}]`, uploadedFileId, fileName)
			deleteParams := map[string]string{
				"type":           "DELETE",
				"taskInfos":      taskInfo,
				"targetFolderId": "",
			}
			_ = apiClient.WebPost(ctx, "/open/batch/createBatchTask.action", deleteParams, &deleteResult)
		} else {
			deleteParams := map[string]string{"fileIdList": uploadedFileId}
			_ = apiClient.AppPost(ctx, "/batchDeleteFile.action", deleteParams, &deleteResult)
		}
		fmt.Println("   云端文件已清理")
	}

	return nil
}
