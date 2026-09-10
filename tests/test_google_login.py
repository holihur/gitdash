"""Google 登录黑盒测试 —— 专用实例 + 进程内假 Google 端点。

只通过 HTTP API 验证：管理端开启 Google 登录后，完整授权码流程可换取会话；
未开启时 start 端点 404。
"""
from __future__ import annotations

import json
import os
import socket
import subprocess
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import parse_qs, quote, urlparse

import pytest
import requests

GITDASH_BIN = os.environ.get("GITDASH_BIN", "").strip()
ADMIN_USER = "gitdash-admin"
ADMIN_PASS = "admin-test-pass-123456"


class _FakeGoogle(BaseHTTPRequestHandler):
    def log_message(self, *args):  # noqa: D102
        pass

    def _json(self, code: int, payload: dict):
        body = json.dumps(payload).encode()
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self):  # noqa: N802
        u = urlparse(self.path)
        if u.path == "/auth":
            q = parse_qs(u.query)
            rd = q.get("redirect_uri", [""])[0]
            state = q.get("state", [""])[0]
            self.send_response(302)
            self.send_header("Location", f"{rd}?code=fake-code&state={quote(state)}")
            self.end_headers()
        elif u.path == "/userinfo":
            if self.headers.get("Authorization") != "Bearer fake-google-token":
                self._json(401, {"error": "unauthorized"})
                return
            self._json(200, {"sub": "g-1", "email": "bob@gmail.com", "email_verified": True, "name": "Bob"})
        else:
            self._json(404, {"error": "not found"})

    def do_POST(self):  # noqa: N802
        u = urlparse(self.path)
        if u.path == "/token":
            n = int(self.headers.get("Content-Length") or 0)
            self.rfile.read(n)
            self._json(200, {"access_token": "fake-google-token"})
        else:
            self._json(404, {"error": "not found"})


def _free_port() -> int:
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as s:
        s.bind(("127.0.0.1", 0))
        return s.getsockname()[1]


@pytest.fixture(scope="module")
def google_env(tmp_path_factory):
    """独立实例：GITDASH_GOOGLE_* 指向进程内假 Google 端点。"""
    if not GITDASH_BIN:
        pytest.skip("google login test needs GITDASH_BIN")

    fake = ThreadingHTTPServer(("127.0.0.1", 0), _FakeGoogle)
    thread = threading.Thread(target=fake.serve_forever, daemon=True)
    thread.start()
    fake_base = f"http://127.0.0.1:{fake.server_address[1]}"

    tmpdir = tmp_path_factory.mktemp("gitdash-google")
    http_port, ssh_port = _free_port(), _free_port()
    env = dict(os.environ)
    env.update(
        GITDASH_DATA=str(tmpdir / "data"),
        GITDASH_DISABLE_RATE_LIMIT="1",
        GITDASH_HTTP_ADDR=f"127.0.0.1:{http_port}",
        GITDASH_SSH_ADDR=f"127.0.0.1:{ssh_port}",
        GITDASH_ADMIN_USER=ADMIN_USER,
        GITDASH_ADMIN_PASSWORD=ADMIN_PASS,
        GITDASH_GOOGLE_AUTH_URL=fake_base + "/auth",
        GITDASH_GOOGLE_TOKEN_URL=fake_base + "/token",
        GITDASH_GOOGLE_USERINFO_URL=fake_base + "/userinfo",
    )
    log = open(tmpdir / "server.log", "wb")
    proc = subprocess.Popen([GITDASH_BIN, "serve"], env=env, stdout=log, stderr=subprocess.STDOUT)
    base = f"http://127.0.0.1:{http_port}"
    ready = False
    deadline = time.time() + 30
    while time.time() < deadline:
        if proc.poll() is not None:
            break
        try:
            if requests.get(f"{base}/api/health", timeout=1).status_code == 200:
                ready = True
                break
        except requests.RequestException:
            pass
        time.sleep(0.2)
    if not ready:
        proc.kill()
        log.close()
        fake.shutdown()
        pytest.fail(f"google-login server not ready; see {tmpdir / 'server.log'}")

    try:
        yield base
    finally:
        proc.terminate()
        try:
            proc.wait(timeout=10)
        except subprocess.TimeoutExpired:
            proc.kill()
        log.close()
        fake.shutdown()


@pytest.fixture(scope="module")
def admin_session(google_env):
    s = requests.Session()
    r = s.post(
        f"{google_env}/api/admin/login",
        json={"username": ADMIN_USER, "password": ADMIN_PASS},
        timeout=15,
    )
    if r.status_code == 404:
        pytest.skip("admin panel disabled on this instance")
    assert r.status_code == 200, r.text
    return s


def test_google_disabled_by_default(google_env):
    r = requests.get(f"{google_env}/api/auth/google", allow_redirects=False, timeout=15)
    assert r.status_code == 404
    providers = requests.get(f"{google_env}/api/auth/providers", timeout=15).json()
    assert providers["google"]["enabled"] is False


def test_google_login_flow(google_env, admin_session):
    # 管理端开启 Google 登录
    r = admin_session.post(
        f"{google_env}/api/admin/settings",
        json={
            "google_oauth_enabled": True,
            "google_client_id": "gcid",
            "google_client_secret": "gsecret-very-long",
        },
        timeout=15,
    )
    assert r.status_code == 200, r.text

    providers = requests.get(f"{google_env}/api/auth/providers", timeout=15).json()
    assert providers["google"]["enabled"] is True

    # 完整授权码流程（requests 自动跟随 假 Google → 回调 → /）
    sess = requests.Session()
    sess.get(f"{google_env}/api/auth/google", timeout=15)
    me = sess.get(f"{google_env}/api/me", timeout=15)
    assert me.status_code == 200, me.text
    assert me.json()["username"] == "bob"

    # 关闭后 start 端点恢复 404
    admin_session.post(
        f"{google_env}/api/admin/settings",
        json={"google_oauth_enabled": False, "google_client_id": "gcid"},
        timeout=15,
    )
    r = requests.get(f"{google_env}/api/auth/google", allow_redirects=False, timeout=15)
    assert r.status_code == 404
