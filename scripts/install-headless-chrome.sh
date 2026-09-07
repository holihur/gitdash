#!/usr/bin/env bash
# 后台安装 headless chromium（通过 Playwright）
# 用法: bash scripts/install-headless-chrome.sh
# 日志写入 /tmp/headless-chrome-install.log

set -euo pipefail

LOG=/tmp/headless-chrome-install.log
MARK=/tmp/headless-chrome-install.done

# 已经在后台跑过且成功完成
if [[ -f "$MARK" ]]; then
  echo "已安装完成: $MARK"
  exit 0
fi

# 以后台方式重启本脚本，父进程立即返回
if [[ "${BG_RUN:-0}" != "1" ]]; then
  nohup env BG_RUN=1 bash "$0" >"$LOG" 2>&1 &
  echo "已在后台开始安装，日志: $LOG"
  echo "查看进度:  tail -f $LOG"
  echo "完成后会生成标记文件: $MARK"
  exit 0
fi

# ---------- 以下是后台真正执行的逻辑 ----------
log() { echo "[$(date '+%F %T')] $*"; }

install_deps() {
  log "检查系统依赖 (playwright 所需)..."
  if command -v apt-get >/dev/null 2>&1; then
    sudo apt-get update
    # playwright install-deps 会装好运行 chromium 所需的所有系统库
    sudo npx playwright install-deps chromium || {
      log "install-deps 失败，尝试常见依赖包"; return 1; }
  fi
}

main() {
  log "开始安装 headless chromium (Playwright) ..."

  # 1) 找一个可用的项目目录放 playwright（避免全局污染任意目录）
  local target_dir="${PLAYWRIGHT_DIR:-}"
  if [[ -z "$target_dir" ]]; then
    for d in "$HOME/.playwright-setup" /tmp/playwright-setup; do
      mkdir -p "$d" && target_dir="$d" && break
    done
  fi
  cd "$target_dir"
  log "工作目录: $target_dir"

  # 2) 确保有 playwright
  if [[ ! -d node_modules/playwright ]]; then
    log "安装 npm 包 playwright ..."
    npm init -y >/dev/null 2>&1 || true
    npm install playwright
  fi

  # 3) 下载 chromium 二进制（只装 chromium，不装 ffmpeg/webkit 等）
  log "下载 chromium ..."
  PLAYWRIGHT_BROWSERS_PATH="${PLAYWRIGHT_BROWSERS_PATH:-$HOME/.cache/ms-playwright}" \
    npx playwright install chromium

  # 4) 版本校验
  log "校验版本 ..."
  PLAYWRIGHT_BROWSERS_PATH="${PLAYWRIGHT_BROWSERS_PATH:-$HOME/.cache/ms-playwright}" \
    npx playwright --version

  log "全部完成，生成标记文件"
  touch "$MARK"
}

main
