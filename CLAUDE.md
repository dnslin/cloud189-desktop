# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

天翼云盘桌面客户端（Linux + Windows），采用 monorepo 单 go.mod 结构，提供 TUI（Bubble Tea）和 GUI（Wails）两种入口。核心原则：**一个 Core，多种入口**。

## Build & Test Commands

```bash
make check          # 全量检查（fmt + vet + lint + test + frontend-check）
make test           # Go 测试
make lint           # golangci-lint
make fmt            # gofmt
make vet            # go vet

# TUI 开发
go run ./cmd/tui

# GUI 开发（在 app 目录）
cd app && wails dev
cd app && wails build

# 前端（在 app/frontend）
pnpm install
pnpm lint
pnpm typecheck
```

## Architecture

```
core/           # 纯业务库（禁止引用 cmd/app/UI 框架）
├── cloud189/   # 189 API 实现
├── drive/      # 业务接口（面向上层）
├── auth/       # 登录/刷新
├── model/      # 领域模型（File/Task/...）
└── store/      # 存储接口定义（TokenStore/ConfigStore/SecretStore）

cmd/
├── tui/        # bubbletea 终端入口
└── cli/        # 调试入口

app/            # Wails 工程
├── backend/    # Go backend（薄封装 core）
└── frontend/   # Web 前端
```

## Critical Constraints

**Core 三无原则**（`core/**` 必须遵守）：
1. 无 UI 依赖（bubbletea/wails/前端库均禁止）
2. 无持久化副作用（不直接读写文件/数据库）
3. 无输出副作用（不直接 fmt.Println，日志通过注入）

**依赖方向**：
- `core/**` 禁止引用 `cmd/**`、`app/**`
- `cmd/tui`、`app/backend` 只能依赖 `core/**`
- 业务逻辑不得在 UI 层重复实现

**传输任务**：上传/下载使用统一 Task 模型，UI 只订阅进度事件，传输逻辑只写一次。

## Reference

`cloud189-example/` 目录为参考项目，**只读不写**。
