# 天翼云盘 TUI 开发方案

## 一、项目概述

### 1.1 目标

基于 Bubble Tea + Bubbles + Lip Gloss 框架开发天翼云盘 TUI 客户端，实现：
- 双栏布局（左侧文件列表 + 右侧详情面板）
- 底部固定任务面板显示传输进度
- 完整文件管理功能（浏览、上传、下载、删除、重命名、复制、移动）
- 多账号管理

### 1.2 技术栈

| 组件 | 用途 | 版本 |
|------|------|------|
| Bubble Tea | TUI 框架（MVU 架构） | latest |
| Bubbles | 预制 UI 组件库 | latest |
| Lip Gloss | 样式定义（类 CSS） | latest |
| Core | 业务逻辑层（已完成） | - |

### 1.3 架构原则

- **Core 三无原则**：TUI 不修改 core，只调用
- **依赖方向**：`cmd/tui` → `core/**`，禁止反向依赖
- **存储注入**：存储接口实现放在 `cmd/tui/store/`

---

## 二、Bubble Tea 框架知识

### 2.1 MVU 架构（Model-View-Update）

Bubble Tea 采用 Elm 架构，程序由三个核心部分组成：

```go
// Model - 应用状态
type Model struct {
    cursor int
    items  []string
}

// Init - 初始化命令
func (m Model) Init() tea.Cmd {
    return nil // 返回初始命令，nil 表示无命令
}

// Update - 处理消息，更新状态
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case tea.KeyMsg:
        switch msg.String() {
        case "q", "ctrl+c":
            return m, tea.Quit
        case "up", "k":
            if m.cursor > 0 {
                m.cursor--
            }
        case "down", "j":
            if m.cursor < len(m.items)-1 {
                m.cursor++
            }
        }
    }
    return m, nil
}

// View - 渲染 UI
func (m Model) View() string {
    s := "选择一个选项:\n\n"
    for i, item := range m.items {
        cursor := " "
        if m.cursor == i {
            cursor = ">"
        }
        s += fmt.Sprintf("%s %s\n", cursor, item)
    }
    return s
}
```

### 2.2 消息（Msg）与命令（Cmd）

**消息类型**：
```go
// 内置消息
tea.KeyMsg        // 键盘输入
tea.MouseMsg      // 鼠标事件
tea.WindowSizeMsg // 窗口大小变化
tea.QuitMsg       // 退出信号

// 自定义消息
type FilesLoadedMsg struct {
    Files []FileInfo
    Err   error
}

type LoginSuccessMsg struct {
    AccountID string
}
```

**命令（异步操作）**：
```go
// Cmd 是返回 Msg 的函数
type Cmd func() Msg

// 创建命令
func loadFiles(folderID string) tea.Cmd {
    return func() tea.Msg {
        files, err := client.ListFiles(ctx, folderID)
        return FilesLoadedMsg{Files: files, Err: err}
    }
}

// 批量执行命令（并发）
tea.Batch(cmd1, cmd2, cmd3)

// 顺序执行命令
tea.Sequence(cmd1, cmd2, cmd3)

// 定时器
tea.Tick(time.Second, func(t time.Time) tea.Msg {
    return TickMsg(t)
})
```

### 2.3 程序启动

```go
func main() {
    // 创建程序
    p := tea.NewProgram(
        initialModel(),
        tea.WithAltScreen(),       // 使用备用屏幕
        tea.WithMouseCellMotion(), // 启用鼠标支持
    )

    // 运行
    if _, err := p.Run(); err != nil {
        fmt.Printf("Error: %v", err)
        os.Exit(1)
    }
}
```

### 2.4 外部消息注入

```go
// 从外部向程序发送消息
p := tea.NewProgram(model)
go func() {
    // 任务进度回调
    taskManager.Subscribe(func(task *Task) {
        p.Send(TaskProgressMsg{Task: task})
    })
}()
p.Run()
```

---

## 三、Bubbles 组件库

### 3.1 textinput - 文本输入

