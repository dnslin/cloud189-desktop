# Contributing / 开发规范

本项目是单机客户端（Linux + Windows），采用 monorepo；核心目标是**可持续迭代**。

## 1. 分支与合并

- `main`：永远可发布（CI 全绿）
- `feat/*`：功能
- `fix/*`：修复
- `chore/*`：工具/CI/依赖
- 所有改动走 PR；PR 必须通过 CI 才能合并。

## 2. Commit 规范（Conventional Commits）

示例：
- `feat(core): add token refresh`
- `feat(tui): add transfer panel`
- `fix(core): handle expired cookie`
- `chore(ci): add golangci-lint`
- `docs(adr): record transfer task model decision`

## 3. 代码边界（最重要）

### 3.1 Core 三无原则

`core/**` 必须做到：
1) 无 UI 依赖（Bubble Tea / Wails / 前端库都不许）
2) 无持久化副作用（不直接读写配置文件/数据库/注册表）
3) 无输出副作用（不直接 `fmt.Println`，日志通过注入）

### 3.2 存储通过接口注入

在 `core/store` 定义接口：
- `TokenStore`：token/refresh token/cookie
- `ConfigStore`：用户偏好（下载目录、并发数等）
- `SecretStore`：安全存储（keyring/DPAPI/Secret Service）

实现放在上层（`cmd/**` 或 `app/backend/**`）。

### 3.3 错误与返回值

- Core 对外暴露结构化错误（code/op/cause），UI 负责展示文案。
- 禁止用“中文提示字符串”做业务判断。

## 4. 任务系统（上传/下载）

上传/下载必须使用统一 Task 模型（详见 ADR 0003）。
- 任务是异步的，返回 `TaskID`
- UI 通过订阅进度事件展示进度/速度/ETA
- 传输逻辑只写一次，禁止 TUI/GUI 各自实现

## 5. 质量闸门（工具强制）

### Go
- `gofmt` 必须
- `go vet` 必须
- `golangci-lint` 必须
- `go test ./...` 必须

### Frontend (Wails)
- `eslint` + `prettier`
- `typecheck`（TS）
- 锁定包管理器（建议 `pnpm`）

## 6. PR Checklist

- [ ] 通过 `make check`
- [ ] 新增/修改 API 有相应测试或至少 smoke 测试
- [ ] 文档或 ADR 有同步更新（当架构决策发生变化）
