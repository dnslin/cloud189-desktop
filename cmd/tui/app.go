package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/dnslin/cloud189-desktop/cmd/tui/service"
	"github.com/dnslin/cloud189-desktop/cmd/tui/ui/common"
	"github.com/dnslin/cloud189-desktop/cmd/tui/ui/login"
	"github.com/dnslin/cloud189-desktop/core/model"
)

type AppState int

const (
	StateLogin AppState = iota
	StateBrowser
)

type Model struct {
	state       AppState
	loginModel  login.Model
	authService *service.AuthService
	userInfo    *model.User // 用户信息
	userInfoErr error       // 用户信息获取错误
}

func NewApp(authService *service.AuthService) Model {
	submit := func(username, password string) tea.Cmd {
		return loginCmd(authService, username, password)
	}

	return Model{
		state:       StateLogin,
		loginModel:  login.NewModel(submit),
		authService: authService,
	}
}

func (m Model) Init() tea.Cmd {
	if m.state == StateLogin {
		return m.loginModel.Init()
	}
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "r":
			if m.state == StateBrowser {
				return m, m.fetchUserInfoCmd()
			}
		}
	case LoginSuccessMsg:
		m.loginModel = m.loginModel.SetSubmitting(false)
		m.state = StateBrowser
		m.userInfo = nil
		m.userInfoErr = nil
		// 登录成功后获取用户信息
		return m, m.fetchUserInfoCmd()
	case UserInfoMsg:
		if msg.Err != nil {
			m.userInfoErr = msg.Err
			return m, nil
		}
		m.userInfoErr = nil
		m.userInfo = msg.User
		return m, nil
	case LoginErrorMsg:
		m.loginModel = m.loginModel.SetError(msg.Err)
		return m, nil
	case SwitchViewMsg:
		m = m.switchView(msg.View)
		return m, nil
	}

	switch m.state {
	case StateLogin:
		updated, cmd := m.loginModel.Update(msg)
		if model, ok := updated.(login.Model); ok {
			m.loginModel = model
		}
		return m, cmd
	case StateBrowser:
		return m, nil
	default:
		return m, nil
	}
}

func (m Model) View() string {
	switch m.state {
	case StateLogin:
		return m.loginModel.View()
	case StateBrowser:
		return m.renderBrowserPlaceholder()
	default:
		return ""
	}
}

func (m Model) switchView(view ViewID) Model {
	switch view {
	case ViewLogin:
		m.state = StateLogin
	case ViewBrowser:
		m.state = StateBrowser
	}
	return m
}

func (m Model) renderBrowserPlaceholder() string {
	var builder strings.Builder
	// 顶栏：标题 + 用户信息
	title := common.TitleStyle.Render("天翼云盘")
	if m.userInfo != nil {
		name := m.userInfo.NickName
		if name == "" {
			name = m.userInfo.LoginName
		}
		if name == "" {
			name = m.userInfo.Name
		}
		title += common.MutedTextStyle.Render(" │ 用户: " + name)
		// 显示容量
		used := formatSize(m.userInfo.Quota.Used)
		total := formatSize(m.userInfo.Quota.Capacity)
		title += common.MutedTextStyle.Render(fmt.Sprintf(" │ 容量: %s/%s", used, total))
	}
	builder.WriteString(title)
	builder.WriteString("\n")
	builder.WriteString(common.MutedTextStyle.Render("─────────────────────────────────────────"))
	builder.WriteString("\n\n")
	if m.userInfoErr != nil {
		builder.WriteString(common.ErrorTextStyle.Render("用户信息获取失败: " + m.userInfoErr.Error()))
		builder.WriteString("\n")
		builder.WriteString(common.MutedTextStyle.Render("按 r 重试获取用户信息"))
		builder.WriteString("\n\n")
	}
	builder.WriteString(common.MutedTextStyle.Render("文件浏览器 - 待实现"))
	builder.WriteString("\n\n")
	builder.WriteString(common.MutedTextStyle.Render("按 q 退出"))
	return common.PanelStyle.Render(builder.String())
}

func (m Model) fetchUserInfoCmd() tea.Cmd {
	return func() tea.Msg {
		if m.authService == nil {
			return UserInfoMsg{Err: errors.New("认证服务未初始化")}
		}
		user, err := m.authService.GetUserInfo(context.Background())
		return UserInfoMsg{User: user, Err: err}
	}
}

func formatSize(bytes uint64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
		TB = GB * 1024
	)
	switch {
	case bytes >= TB:
		return fmt.Sprintf("%.1fTB", float64(bytes)/TB)
	case bytes >= GB:
		return fmt.Sprintf("%.1fGB", float64(bytes)/GB)
	case bytes >= MB:
		return fmt.Sprintf("%.1fMB", float64(bytes)/MB)
	case bytes >= KB:
		return fmt.Sprintf("%.1fKB", float64(bytes)/KB)
	default:
		return fmt.Sprintf("%dB", bytes)
	}
}

func loginCmd(authService *service.AuthService, username, password string) tea.Cmd {
	return func() tea.Msg {
		if authService == nil {
			return LoginErrorMsg{Err: errors.New("认证服务未初始化")}
		}
		session, err := authService.Login(context.Background(), username, password)
		if err != nil {
			return LoginErrorMsg{Err: err}
		}
		return LoginSuccessMsg{Session: session, Username: username}
	}
}