```go
import "github.com/charmbracelet/bubbles/textinput"

type Model struct {
    usernameInput textinput.Model
    passwordInput textinput.Model
    focusIndex    int
}

func NewModel() Model {
    username := textinput.New()
    username.Placeholder = "用户名"
    username.Focus()

    password := textinput.New()
    password.Placeholder = "密码"
    password.EchoMode = textinput.EchoPassword // 密码模式
    password.EchoCharacter = '•'

    return Model{
        usernameInput: username,
        passwordInput: password,
    }
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case tea.KeyMsg:
        switch msg.String() {
        case "tab", "shift+tab":
            // 切换焦点
            if m.focusIndex == 0 {
                m.usernameInput.Blur()
                m.passwordInput.Focus()
                m.focusIndex = 1
            } else {
                m.passwordInput.Blur()
                m.usernameInput.Focus()
                m.focusIndex = 0
            }
        case "enter":
            // 提交登录
            return m, doLogin(m.usernameInput.Value(), m.passwordInput.Value())
        }
    }

    // 更新当前焦点的输入框
    var cmd tea.Cmd
    if m.focusIndex == 0 {
        m.usernameInput, cmd = m.usernameInput.Update(msg)
    } else {
        m.passwordInput, cmd = m.passwordInput.Update(msg)
    }
    return m, cmd
}
```

### 3.2 list - 列表组件

```go
import "github.com/charmbracelet/bubbles/list"

// 实现 list.Item 接口
type FileItem struct {
    id       string
    name     string
    size     int64
    isFolder bool
}

func (f FileItem) Title() string       { return f.name }
func (f FileItem) Description() string { return formatSize(f.size) }
func (f FileItem) FilterValue() string { return f.name }

// 创建列表
func NewFileList(width, height int) list.Model {
    delegate := list.NewDefaultDelegate()
    l := list.New([]list.Item{}, delegate, width, height)
    l.Title = "文件列表"
    l.SetFilteringEnabled(true)  // 启用过滤
    l.SetShowStatusBar(true)
    l.SetShowHelp(true)
    return l
}

// 更新列表项
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case FilesLoadedMsg:
        items := make([]list.Item, len(msg.Files))
        for i, f := range msg.Files {
            items[i] = FileItem{id: f.ID, name: f.Name, size: f.Size, isFolder: f.IsFolder}
        }
        m.list.SetItems(items)
    case tea.KeyMsg:
        switch msg.String() {
        case "enter":
            if item, ok := m.list.SelectedItem().(FileItem); ok {
                if item.isFolder {
                    return m, loadFiles(item.id)
                }
            }
        }
    }
    var cmd tea.Cmd
    m.list, cmd = m.list.Update(msg)
    return m, cmd
}
```

### 3.3 progress - 进度条

```go
import "github.com/charmbracelet/bubbles/progress"

type Model struct {
    progress progress.Model
    percent  float64
}

func NewModel() Model {
    return Model{
        progress: progress.New(
            progress.WithDefaultGradient(),  // 渐变色
            progress.WithWidth(40),
        ),
    }
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case TaskProgressMsg:
        m.percent = float64(msg.Task.Progress) / float64(msg.Task.Total)
        cmd := m.progress.SetPercent(m.percent)
        return m, cmd
    case progress.FrameMsg:
        progressModel, cmd := m.progress.Update(msg)
        m.progress = progressModel.(progress.Model)
        return m, cmd
    }
    return m, nil
}

func (m Model) View() string {
    return fmt.Sprintf("下载中... %s %.1f%%", m.progress.View(), m.percent*100)
}
```

### 3.4 spinner - 加载指示器

```go
import "github.com/charmbracelet/bubbles/spinner"

type Model struct {
    spinner spinner.Model
    loading bool
}

func NewModel() Model {
    s := spinner.New()
    s.Spinner = spinner.Dot  // 可选: Line, Dot, MiniDot, Jump, Pulse, Points
    s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
    return Model{spinner: s, loading: true}
}

func (m Model) Init() tea.Cmd {
    return m.spinner.Tick
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case spinner.TickMsg:
        var cmd tea.Cmd
        m.spinner, cmd = m.spinner.Update(msg)
        return m, cmd
    }
    return m, nil
}

func (m Model) View() string {
    if m.loading {
        return fmt.Sprintf("%s 加载中...", m.spinner.View())
    }
    return "加载完成"
}
```

### 3.5 table - 表格组件

