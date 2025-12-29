package main

import (
	"github.com/dnslin/cloud189-desktop/core/auth"
	"github.com/dnslin/cloud189-desktop/core/model"
)

type LoginSubmittedMsg struct {
	Username string
	Password string
}

type LoginSuccessMsg struct {
	Session  *auth.Session
	Username string // 登录用户名
}

type LoginErrorMsg struct {
	Err error
}

// UserInfoMsg 用户信息加载完成消息
type UserInfoMsg struct {
	User *model.User
	Err  error
}

type ViewID string

const (
	ViewLogin   ViewID = "login"
	ViewBrowser ViewID = "browser"
)

type SwitchViewMsg struct {
	View ViewID
}
