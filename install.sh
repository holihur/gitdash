#!/usr/bin/env bash
# gitdash 一键安装脚本
#
#   curl -fsSL https://raw.githubusercontent.com/holihur/gitdash/main/install.sh | bash
#
# 环境变量:
#   GITDASH_VERSION      指定版本 (如 v0.1.0)，默认最新 release
#   GITDASH_INSTALL_DIR  安装目录，默认 /usr/local/bin（无权限时 ~/.local/bin）
set -euo pipefail

REPO="holihur/gitdash"
BIN_NAME="gitdash"
INSTALL_DIR="${GITDASH_INSTALL_DIR:-}"
VERSION="${GITDASH_VERSION:-}"

log() { printf '\033[1;34m==>\033[0m %s\n' "$*"; }
err() { printf '\033[1;31merror:\033[0m %s\n' "$*" >&2; exit 1; }

fetch() {
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$1"
  elif command -v wget >/dev/null 2>&1; then
    wget -qO- "$1"
  else
    err "需要 curl 或 wget"
  fi
}

fetch_to() {
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$1" -o "$2"
  else
    wget -qO "$2" "$1"
  fi
}

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$OS" in
linux | darwin) ;;
*)
  err "暂不支持 $OS，请到 https://github.com/$REPO/releases 手动下载"
  ;;
esac

case "$(uname -m)" in
x86_64 | amd64) ARCH="amd64" ;;
aarch64 | arm64) ARCH="arm64" ;;
*) err "不支持的架构: $(uname -m)" ;;
esac

if [ -z "$VERSION" ]; then
  log "查询最新版本..."
  VERSION="$(fetch "https://api.github.com/repos/$REPO/releases/latest" |
    sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -n1)"
  [ -n "$VERSION" ] || err "无法获取最新版本，可设置 GITDASH_VERSION 重试"
fi
VER="${VERSION#v}"
URL="https://github.com/$REPO/releases/download/$VERSION/${BIN_NAME}_${VER}_${OS}_${ARCH}.tar.gz"

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

log "下载 $URL"
fetch_to "$URL" "$TMP/gitdash.tar.gz"
tar -xzf "$TMP/gitdash.tar.gz" -C "$TMP"
[ -f "$TMP/$BIN_NAME" ] || err "压缩包中未找到 $BIN_NAME"

if [ -z "$INSTALL_DIR" ]; then
  if [ -w /usr/local/bin ] || [ "$(id -u)" = "0" ]; then
    INSTALL_DIR="/usr/local/bin"
  else
    INSTALL_DIR="$HOME/.local/bin"
    log "无权限写 /usr/local/bin，安装到 $INSTALL_DIR"
  fi
fi
mkdir -p "$INSTALL_DIR"

if [ -e "$INSTALL_DIR/$BIN_NAME" ] && [ ! -w "$INSTALL_DIR" ] && [ "$(id -u)" != "0" ]; then
  log "需要 sudo 覆盖 $INSTALL_DIR/$BIN_NAME"
  sudo install -m 0755 "$TMP/$BIN_NAME" "$INSTALL_DIR/$BIN_NAME"
else
  install -m 0755 "$TMP/$BIN_NAME" "$INSTALL_DIR/$BIN_NAME"
fi

case ":$PATH:" in
*":$INSTALL_DIR:"*) ;;
*)
  log "提示: $INSTALL_DIR 不在 PATH 中，请执行: export PATH=\$PATH:$INSTALL_DIR"
  ;;
esac

log "已安装 ${BIN_NAME} ${VERSION} -> $INSTALL_DIR/$BIN_NAME"
echo
echo "快速开始:"
echo "  ${BIN_NAME} serve                          # 默认 http://localhost:8080 / ssh :2222"
echo "  GITDASH_TOKEN=自定义token ${BIN_NAME} serve # 修改 Web API token"
echo "  systemd: 参考仓库 packaging/gitdash.service"