```go
import "github.com/charmbracelet/bubbles/table"

func NewFileTable() table.Model {
    columns := []table.Column{
        {Title: "名称", Width: 30},
        {Title: "大小", Width: 10},
        {Title: "修改时间", Width: 20},
    }

    rows := []table.Row{
        {"文件夹1", "-", "2024-01-01"},
        {"文件.txt", "10KB", "2024-01-02"},
    }

    t := table.New(
        table.WithColumns(columns),
        table.WithRows(rows),
        table.WithFocused(true),
        table.WithHeight(10),
    )

    // 设置样式
    s := table.DefaultStyles()
    s.Header = s.Header.
        BorderStyle(lipgloss.NormalBorder()).
        BorderForeground(lipgloss.Color("240")).
        BorderBottom(true).
        Bold(false)
    s.Selected = s.Selected.
        Foreground(lipgloss.Color("229")).
        Background(lipgloss.Color("57")).
        Bold(false)
    t.SetStyles(s)

    return t
}
```

### 3.6 filepicker - 文件选择器

```go
import "github.com/charmbracelet/bubbles/filepicker"

type Model struct {
    filepicker filepicker.Model
}

func NewModel() Model {
    fp := filepicker.New()
    fp.AllowedTypes = []string{".txt", ".pdf", ".zip"}  // 限制文件类型
    fp.CurrentDirectory = "."
    return Model{filepicker: fp}
}

func (m Model) Init() tea.Cmd {
    return m.filepicker.Init()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    var cmd tea.Cmd
    m.filepicker, cmd = m.filepicker.Update(msg)

    // 检查是否选择了文件
    if didSelect, path := m.filepicker.DidSelectFile(msg); didSelect {
        return m, startUpload(path)
    }

    return m, cmd
}
```

### 3.7 help - 帮助组件

```go
import (
    "github.com/charmbracelet/bubbles/help"
    "github.com/charmbracelet/bubbles/key"
)

// 定义快捷键
type keyMap struct {
    Up     key.Binding
    Down   key.Binding
    Enter  key.Binding
    Delete key.Binding
    Quit   key.Binding
}

func (k keyMap) ShortHelp() []key.Binding {
    return []key.Binding{k.Enter, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
    return [][]key.Binding{
        {k.Up, k.Down, k.Enter},
        {k.Delete, k.Quit},
    }
}

var keys = keyMap{
    Up:     key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "上移")),
    Down:   key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "下移")),
    Enter:  key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "确认")),
    Delete: key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "删除")),
    Quit:   key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "退出")),
}

type Model struct {
    help help.Model
    keys keyMap
}

func (m Model) View() string {
    return m.help.View(m.keys)
}
```

### 3.8 viewport - 滚动视图

```go
import "github.com/charmbracelet/bubbles/viewport"

type Model struct {
    viewport viewport.Model
    ready    bool
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case tea.WindowSizeMsg:
        if !m.ready {
            m.viewport = viewport.New(msg.Width, msg.Height-4)
            m.viewport.SetContent("长文本内容...")
            m.ready = true
        } else {
            m.viewport.Width = msg.Width
            m.viewport.Height = msg.Height - 4
        }
    }
    var cmd tea.Cmd
    m.viewport, cmd = m.viewport.Update(msg)
    return m, cmd
}
```

---

## 四、Lip Gloss 样式系统

### 4.1 基础样式

```go
import "github.com/charmbracelet/lipgloss"

// 创建样式
var style = lipgloss.NewStyle().
    Bold(true).
    Italic(true).
    Foreground(lipgloss.Color("#FAFAFA")).
    Background(lipgloss.Color("#7D56F4")).
    PaddingTop(2).
    PaddingLeft(4).
    Width(22)

// 渲染文本
output := style.Render("Hello, World")
```

### 4.2 颜色系统

```go
// ANSI 256 色
lipgloss.Color("205")      // 粉色
lipgloss.Color("63")       // 蓝色

// 十六进制
lipgloss.Color("#FF0000")  // 红色
lipgloss.Color("#7D56F4")  // 紫色

// 自适应颜色（深色/浅色终端）
lipgloss.AdaptiveColor{Light: "236", Dark: "248"}
```

