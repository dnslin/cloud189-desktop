package login

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/dnslin/cloud189-desktop/cmd/tui/ui/common"
)

type SubmitFunc func(username, password string) tea.Cmd

type Model struct {
	usernameInput textinput.Model
	passwordInput textinput.Model
	focusIndex    int
	errMsg        string
	isSubmitting  bool
	submit        SubmitFunc
}

func NewModel(submit SubmitFunc) Model {
	username := textinput.New()
	username.Placeholder = "用户名"
	username.Prompt = ""
	username.CharLimit = 64
	username.Width = 32
	username.Focus()
	username.TextStyle = common.InputTextStyle
	username.PlaceholderStyle = common.InputPlaceholderStyle
	username.CursorStyle = common.InputTextStyle

	password := textinput.New()
	password.Placeholder = "密码"
	password.Prompt = ""
	password.CharLimit = 64
	password.Width = 32
	password.EchoMode = textinput.EchoPassword
	password.EchoCharacter = '*'
	password.TextStyle = common.InputTextStyle
	password.PlaceholderStyle = common.InputPlaceholderStyle
	password.CursorStyle = common.InputTextStyle

	return Model{
		usernameInput: username,
		passwordInput: password,
		focusIndex:    0,
		submit:        submit,
	}
}

func (m Model) Init() tea.Cmd {
	return textinput.Blink
}

func (m Model) SetError(err error) Model {
	m.isSubmitting = false
	if err == nil {
		m.errMsg = ""
		return m
	}
	m.errMsg = "登录失败：" + err.Error()
	return m
}

func (m Model) SetSubmitting(isSubmitting bool) Model {
	m.isSubmitting = isSubmitting
	return m
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "tab", "shift+tab":
			m = m.toggleFocus()
			return m, nil
		case "enter":
			if m.isSubmitting {
				return m, nil
			}
			m.errMsg = ""
			if m.submit != nil {
				m.isSubmitting = true
				return m, m.submit(m.usernameInput.Value(), m.passwordInput.Value())
			}
			return m, nil
		case "esc":
			return m, tea.Quit
		default:
			if m.errMsg != "" {
				m.errMsg = ""
			}
		}
	}

	var cmd tea.Cmd
	if m.focusIndex == 0 {
		m.usernameInput, cmd = m.usernameInput.Update(msg)
	} else {
		m.passwordInput, cmd = m.passwordInput.Update(msg)
	}

	return m, cmd
}

func (m Model) View() string {
	usernameView := m.renderInput(0)
	passwordView := m.renderInput(1)

	var builder strings.Builder
	builder.WriteString(common.TitleStyle.Render("账号登录"))
	builder.WriteString("\n\n")
	builder.WriteString(common.MutedTextStyle.Render("用户名"))
	builder.WriteString("\n")
	builder.WriteString(usernameView)
	builder.WriteString("\n\n")
	builder.WriteString(common.MutedTextStyle.Render("密码"))
	builder.WriteString("\n")
	builder.WriteString(passwordView)

	if m.errMsg != "" {
		builder.WriteString("\n\n")
		builder.WriteString(common.ErrorTextStyle.Render(m.errMsg))
	}
	if m.isSubmitting {
		builder.WriteString("\n\n")
		builder.WriteString(common.MutedTextStyle.Render("登录中..."))
	}

	builder.WriteString("\n\n")
	builder.WriteString(common.MutedTextStyle.Render("Tab 切换，Enter 登录，Esc 退出"))

	return common.PanelStyle.Render(builder.String())
}

func (m Model) toggleFocus() Model {
	if m.focusIndex == 0 {
		m.focusIndex = 1
	} else {
		m.focusIndex = 0
	}
	return m.applyFocus()
}

func (m Model) applyFocus() Model {
	if m.focusIndex == 0 {
		m.usernameInput.Focus()
		m.passwordInput.Blur()
		return m
	}
	m.usernameInput.Blur()
	m.passwordInput.Focus()
	return m
}

func (m Model) renderInput(index int) string {
	var value string
	if index == 0 {
		value = m.usernameInput.View()
	} else {
		value = m.passwordInput.View()
	}
	if index == m.focusIndex {
		return common.InputFocusedStyle.Render(value)
	}
	return common.InputStyle.Render(value)
}
