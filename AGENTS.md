# Repository Guidelines

Cloud189 Desktop targets one Core with multiple entry points (TUI via Bubble Tea, GUI via Wails). Keep business logic centralized and side-effect free.
Replies must be in Chinese and logs and comments must be output in Chinese.

## Project Structure & Module Organization

- Root tools: `Makefile`, `Taskfile.yml`, `scripts/` (checks/setup), `docs/` (dev guide + ADRs), `CONTRIBUTING.md`.
- `cloud189-example/` is an upstream CLI mirror; treat it as read-only.
- Planned layout (see README): `core/**` (business only), `cmd/tui` (terminal UI), `cmd/cli` (debug), `app/backend` (thin Go wrapper), `app/frontend` (web UI). Flow dependencies into `core`, never the reverse.
- ADRs: `docs/adr/*.md` (e.g., 0002 no side effects, 0003 transfer task model).

## Build, Test, and Development Commands

- `make check` (or `task check`): gofmt + vet + golangci-lint + go test + frontend lint/typecheck.
- `make fmt | vet | lint | test`: run individual steps; `./scripts/check.sh` is an all-in-one variant that skips missing tools.
- `./scripts/check_frontend.sh`: installs deps (pnpm/npm) and runs lint/typecheck when `app/frontend` exists.
- Run TUI: `go run ./cmd/tui` (or `./cmd/cli`). GUI: `cd app && wails dev` / `wails build`.

## Coding Style & Naming Conventions

- Go: `gofmt` is mandatory; idiomatic naming (exported PascalCase, locals lowerCamel). Avoid globals; inject storage/logging.
- Core rules (ADR 0002): no UI imports, no direct persistence, no direct printing—return structured errors/events instead.
- Frontend: eslint + prettier + TS typecheck; prefer pnpm via `corepack enable`.
- Configuration/credentials stay pluggable via `core/store`; avoid hardcoded paths.

## Testing Guidelines

- Go tests sit beside code in `_test.go`; prefer table-driven cases and cover error paths. Run `go test ./...` (inside `make check`).
- Core: add tests for new APIs and regressions; avoid network calls—stub interfaces instead.
- Frontend: add `lint`/`typecheck` scripts to `package.json` if you add UI code; keep snapshot/UI diffs minimal.

## Commit & Pull Request Guidelines

- Conventional Commits with scopes such as `core`, `tui`, `app`, `docs`, `ci`, `adr` (e.g., `feat(core): add token refresh`).
- Branches: `feat/*`, `fix/*`, `chore/*`; all changes land via PR with green CI.
- Before a PR: ensure `make check` passes, link related issues/ADRs, state tests run, include screenshots for UI changes.

## Security & Configuration Tips

- Never commit real tokens, cookies, or personal cloud paths. Prefer keyring/OS secrets; use local disk only for dev scaffolding.
- Respect the Core boundary: keep secrets and platform-specific code above `core/**`. Do not edit `cloud189-example/` unless explicitly requested.