### 4.3 边框样式

```go
// 预定义边框
lipgloss.NormalBorder()   // 普通边框 ┌─┐
lipgloss.RoundedBorder()  // 圆角边框 ╭─╮
lipgloss.ThickBorder()    // 粗边框
lipgloss.DoubleBorder()   // 双线边框 ╔═╗
lipgloss.BlockBorder()    // 块边框
lipgloss.ASCIIBorder()    // ASCII 边框 +-+

// 应用边框
var boxStyle = lipgloss.NewStyle().
    BorderStyle(lipgloss.RoundedBorder()).
    BorderForeground(lipgloss.Color("62")).
    Padding(1, 2)

// 部分边框
lipgloss.NewStyle().
    Border(lipgloss.NormalBorder(), true, false, true, false) // 上、右、下、左
```

### 4.4 布局函数

```go
// 水平拼接
lipgloss.JoinHorizontal(lipgloss.Top, left, right)
lipgloss.JoinHorizontal(lipgloss.Center, left, right)
lipgloss.JoinHorizontal(lipgloss.Bottom, left, right)

// 垂直拼接
lipgloss.JoinVertical(lipgloss.Left, top, bottom)
lipgloss.JoinVertical(lipgloss.Center, top, bottom)
lipgloss.JoinVertical(lipgloss.Right, top, bottom)

// 放置在指定位置
lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, content)
```

### 4.5 TUI 样式定义示例

```go
// ui/common/styles.go
package common

import "github.com/charmbracelet/lipgloss"

var (
    // 颜色定义
    Primary   = lipgloss.Color("62")   // 主色
    Secondary = lipgloss.Color("240")  // 次要色
    Success   = lipgloss.Color("42")   // 成功
    Warning   = lipgloss.Color("214")  // 警告
    Error     = lipgloss.Color("196")  // 错误
    Muted     = lipgloss.Color("245")  // 灰色

    // 面板样式
    FocusedPanel = lipgloss.NewStyle().
        BorderStyle(lipgloss.RoundedBorder()).
        BorderForeground(Primary).
        Padding(0, 1)

    BlurredPanel = lipgloss.NewStyle().
        BorderStyle(lipgloss.RoundedBorder()).
        BorderForeground(Secondary).
        Padding(0, 1)

    // 文件样式
    FolderStyle = lipgloss.NewStyle().
        Foreground(lipgloss.Color("33")).
        Bold(true)

    FileStyle = lipgloss.NewStyle().
        Foreground(lipgloss.Color("252"))

    SelectedStyle = lipgloss.NewStyle().
        Background(lipgloss.Color("57")).
        Foreground(lipgloss.Color("229"))

    // 状态栏
    StatusBar = lipgloss.NewStyle().
        Background(lipgloss.Color("236")).
        Foreground(lipgloss.Color("252")).
        Padding(0, 1)

    // 帮助栏
    HelpBar = lipgloss.NewStyle().
        Foreground(Muted)

    // 标题
    Title = lipgloss.NewStyle().
        Bold(true).
        Foreground(Primary).
        Padding(0, 1)
)
```

---

## 五、项目目录结构

```
cmd/tui/
├── main.go                    # 入口，初始化依赖并启动 TUI
├── app.go                     # 根 Model，组合所有子组件
├── messages.go                # 自定义消息类型定义
├── store/                     # 存储实现（实现 core/store 接口）
│   ├── session.go             # SessionStore[*auth.Session] - JSON 文件
│   ├── config.go              # ConfigStore[Config] - 配置存储
│   └── upload_state.go        # UploadStateStore - 断点续传
├── service/                   # 业务服务层（封装 Core 调用）
│   ├── auth.go                # 登录/会话管理
│   ├── file.go                # 文件操作
│   └── transfer.go            # 传输服务（Uploader/Downloader 实现）
├── ui/                        # UI 组件
│   ├── login/                 # 登录视图
│   │   └── login.go           # 登录表单 Model
│   ├── browser/               # 文件浏览器
│   │   ├── browser.go         # 双栏布局主 Model
│   │   ├── filelist.go        # 左侧文件列表组件
│   │   └── detail.go          # 右侧详情面板组件
│   ├── task/                  # 任务面板
│   │   └── panel.go           # 底部任务进度面板
│   ├── account/               # 账号管理
│   │   └── switcher.go        # 多账号切换组件
│   ├── dialog/                # 对话框
│   │   ├── confirm.go         # 确认对话框
│   │   └── input.go           # 输入对话框
│   └── common/                # 通用组件
│       ├── styles.go          # Lip Gloss 样式定义
│       └── utils.go           # 工具函数
└── logger.go                  # httpclient.Logger 实现
```

