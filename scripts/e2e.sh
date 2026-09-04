#!/usr/bin/env bash
# gitdash E2E 冒烟测试：注册/登录 -> 建仓库 -> 加公钥 -> SSH clone/push -> 代码浏览 -> 多用户隔离
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
HTTP="127.0.0.1:18080"
SSH_ADDR="127.0.0.1:12222"
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

GITDASH_DATA="$TMP/data" GITDASH_HTTP_ADDR="$HTTP" GITDASH_SSH_ADDR="$SSH_ADDR" \
  "$BIN" >"$TMP/server.log" 2>&1 &
SERVER_PID=$!

for _ in $(seq 1 40); do
  if curl -sf "http://$HTTP/api/health" >/dev/null 2>&1; then break; fi
  sleep 0.25
done

API="http://$HTTP/api"
json_field() { sed -n "s/.*\"$1\":\"\([^\"]*\)\".*/\1/p"; }
auth_header() { printf 'Authorization: Bearer %s' "$1"; }

echo "== unauthorized request rejected"
if curl -sf "$API/repos" >/dev/null 2>&1; then
  echo "expected 401 without token" >&2
  exit 1
fi

echo "== register alice (auto-login)"
TOKEN_A="$(curl -sf -X POST -d '{"username":"alice","password":"alice-pass-123"}' "$API/auth/register" | json_field token)"
[ -n "$TOKEN_A" ]

echo "== duplicate register rejected"
if curl -sf -X POST -d '{"username":"alice","password":"another-pass-1"}' "$API/auth/register" >/dev/null 2>&1; then
  echo "expected 409 on duplicate username" >&2
  exit 1
fi

echo "== wrong password rejected"
if curl -sf -X POST -d '{"username":"alice","password":"wrong-pass-12"}' "$API/auth/login" >/dev/null 2>&1; then
  echo "expected 401 on wrong password" >&2
  exit 1
fi

echo "== login alice"
TOKEN_A="$(curl -sf -X POST -d '{"username":"alice","password":"alice-pass-123"}' "$API/auth/login" | json_field token)"
[ -n "$TOKEN_A" ]
curl -sf -H "$(auth_header "$TOKEN_A")" "$API/me" | grep -q '"username":"alice"'

echo "== create repo (owner=alice)"
curl -sf -H "$(auth_header "$TOKEN_A")" -X POST -d '{"name":"demo","description":"e2e repo"}' "$API/repos" |
  grep -q '"owner":"alice"'

echo "== add ssh key (bound to alice)"
ssh-keygen -q -t ed25519 -N "" -f "$TMP/key-alice"
curl -sf -H "$(auth_header "$TOKEN_A")" -X POST \
  -d "$(printf '{"name":"e2e","public_key":"%s"}' "$(cat "$TMP/key-alice.pub")")" "$API/keys" | grep -q 'SHA256'

echo "== ssh clone (bare path -> own repo) / commit / push"
export GIT_SSH_COMMAND="ssh -i $TMP/key-alice -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null"
git clone -q "ssh://git@$SSH_ADDR/demo.git" "$TMP/demo"
(
  cd "$TMP/demo"
  echo "# demo" >README.md
  mkdir -p src
  printf 'package main\n\nfunc main() {}\n' >src/main.go
  git add -A
  git -c user.name=alice -c user.email=alice@example.com commit -qm "initial commit"
  git push -q origin HEAD
)

echo "== fresh clone via owner path (alice/demo.git)"
git clone -q "ssh://git@$SSH_ADDR/alice/demo.git" "$TMP/demo2"
grep -q '# demo' "$TMP/demo2/README.md"

echo "== browsing api (alice)"
curl -sf -H "$(auth_header "$TOKEN_A")" "$API/repos/demo/branches" | grep -q 'main'
curl -sf -H "$(auth_header "$TOKEN_A")" "$API/repos/demo/tree?ref=main" | grep -q '"name":"README.md"'
curl -sf -H "$(auth_header "$TOKEN_A")" "$API/repos/demo/tree?ref=main&path=src" | grep -q '"name":"main.go"'
curl -sf -H "$(auth_header "$TOKEN_A")" "$API/repos/demo/blob?ref=main&path=README.md" | grep -q '# demo'
curl -sf -H "$(auth_header "$TOKEN_A")" "$API/repos/demo/commits?ref=main" | grep -q 'initial commit'

echo "== register bob, add his key"
TOKEN_B="$(curl -sf -X POST -d '{"username":"bob","password":"bob-pass-1234"}' "$API/auth/register" | json_field token)"
[ -n "$TOKEN_B" ]
ssh-keygen -q -t ed25519 -N "" -f "$TMP/key-bob"
curl -sf -H "$(auth_header "$TOKEN_B")" -X POST \
  -d "$(printf '{"name":"e2e","public_key":"%s"}' "$(cat "$TMP/key-bob.pub")")" "$API/keys" | grep -q 'SHA256'

echo "== bob cannot clone alice's repo via ssh"
if GIT_SSH_COMMAND="ssh -i $TMP/key-bob -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null" \
  git clone -q "ssh://git@$SSH_ADDR/alice/demo.git" "$TMP/bob-alice-demo" 2>/dev/null; then
  echo "expected clone failure for other user's repo" >&2
  exit 1
fi

echo "== bob cannot touch alice's repo via api"
if curl -sf -H "$(auth_header "$TOKEN_B")" "$API/repos/demo" >/dev/null 2>&1; then
  echo "expected 404 for other user's repo" >&2
  exit 1
fi
if curl -sf -H "$(auth_header "$TOKEN_B")" -X DELETE "$API/repos/demo" >/dev/null 2>&1; then
  echo "expected delete failure for other user's repo" >&2
  exit 1
fi

echo "== same repo name across users is allowed (bob/demo)"
curl -sf -H "$(auth_header "$TOKEN_B")" -X POST -d '{"name":"demo"}' "$API/repos" | grep -q '"owner":"bob"'
GIT_SSH_COMMAND="ssh -i $TMP/key-bob -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null" \
  git clone -q "ssh://git@$SSH_ADDR/bob/demo.git" "$TMP/bob-demo"

echo "== keys are per-user"
curl -sf -H "$(auth_header "$TOKEN_A")" "$API/keys" | grep -q '"name":"e2e"'
curl -sf -H "$(auth_header "$TOKEN_B")" "$API/keys" | grep -q '"name":"e2e"'

echo "== logout invalidates the session"
curl -sf -H "$(auth_header "$TOKEN_A")" -X POST "$API/auth/logout" >/dev/null
if curl -sf -H "$(auth_header "$TOKEN_A")" "$API/me" >/dev/null 2>&1; then
  echo "expected 401 after logout" >&2
  exit 1
fi

echo
echo "E2E OK ✔"
