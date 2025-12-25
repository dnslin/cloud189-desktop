# Cloud189 Desktop (Monorepo) — 规范与骨架（草案）

本仓库目标：**单机桌面云盘客户端（Linux + Windows）**，同时提供：

- **TUI**（Bubble Tea + Bubbles）
- **GUI**（Wails：Go backend + Web frontend）

核心原则：**一个 Core，多种入口**。Core 只实现一次业务能力；TUI/GUI 只是调用 Core。

## 目录结构（建议）

```
.
├── core/                  # 纯业务库（天翼云盘能力层）
│   ├── cloud189/          # 189 API 实现（HTTP/解析/动作）
│   ├── drive/             # 业务接口（面向上层）
│   ├── auth/              # 登录/刷新
│   ├── model/             # 领域模型（File/Task/...）
│   └── store/             # Token/Config/Secret 存储接口定义（实现放上层）
├── cmd/
│   ├── tui/               # bubbletea 终端入口
│   └── cli/               # (可选) 最小调试入口（开发排障用）
├── app/                   # wails 工程
│   ├── backend/           # Go backend：仅薄封装 core
│   └── frontend/          # Web 前端（React/Vue/Svelte 任选其一）
├── docs/
│   ├── dev.md             # 本地开发指南
│   └── adr/               # 架构决策记录（ADR）
├── scripts/               # CI / 本地脚本
└── .github/workflows/ci.yml
```

## 依赖方向（强制）

- `core/**` **禁止**引用：`cmd/**`、`app/**`、任何 UI 框架、任何直接持久化/打印。
- `cmd/tui`、`app/backend` 只能依赖 `core/**`。
- 业务逻辑不得在 UI 层重复实现。

## 快速开始

- Go：`make test`、`make lint`
- 前端（在 `app/frontend`）：参见 `docs/dev.md`
- 全量检查：`make check`

## 文档入口

- 开发与命令：`docs/dev.md`
- 贡献规范：`CONTRIBUTING.md`
- 架构决策：`docs/adr/*`

## 参考项目

- cloud189-example 禁止写入此目录,此目录只能读取