---

## 六、界面布局设计

### 6.1 整体布局

```
┌─────────────────────────────────────────────────────────────────┐
│ 天翼云盘 │ 用户: xxx │ 容量: 50GB/100GB │ [Tab 切换账号]        │ <- 顶栏 (1行)
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  ┌──────────────────────────┐  ┌──────────────────────────────┐ │
│  │ 📁 ..                    │  │ 文件详情                     │ │
│  │ 📁 文件夹1               │  │ ─────────────────────────    │ │
│  │ 📁 文件夹2               │  │ 名称: 文件1.txt              │ │
│  │ 📄 文件1.txt        10KB │  │ 大小: 10.5 KB                │ │
│  │ 📄 文件2.pdf        2MB  │  │ 类型: 文本文件               │ │
│  │ > 📄 文件3.mp4     100MB │  │ 修改: 2024-01-15 10:30       │ │
│  │                          │  │ MD5: abc123def456...         │ │
│  │                          │  │                              │ │
│  │                          │  │ ─────────────────────────    │ │
│  │                          │  │ [d]下载 [D]删除 [r]重命名    │ │
│  └──────────────────────────┘  └──────────────────────────────┘ │
│  左侧文件列表 (60%)              右侧详情面板 (40%)              │
│                                                                 │
├─────────────────────────────────────────────────────────────────┤
│ 传输任务 (2/5)                                        [p]暂停全部│ <- 任务面板 (6行)
│ ↑ 上传: file1.zip    45%  ████████░░░░░░░░  2.5MB/s  剩余 1:30 │
│ ↓ 下载: video.mp4    78%  ████████████░░░░  5.0MB/s  剩余 0:45 │
│ ↑ 等待: file2.doc    0%   ░░░░░░░░░░░░░░░░  等待中             │
├─────────────────────────────────────────────────────────────────┤
│ [h]帮助 [q]退出 [u]上传 [n]新建 [r]刷新 [/]搜索 [Tab]切换面板  │ <- 底栏 (1行)
└─────────────────────────────────────────────────────────────────┘
```

### 6.2 高度分配

```go
const (
    TopBarHeight    = 1  // 顶栏
    TaskPanelHeight = 6  // 任务面板（可折叠）
    HelpBarHeight   = 1  // 底栏
)

// 主区域高度 = 总高度 - 顶栏 - 任务面板 - 底栏
mainHeight := windowHeight - TopBarHeight - TaskPanelHeight - HelpBarHeight
```

### 6.3 宽度分配

```go
// 双栏布局
leftWidth := windowWidth * 60 / 100   // 60%
rightWidth := windowWidth - leftWidth  // 40%
```

---

## 七、Core 接口对接

### 7.1 需要实现的存储接口

```go
// core/store/store.go 定义的接口

// SessionStore - 会话存储
type SessionStore[T any] interface {
    SaveSession(session T) error
    LoadSession() (T, error)
    ClearSession() error
}

// ConfigStore - 配置存储
type ConfigStore[T any] interface {
    SaveConfig(cfg T) error
    LoadConfig() (T, error)
    ClearConfig() error
}

// UploadStateStore - 上传断点续传
type UploadStateStore interface {
    SaveState(localPath string, state *UploadState) error
    LoadState(localPath string) (*UploadState, error)
    DeleteState(localPath string) error
}
```

### 7.2 需要实现的任务接口

