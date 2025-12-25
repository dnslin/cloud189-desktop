# 本地开发指南

本文档约定仓库根目录为项目根（单 go.mod）。

## 1) 先决条件

### Go
- Go 1.22+（建议与 CI 一致）

### Wails（GUI）
- 按 Wails 官方文档安装（Go + Node + 平台依赖）
- Windows 需要 WebView2（通常系统自带/可安装）
- Linux 需要 WebKitGTK 等依赖（按发行版提示安装）

## 2) 常用命令（根目录）

### 全量检查
```bash
make check
```

### Go 测试与静态检查
```bash
make test
make lint
```

### 仅检查变更（建议配合 pre-commit / lefthook）
```bash
./scripts/check.sh
```

## 3) TUI 开发

建议先用一个最小 `cmd/cli`（可选）跑通：login + ls。随后再上 bubbletea。

运行（示例）：
```bash
go run ./cmd/tui
```

## 4) GUI（Wails）开发

进入目录：
```bash
cd app
```

前端依赖（示例使用 pnpm）：
```bash
pnpm install
```

开发模式：
```bash
wails dev
```

构建：
```bash
wails build
```

> 注意：本项目约定 GUI 的 backend 只做薄封装，所有业务走 `core/**`。

## 5) 推荐：将配置/凭证存储实现为“可替换”

- `TokenStore`：开发阶段可先落盘（本地），正式版再接 Keyring/DPAPI/Secret Service
- `SecretStore`：尽量不用明文 JSON
