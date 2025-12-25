#!/usr/bin/env bash
set -euo pipefail

echo "[setup] Installing Go tools (optional)"
if command -v go >/dev/null 2>&1; then
  echo " - golangci-lint: https://golangci-lint.run/usage/install/"
else
  echo "Go not found; install Go first."
fi

echo "[setup] Frontend package manager"
echo "Recommended: enable corepack and use pnpm"
echo "  corepack enable"
