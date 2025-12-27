# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

Replies must be in Chinese and logs and comments must be output in Chinese.

## Project Overview

天翼云盘桌面客户端（Linux + Windows），采用 monorepo 单 go.mod 结构。核心原则：**一个 Core，多种入口**。

当前状态：Core 业务库已完成，TUI/GUI 入口待开发。

## Build & Test Commands

```bash
make check          # 全量检查（fmt + vet + lint + test + frontend-check）
make test           # Go 测试（含 race 检测和覆盖率）
make lint           # golangci-lint（仅 core 目录）
make fmt            # gofmt
make vet            # go vet

# 调试入口
go run ./cmd/apitest

# 单个测试
go test ./core/auth/... -v -run TestLogin
```

## Architecture

```
core/               # 纯业务库（禁止引用 cmd/app/UI 框架）
├── auth/           # 登录/刷新/多账号会话管理
├── cloud189/       # 天翼云 API 实现（client/signer/upload）
├── crypto/         # 加密工具（RSA/AES/HMAC/MD5）
├── errors/         # 结构化错误（code/op/cause）
├── httpclient/     # HTTP 客户端封装（重试/限流/中间件）
├── model/          # 领域模型（File/User）
└── store/          # 存储接口定义（SessionStore/TokenStore/ConfigStore）

cmd/
└── apitest/        # 调试入口

cloud189-example/   # 参考项目（只读不写）
```

## Key Files

- `core/auth/manager.go` - 多账号会话管理器 AuthManager
- `core/cloud189/client.go` - API 客户端（会话刷新/账号切换）
- `core/store/store.go` - 存储接口定义

## Critical Constraints

**Core 三无原则**（`core/**` 必须遵守）：

1. 无 UI 依赖（bubbletea/wails/前端库均禁止）
2. 无持久化副作用（不直接读写文件/数据库）
3. 无输出副作用（不直接 fmt.Println，日志通过注入）

**依赖方向**：

- `core/**` 禁止引用 `cmd/**`、`app/**`
- 业务逻辑不得在 UI 层重复实现
- 存储通过接口注入，实现放上层

## Reference

`cloud189-example/` 目录为参考项目，**只读不写**。
