#!/usr/bin/env bash
set -euo pipefail

echo "[check] gofmt"
test -z "$(gofmt -l .)"

echo "[check] go vet"
go vet ./...

echo "[check] go test"
go test ./...

if command -v golangci-lint >/dev/null 2>&1; then
  echo "[check] golangci-lint"
  golangci-lint run
else
  echo "[check] golangci-lint not found, skipping"
fi

echo "[check] frontend"
./scripts/check_frontend.sh
