#!/usr/bin/env bash
#
# Backup / restore drill (issue #4 P2.8).
#
# Boots a throwaway gitdash instance, creates data, backs it up, verifies the
# archive, restores it into a fresh data dir, boots that instance and confirms
# the account + repo survived the round-trip.
#
# Usage: GITDASH_BIN=/path/to/gitdash scripts/backup-drill.sh
set -euo pipefail

BIN="${GITDASH_BIN:-}"
if [ -z "$BIN" ] || [ ! -x "$BIN" ]; then
  echo "GITDASH_BIN must point to a built gitdash binary" >&2
  exit 2
fi

WORK="$(mktemp -d)"
SERVER_PID=""
SERVER2_PID=""
cleanup() {
  [ -n "$SERVER_PID" ] && kill "$SERVER_PID" 2>/dev/null || true
  [ -n "$SERVER2_PID" ] && kill "$SERVER2_PID" 2>/dev/null || true
  rm -rf "$WORK"
}
trap cleanup EXIT

DATA="$WORK/data"
BACKUPS="$WORK/backups"
RESTORED="$WORK/restored"
mkdir -p "$DATA" "$BACKUPS" "$RESTORED"

json_field() { # json_field <field>  (reads JSON on stdin)
  python3 -c 'import json,sys;print(json.load(sys.stdin)["'"$1"'"])'
}

wait_health() {
  for _ in $(seq 1 60); do
    curl -fsS "$1/api/health" >/dev/null 2>&1 && return 0
    sleep 1
  done
  return 1
}

echo "== boot source instance =="
GITDASH_DATA="$DATA" GITDASH_HTTP_ADDR=127.0.0.1:18091 GITDASH_SSH_ADDR=127.0.0.1:18291 \
  GITDASH_PROFILE_REPO=0 GITDASH_DISABLE_RATE_LIMIT=1 \
  "$BIN" serve >"$WORK/source.log" 2>&1 &
SERVER_PID=$!
wait_health http://127.0.0.1:18091 || { cat "$WORK/source.log"; exit 1; }

REG="$(curl -fsS -X POST http://127.0.0.1:18091/api/auth/register \
  -H 'Content-Type: application/json' \
  -d '{"username":"drilluser","password":"drill-pass-123"}')"
TOKEN="$(printf '%s' "$REG" | json_field token)"
curl -fsS -X POST http://127.0.0.1:18091/api/repos \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"name":"drill","private":true}' >/dev/null
echo "created account + repo"

kill "$SERVER_PID"; wait "$SERVER_PID" 2>/dev/null || true; SERVER_PID=""

echo "== backup =="
GITDASH_DATA="$DATA" "$BIN" backup -d "$BACKUPS" -k 3
ARCHIVE="$(ls "$BACKUPS"/gitdash-backup-*.tar.gz | head -1)"
echo "archive: $ARCHIVE"

echo "== verify + restore =="
GITDASH_DATA="$RESTORED" "$BIN" restore "$ARCHIVE" --dry-run
GITDASH_DATA="$RESTORED" "$BIN" restore "$ARCHIVE" --force

echo "== boot restored instance and verify data =="
GITDASH_DATA="$RESTORED" GITDASH_HTTP_ADDR=127.0.0.1:18092 GITDASH_SSH_ADDR=127.0.0.1:18292 \
  GITDASH_PROFILE_REPO=0 GITDASH_DISABLE_RATE_LIMIT=1 \
  "$BIN" serve >"$WORK/restored.log" 2>&1 &
SERVER2_PID=$!
wait_health http://127.0.0.1:18092 || { cat "$WORK/restored.log"; exit 1; }

LOGIN="$(curl -fsS -X POST http://127.0.0.1:18092/api/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"drilluser","password":"drill-pass-123"}')"
RTOKEN="$(printf '%s' "$LOGIN" | json_field token)"
curl -fsS http://127.0.0.1:18092/api/users/drilluser/repos/drill \
  -H "Authorization: Bearer $RTOKEN" | grep -q '"name":"drill"'

echo "BACKUP DRILL OK"
