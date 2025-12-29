package main

import (
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"os"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/dnslin/cloud189-desktop/cmd/tui/service"
	"github.com/dnslin/cloud189-desktop/core/auth"
	"github.com/dnslin/cloud189-desktop/core/httpclient"
)

const dataDirName = ".cloud189-tui"

func main() {
	dataDir, err := initDataDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "初始化数据目录失败：%v\n", err)
		os.Exit(1)
	}

	logger := NewTUILogger()

	jar, err := cookiejar.New(nil)
	if err != nil {
		logger.Errorf("创建 CookieJar 失败：%v", err)
		os.Exit(1)
	}

	rawHTTP := &http.Client{
		Jar:     jar,
		Timeout: 60 * time.Second,
	}

	httpClient := httpclient.NewClient(
		httpclient.WithHTTPClient(rawHTTP),
		httpclient.WithCookieJar(jar),
		httpclient.WithLogger(logger),
	)

	loginClient := auth.NewLoginClient(httpClient, auth.WithLoginLogger(logger))
	authManager := auth.NewAuthManager()
	authService := service.NewAuthService(authManager, loginClient, httpClient, dataDir)

	logger.Debugf("数据目录已就绪：%s", dataDir)
	logger.Debugf("HTTP 客户端初始化完成，超时：%s", rawHTTP.Timeout)
	logger.Debugf("登录客户端与认证管理器已创建，等待后续流程")

	app := NewApp(authService)
	program := tea.NewProgram(app, tea.WithAltScreen())
	if _, err := program.Run(); err != nil {
		logger.Errorf("启动 TUI 失败：%v", err)
		os.Exit(1)
	}
}

// initDataDir 确保数据目录存在。
func initDataDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("获取用户目录失败：%w", err)
	}
	dataDir := filepath.Join(homeDir, dataDirName)
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return "", fmt.Errorf("创建数据目录失败：%w", err)
	}
	return dataDir, nil
}
