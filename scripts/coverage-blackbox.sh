#!/usr/bin/env bash
# 黑盒（API + UI）集成代码覆盖率：
#   go build -cover 构建被测二进制 → 通过 GOCOVERDIR 收集 pytest + Playwright
#   运行期间执行的代码 → go tool covdata 汇总百分比并转成 codecov 兼容的 profile。
# 产物：
#   /tmp/gitdash-blackbox-coverage/        （covdata 原始目录）
#   /tmp/gitdash-blackbox-coverage.out     （可上传 codecov 的 profile）
#   /tmp/gitdash-blackbox-coverage.html    （可视 HTML，可选）
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN=/tmp/gitdash-server-cover
COV_DIR=/tmp/gitdash-blackbox-coverage
OUT=/tmp/gitdash-blackbox-coverage.out

# 1) 前端构建 + 内嵌（被测二进制必须带 UI，Playwright 才能跑）
(cd "$ROOT/frontend" && pnpm install --frozen-lockfile && pnpm run build)
rm -rf "$ROOT/backend/internal/webui/dist"
mkdir -p "$ROOT/backend/internal/webui/dist"
cp -r "$ROOT/frontend/dist/." "$ROOT/backend/internal/webui/dist/"
touch "$ROOT/backend/internal/webui/dist/.gitkeep"

# 2) 覆盖率插桩二进制
(cd "$ROOT/backend" && go build -cover -o "$BIN" .)

rm -rf "$COV_DIR" "$OUT"
mkdir -p "$COV_DIR"

ROUTE_API=/tmp/gitdash-route-coverage-api.txt
ROUTE_UI=/tmp/gitdash-route-coverage-ui.txt

# 3) 跑黑盒 API 测试（conftest 自启的实例继承 GOCOVERDIR 与路由覆盖文件；
#    显式覆盖环境变量里的镜像（防 403），统一走阿里云源）
rm -f "$ROUTE_API"
(cd "$ROOT/tests" && \
  UV_DEFAULT_INDEX="https://mirrors.aliyun.com/pypi/simple/" UV_INDEX_URL="https://mirrors.aliyun.com/pypi/simple/" \
  GOCOVERDIR="$COV_DIR" GITDASH_ROUTE_COVERAGE_FILE="$ROUTE_API" GITDASH_BIN="$BIN" uv run pytest -q)

# 3b) 端点覆盖率门禁：每个 API 路由至少被黑盒测试命中一次
python3 "$ROOT/scripts/route-coverage.py" "$ROUTE_API" --min 100

# 4) 跑黑盒 UI 测试（fixture 自启的实例继承 GOCOVERDIR；SIGTERM 优雅退出后落盘）
rm -f "$ROUTE_UI"
(cd "$ROOT/tests/ui" && npm install >/dev/null 2>&1)
(cd "$ROOT/tests/ui" && GOCOVERDIR="$COV_DIR" GITDASH_ROUTE_COVERAGE_FILE="$ROUTE_UI" GITDASH_BIN="$BIN" npx playwright test)

# 4b) UI 端点覆盖率（页面操作驱动的 API 调用）；暂仅报告，阶段推进后再收紧门禁
python3 "$ROOT/scripts/route-coverage.py" "$ROUTE_UI" --min 0 || true

# 5) 汇总
(cd "$ROOT/backend" && go tool covdata percent -i="$COV_DIR" | tail -1)
(cd "$ROOT/backend" && go tool covdata textfmt -i="$COV_DIR" -o="$OUT")
(cd "$ROOT/backend" && go tool cover -func="$OUT" | tail -1)
echo "profile -> $OUT"