```go
// core/task/upload.go

type Uploader interface {
    InitUpload(ctx context.Context, parentID, filename string, size int64, resumeState *ResumeState) (uploadFileID string, exists bool, uploadedSize int64, err error)
    UploadPart(ctx context.Context, uploadFileID string, partNum int, data io.Reader) error
    CommitUpload(ctx context.Context, uploadFileID string, fileMD5, sliceMD5 string) (fileID string, err error)
    Mode() UploadMode
    GetPartHashes() []string
}

// core/task/download.go

type Downloader interface {
    GetDownloadURL(ctx context.Context, fileID string) (string, error)
    GetFileInfo(ctx context.Context, fileID string) (fileName string, fileSize int64, err error)
    HTTPClient() *http.Client
    Mode() DownloadMode
}
```

### 7.3 初始化流程（参考 cmd/apitest/main.go）

```go
// main.go
func main() {
    // 1. 创建数据目录
    dataDir := filepath.Join(os.UserHomeDir(), ".cloud189")
    os.MkdirAll(dataDir, 0700)

    // 2. 创建存储实现
    sessionStore := store.NewJSONSessionStore(filepath.Join(dataDir, "session.json"))
    configStore := store.NewJSONConfigStore(filepath.Join(dataDir, "config.json"))
    uploadStateStore := store.NewJSONUploadStateStore(filepath.Join(dataDir, "uploads"))

    // 3. 创建 HTTP 客户端
    jar, _ := cookiejar.New(nil)
    rawHTTP := &http.Client{Jar: jar, Timeout: 60 * time.Second}
    logger := NewTUILogger()
    httpClient := httpclient.NewClient(
        httpclient.WithHTTPClient(rawHTTP),
        httpclient.WithCookieJar(jar),
        httpclient.WithLogger(logger),
    )

    // 4. 创建登录客户端
    loginClient := auth.NewLoginClient(httpClient, auth.WithLoginLogger(logger))

    // 5. 创建 AuthManager
    authMgr := auth.NewAuthManager()

    // 6. 创建 Cloud189 Client
    client := cloud189.NewClientWithAuthManager(authMgr,
        cloud189.WithHTTPClient(httpClient),
        cloud189.WithLogger(logger),
    )

    // 7. 创建任务管理器
    taskMgr := task.NewManager(
        task.WithMaxConcurrent(3),
        task.WithUploadStateStore(uploadStateStore),
    )

    // 8. 创建服务层
    authSvc := service.NewAuthService(authMgr, loginClient, httpClient, dataDir)
    fileSvc := service.NewFileService(client)
    transferSvc := service.NewTransferService(taskMgr, client, rawHTTP)

    // 9. 创建并运行 TUI
    app := NewApp(authSvc, fileSvc, transferSvc)
    p := tea.NewProgram(app, tea.WithAltScreen(), tea.WithMouseCellMotion())

    // 10. 订阅任务进度
    taskMgr.Subscribe(func(t *task.Task) {
        p.Send(TaskProgressMsg{Task: t.Clone()})
    })

    if _, err := p.Run(); err != nil {
        fmt.Printf("Error: %v\n", err)
        os.Exit(1)
    }
}
```

---

## 八、分阶段实现计划

### 阶段 1：基础框架与登录

**目标**：完成项目骨架和登录流程

**任务清单**：
1. 创建 `cmd/tui/main.go` - 入口和依赖初始化
2. 实现 `cmd/tui/store/session.go` - JSONSessionStore
3. 实现 `cmd/tui/logger.go` - httpclient.Logger
4. 实现 `cmd/tui/service/auth.go` - AuthService
5. 实现 `cmd/tui/ui/login/login.go` - 登录表单（textinput 组件）
6. 实现 `cmd/tui/app.go` - 根 Model 状态切换
7. 实现 `cmd/tui/messages.go` - 消息类型定义

**验收标准**：能够输入账号密码登录成功，切换到浏览器视图

### 阶段 2：文件浏览器

**目标**：完成双栏文件浏览

**任务清单**：
1. 实现 `cmd/tui/service/file.go` - FileService（封装 ListFiles/GetFileInfo）
2. 实现 `cmd/tui/ui/browser/filelist.go` - 文件列表（list 组件）
3. 实现 `cmd/tui/ui/browser/detail.go` - 详情面板
4. 实现 `cmd/tui/ui/browser/browser.go` - 双栏布局
5. 实现 `cmd/tui/ui/common/styles.go` - 样式定义
6. 实现目录导航（进入/返回上级）
7. 实现刷新功能

