#!/usr/bin/env bash
# gitdash E2E 冒烟测试：建仓库 -> 加公钥 -> SSH clone/push -> 代码浏览 API
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
HTTP="127.0.0.1:18080"
SSH_ADDR="127.0.0.1:12222"
TOKEN="e2e-token"
BIN="$ROOT/gitdash-server-e2e"

command -v git >/dev/null || { echo "git is required"; exit 1; }
command -v ssh-keygen >/dev/null || { echo "ssh-keygen is required"; exit 1; }

(cd "$ROOT/backend" && go build -o "$BIN" .)

TMP="$(mktemp -d)"
SERVER_PID=""
cleanup() {
  rc=$?
  if [ -n "$SERVER_PID" ]; then kill "$SERVER_PID" 2>/dev/null || true; fi
  if [ "$rc" -ne 0 ] && [ -f "$TMP/server.log" ]; then
    echo "--- server.log (tail) ---"
    tail -n 50 "$TMP/server.log" || true
  fi
  rm -rf "$TMP" "$BIN" 2>/dev/null || true
  exit "$rc"
}
trap cleanup EXIT

GITDASH_DATA="$TMP/data" GITDASH_HTTP_ADDR="$HTTP" GITDASH_SSH_ADDR="$SSH_ADDR" GITDASH_TOKEN="$TOKEN" \
  "$BIN" >"$TMP/server.log" 2>&1 &
SERVER_PID=$!

for _ in $(seq 1 40); do
  if curl -sf "http://$HTTP/api/health" >/dev/null 2>&1; then break; fi
  sleep 0.25
done

auth() { curl -sf -H "Authorization: Bearer $TOKEN" "$@"; }

echo "== health"
curl -sf "http://$HTTP/api/health" | grep -q '"ok"'

echo "== unauthorized request rejected"
if curl -sf "http://$HTTP/api/repos" >/dev/null 2>&1; then
  echo "expected 401 without token" >&2
  exit 1
fi

echo "== create repo"
auth -X POST -d '{"name":"demo","description":"e2e repo"}' "http://$HTTP/api/repos" | grep -q '"name":"demo"'

echo "== duplicate create rejected"
if auth -X POST -d '{"name":"demo"}' "http://$HTTP/api/repos" >/dev/null 2>&1; then
  echo "expected 409 on duplicate" >&2
  exit 1
fi

echo "== add ssh key"
ssh-keygen -q -t ed25519 -N "" -f "$TMP/key"
auth -X POST -d "$(printf '{"name":"e2e","public_key":"%s"}' "$(cat "$TMP/key.pub")")" \
  "http://$HTTP/api/keys" | grep -q 'SHA256'

echo "== ssh clone / commit / push"
export GIT_SSH_COMMAND="ssh -i $TMP/key -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null"
git clone -q "ssh://git@$SSH_ADDR/demo.git" "$TMP/demo"
(
  cd "$TMP/demo"
  echo "# demo" >README.md
  mkdir -p src
  printf 'package main\n\nfunc main() {}\n' >src/main.go
  git add -A
  git -c user.name=e2e -c user.email=e2e@example.com commit -qm "initial commit"
  git push -q origin HEAD
)

echo "== fresh clone sees the pushed commit"
git clone -q "ssh://git@$SSH_ADDR/demo.git" "$TMP/demo2"
grep -q '# demo' "$TMP/demo2/README.md"

echo "== branches api"
auth "http://$HTTP/api/repos/demo/branches" | grep -q 'main'

echo "== tree api"
auth "http://$HTTP/api/repos/demo/tree?ref=main" | grep -q '"name":"README.md"'
auth "http://$HTTP/api/repos/demo/tree?ref=main&path=src" | grep -q '"name":"main.go"'

echo "== blob api"
auth "http://$HTTP/api/repos/demo/blob?ref=main&path=README.md" | grep -q '# demo'

echo "== commits api"
auth "http://$HTTP/api/repos/demo/commits?ref=main" | grep -q 'initial commit'

echo "== unknown key rejected by ssh"
ssh-keygen -q -t ed25519 -N "" -f "$TMP/other" -C other
if GIT_SSH_COMMAND="ssh -i $TMP/other -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null" \
  git clone -q "ssh://git@$SSH_ADDR/demo.git" "$TMP/demo3" 2>/dev/null; then
  echo "expected ssh auth failure for unknown key" >&2
  exit 1
fi

echo
echo "E2E OK ✔"
