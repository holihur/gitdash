#!/usr/bin/env bash
# gitdash 备份脚本：SQLite（一致性快照）+ 所有 bare 仓库 + webhook spool + SSH host key。
#
# 用法：
#   bash scripts/backup.sh [数据目录] [输出目录]
#   默认数据目录 ./data，输出目录 ./backups（保留最近 14 份）
#
# 一致性：优先用 sqlite3 .backup 在线备份 DB；若没有 sqlite3 则直接复制文件
# （WAL 模式下建议短暂停服或先 checkpoint）。仓库目录为不可变 bare 仓库，
# 在线打包是安全的。
set -euo pipefail

DATA_DIR="${1:-./data}"
OUT_DIR="${2:-./backups}"
KEEP="${KEEP:-14}"

if [ ! -d "$DATA_DIR" ]; then
  echo "数据目录不存在: $DATA_DIR" >&2
  exit 1
fi

STAMP="$(date -u +%Y%m%d-%H%M%S)"
mkdir -p "$OUT_DIR"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

# 1) SQLite 一致性备份（优先 sqlite3 CLI；否则裸拷 + WAL 文件）
DB="$DATA_DIR/gitdash.db"
if [ -f "$DB" ]; then
  if command -v sqlite3 >/dev/null 2>&1; then
    sqlite3 "$DB" ".backup '$TMP/gitdash.db'"
    echo "db: sqlite .backup ok"
  else
    cp -f "$DB" "$TMP/gitdash.db" 2>/dev/null || true
    cp -f "$DB-wal" "$TMP/gitdash.db-wal" 2>/dev/null || true
    cp -f "$DB-shm" "$TMP/gitdash.db-shm" 2>/dev/null || true
    echo "db: direct copy (WAL) - 建议停服或安装 sqlite3 保证一致性" >&2
  fi
fi

# 2) 仓库 / webhook 事件 / SSH host key
for sub in repos webhook-events ssh_host_ed25519_key; do
  if [ -e "$DATA_DIR/$sub" ]; then
    cp -a "$DATA_DIR/$sub" "$TMP/$sub"
  fi
done

# 3) 打包
ARCHIVE="$OUT_DIR/gitdash-backup-$STAMP.tar.gz"
tar -czf "$ARCHIVE" -C "$TMP" .
echo "backup -> $ARCHIVE"

# 4) 清理旧备份
ls -1t "$OUT_DIR"/gitdash-backup-*.tar.gz 2>/dev/null | tail -n +$((KEEP + 1)) | xargs -r rm -f
echo "kept newest $KEEP backups in $OUT_DIR"