**验收标准**：能够浏览目录、查看文件详情、导航

### 阶段 3：文件操作

**目标**：完成文件管理功能

**任务清单**：
1. 实现 `cmd/tui/ui/dialog/confirm.go` - 确认对话框
2. 实现 `cmd/tui/ui/dialog/input.go` - 输入对话框
3. 实现新建文件夹（CreateFolder）
4. 实现删除（DeleteFiles）+ 确认对话框
5. 实现重命名（RenameFile）+ 输入对话框
6. 实现复制/移动（CopyFiles/MoveFiles）+ 目标选择
7. 实现搜索（SearchFiles）

**验收标准**：能够完成所有文件管理操作

### 阶段 4：传输任务

**目标**：完成上传下载功能

**任务清单**：
1. 实现 `cmd/tui/store/upload_state.go` - UploadStateStore
2. 实现 `cmd/tui/service/transfer.go` - AppUploader/AppDownloader
3. 实现 `cmd/tui/ui/task/panel.go` - 任务面板（progress 组件）
4. 集成 filepicker 组件选择本地文件
5. 实现上传流程
6. 实现下载流程
7. 实现暂停/恢复/取消功能

**验收标准**：能够上传下载文件，显示进度，支持暂停恢复

### 阶段 5：多账号与完善

**目标**：完成多账号和配置持久化

**任务清单**：
1. 实现 `cmd/tui/store/config.go` - ConfigStore
2. 实现 `cmd/tui/ui/account/switcher.go` - 账号切换
3. 完善错误处理和用户提示
4. 添加帮助视图（help 组件）
5. 实现配置持久化（下载目录、并发数等）

**验收标准**：能够管理多账号、持久化配置

---

## 九、关键文件路径

### Core 接口定义
- `/home/dnslin/cloud189-desktop/core/store/store.go` - 存储接口
- `/home/dnslin/cloud189-desktop/core/auth/manager.go` - AuthManager
- `/home/dnslin/cloud189-desktop/core/auth/session.go` - Session 结构
- `/home/dnslin/cloud189-desktop/core/task/manager.go` - 任务管理器
- `/home/dnslin/cloud189-desktop/core/task/upload.go` - Uploader 接口
- `/home/dnslin/cloud189-desktop/core/task/download.go` - Downloader 接口
- `/home/dnslin/cloud189-desktop/core/cloud189/client.go` - API 客户端

### 参考实现
- `/home/dnslin/cloud189-desktop/cmd/apitest/main.go` - 完整调用示例

---

## 十、快捷键设计

| 快捷键 | 功能 | 作用域 |
|--------|------|--------|
| `q` / `Ctrl+C` | 退出程序 | 全局 |
| `Tab` | 切换面板焦点 | 浏览器 |
| `Enter` | 进入目录/确认 | 文件列表 |
| `Backspace` | 返回上级目录 | 文件列表 |
| `j` / `↓` | 下移光标 | 列表 |
| `k` / `↑` | 上移光标 | 列表 |
| `g` | 跳到顶部 | 列表 |
| `G` | 跳到底部 | 列表 |
| `u` | 上传文件 | 浏览器 |
| `d` | 下载文件 | 浏览器 |
| `D` | 删除文件 | 浏览器 |
| `r` | 重命名 | 浏览器 |
| `n` | 新建文件夹 | 浏览器 |
| `c` | 复制 | 浏览器 |
| `m` | 移动 | 浏览器 |
| `/` | 搜索 | 浏览器 |
| `R` | 刷新 | 浏览器 |
| `Space` | 多选 | 文件列表 |
| `a` | 全选/取消全选 | 文件列表 |
| `p` | 暂停/恢复任务 | 任务面板 |
| `x` | 取消任务 | 任务面板 |
| `h` / `?` | 显示帮助 | 全局 |

---

## 十一、约束与规范

1. **Core 三无原则**：TUI 不修改 core，只调用
2. **存储实现**：放在 `cmd/tui/store/`
3. **遵循规范**：CONTRIBUTING.md 中的代码边界和提交规范
4. **错误处理**：使用 core/errors 的结构化错误，UI 负责展示
5. **日志输出**：通过 httpclient.Logger 接口注入，不直接 fmt.Println
