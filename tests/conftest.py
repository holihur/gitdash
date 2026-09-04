"""gitdash 黑盒 API 测试夹具（pytest + requests）。

与后端完全独立 / 隔离：
- 测试只通过 HTTP API 与一个 gitdash 实例通信，不 import 任何后端代码，
  也不在测试内执行 go build / go test。
- 每次 pytest 会话只使用一个全新、自包含的实例，用例之间互不共享状态
  （用户名/仓库/issue 全部随机生成，无跨用例顺序依赖）。

实例来源（二选一，均通过环境变量注入）：
- GITDASH_BIN   指向一个已构建好的 gitdash 可执行文件：
                夹具在独立临时数据目录 + 随机端口上启动全新实例，
                会话结束自动终止并清理 —— 不影响任何开发/生产实例。
- GITDASH_API_URL 指向一个已运行的实例（例如本地 dev server 或 CI 容器）：
                仅做黑盒请求，不做任何状态清理假设。

两者都未设置时整组跳过（并给出提示）。
"""
from __future__ import annotations

import os
import socket
import subprocess
import time
import uuid
from pathlib import Path

import pytest
import requests

GITDASH_API_URL = os.environ.get("GITDASH_API_URL", "").strip()
GITDASH_BIN = os.environ.get("GITDASH_BIN", "").strip()


def free_port() -> int:
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as s:
        s.bind(("127.0.0.1", 0))
        return s.getsockname()[1]


class ApiClient:
    """极简 HTTP API 客户端：所有请求走 /api，可携带 Bearer token。"""

    def __init__(self, base: str, token: str | None = None):
        self.base = base.rstrip("/")
        self.token = token
        self.session = requests.Session()

    def request(self, method: str, path: str, *, expect: int | None = None, **kwargs):
        url = f"{self.base}/api{path}"
        headers = {"Authorization": f"Bearer {self.token}"} if self.token else {}
        resp = self.session.request(method, url, headers=headers, timeout=15, **kwargs)
        if expect is not None and resp.status_code != expect:
            raise AssertionError(
                f"{method} {path} -> {resp.status_code}, want {expect}: {resp.text}"
            )
        return resp

    def get(self, path: str, **kw):
        return self.request("GET", path, **kw)

    def post(self, path: str, **kw):
        return self.request("POST", path, **kw)

    def patch(self, path: str, **kw):
        return self.request("PATCH", path, **kw)

    def delete(self, path: str, **kw):
        return self.request("DELETE", path, **kw)


# ---- 会话级实例管理 ----

def _spawn_server(binary: Path, tmpdir: Path):
    data_dir = tmpdir / "data"
    http_port, ssh_port = free_port(), free_port()
    env = dict(os.environ)
    env.update(
        GITDASH_DATA=str(data_dir),
        GITDASH_HTTP_ADDR=f"127.0.0.1:{http_port}",
        GITDASH_SSH_ADDR=f"127.0.0.1:{ssh_port}",
    )
    log_path = tmpdir / "server.log"
    log = open(log_path, "wb")
    proc = subprocess.Popen(
        [str(binary), "serve"], env=env, stdout=log, stderr=subprocess.STDOUT
    )
    base = f"http://127.0.0.1:{http_port}"
    deadline = time.time() + 30
    while time.time() < deadline:
        if proc.poll() is not None:
            log.close()
            raise RuntimeError(
                f"server exited rc={proc.returncode}; see {log_path}"
            )
        try:
            if requests.get(f"{base}/api/health", timeout=1).status_code == 200:
                log.close()
                return base, proc
        except requests.RequestException:
            pass
        time.sleep(0.2)
    proc.kill()
    log.close()
    raise RuntimeError(f"server did not become ready in time; see {log_path}")


@pytest.fixture(scope="session")
def base_url(tmp_path_factory):
    """返回一个可用实例的 base url，并保证该实例生命周期仅限本会话。"""
    if GITDASH_API_URL:
        yield GITDASH_API_URL
        return
    if GITDASH_BIN:
        binary = Path(GITDASH_BIN)
        if not binary.is_file():
            pytest.fail(f"GITDASH_BIN is not a file: {binary}")
        base, proc = _spawn_server(binary, tmp_path_factory.mktemp("gitdash"))
        try:
            yield base
        finally:
            proc.terminate()
            try:
                proc.wait(timeout=10)
            except subprocess.TimeoutExpired:
                proc.kill()
        return
    pytest.skip(
        "API tests need a server instance: set GITDASH_BIN (prebuilt binary) "
        "or GITDASH_API_URL (already running instance)"
    )


# ---- 常用工厂 fixture ----

def _anon(base: str) -> ApiClient:
    return ApiClient(base)


@pytest.fixture
def anon(base_url) -> ApiClient:
    """无 token 客户端（用于 401 等坏路径）。"""
    return _anon(base_url)


@pytest.fixture
def client_factory(base_url):
    """client_factory(token=None) -> ApiClient"""

    def _make(token: str | None = None) -> ApiClient:
        return ApiClient(base_url, token)

    return _make


@pytest.fixture
def user_factory(client_factory):
    """user_factory(prefix='u') -> (username, token, authed_client)，用户名全局唯一。"""

    def _make(prefix: str = "u") -> tuple[str, str, ApiClient]:
        username = f"{prefix}-{uuid.uuid4().hex[:10]}"
        client = client_factory()
        resp = client.post(
            "/auth/register",
            json={"username": username, "password": "test-pass-123456"},
            expect=201,
        )
        token = resp.json()["token"]
        return username, token, client_factory(token)

    return _make


@pytest.fixture
def repo_factory(user_factory):
    """repo_factory(prefix='r') -> (repo_name, owner_client)。测试结束自动删除仓库。"""

    created: list[tuple[ApiClient, str]] = []

    def _make(prefix: str = "r") -> tuple[str, ApiClient]:
        _, _, client = user_factory()
        name = f"{prefix}-{uuid.uuid4().hex[:8]}"
        client.post("/repos", json={"name": name}, expect=201)
        created.append((client, name))
        return name, client

    yield _make
    for client, name in created:
        try:
            client.delete(f"/repos/{name}", expect=204)
        except Exception:
            pass
