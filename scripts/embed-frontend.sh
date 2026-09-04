#!/usr/bin/env bash
# 构建前端并把产物拷贝到 backend/internal/webui/dist 供 go:embed 使用
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

command -v npm >/dev/null 2>&1 || { echo "npm is required to build the frontend" >&2; exit 1; }

(cd "$ROOT/frontend" && npm ci --no-audit --no-fund && npm run build)

rm -rf "$ROOT/backend/internal/webui/dist"
mkdir -p "$ROOT/backend/internal/webui/dist"
cp -r "$ROOT/frontend/dist/." "$ROOT/backend/internal/webui/dist/"
touch "$ROOT/backend/internal/webui/dist/.gitkeep"

echo "embedded frontend assets -> backend/internal/webui/dist"
