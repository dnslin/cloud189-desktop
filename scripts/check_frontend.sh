#!/usr/bin/env bash
set -euo pipefail

if [ ! -d "app/frontend" ]; then
  echo "[frontend] app/frontend not found, skipping"
  exit 0
fi

pushd app/frontend >/dev/null

if [ ! -f package.json ]; then
  echo "[frontend] package.json not found, skipping"
  popd >/dev/null
  exit 0
fi

if command -v pnpm >/dev/null 2>&1; then
  echo "[frontend] pnpm install"
  if [ -f pnpm-lock.yaml ]; then
    pnpm install --frozen-lockfile
  else
    pnpm install
  fi
  echo "[frontend] lint/typecheck"
  (pnpm -s lint || true)
  (pnpm -s typecheck || true)
elif command -v npm >/dev/null 2>&1; then
  echo "[frontend] npm install"
  if [ -f package-lock.json ]; then
    npm ci
  else
    npm install
  fi
  echo "[frontend] lint/typecheck"
  (npm run lint || true)
  (npm run typecheck || true)
else
  echo "[frontend] no node package manager found, skipping"
fi

popd >/dev/null
