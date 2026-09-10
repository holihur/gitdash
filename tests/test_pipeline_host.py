"""Pipeline host 执行（无 Docker）黑盒 API 测试 —— GITDASH_PIPELINE_EXEC=host 模式。

与后端完全隔离：只通过 HTTP API 验证行为。
- host 模式实例（专用模块夹具）：.gitdash.yml 省略 image 时直接在宿主 sh 执行；
- 默认实例（会话级 base_url 夹具）：省略 image 的流水线应被拒绝（host 执行未开启）。
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

GITDASH_BIN = os.environ.get("GITDASH_BIN", "").strip()

HOST_YAML = (
    "env:\n"
    "  - GREETING=hello-host\n"
    "steps:\n"
    "  - name: hello\n"
    "    run: echo $GREETING\n"
    "  - name: workdir\n"
    "    run: test -f README.md && echo workspace-ok\n"
)

NO_IMAGE_YAML = (
    "steps:\n"
    "  - name: hello\n"
    "    run: echo hi\n"
)


def _uuid() -> str:
    return uuid.uuid4().hex[:10]


def _commit(c, owner, repo, path, content, action="create"):
    c.post(
        f"/users/{owner}/repos/{repo}/commits",
        json={
            "message": f"add {path}",
            "changes": [{"path": path, "action": action, "content": content}],
        },
        expect=201,
    )


def _wait_terminal(c, owner, repo, run_id, timeout=60):
    p = f"/users/{owner}/repos/{repo}/pipeline/runs/{run_id}"
    deadline = time.time() + timeout
    while time.time() < deadline:
        run = c.get(p, expect=200)
        if hasattr(run, "json"):
            run = run.json()
        if run["status"] in ("success", "failed"):
            return run
        time.sleep(0.5)
    return run


# ---- host 模式专用实例 ----

def _free_port() -> int:
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as s:
        s.bind(("127.0.0.1", 0))
        return s.getsockname()[1]


class _Mini:
    """host 模式实例专用极简客户端。"""

    def __init__(self, base, token=None):
        self.base, self.token = base.rstrip("/"), token

    def req(self, method, path, expect, **kw):
        headers = {"Authorization": f"Bearer {self.token}"} if self.token else {}
        r = requests.request(method, f"{self.base}/api{path}", headers=headers, timeout=15, **kw)
        assert r.status_code == expect, f"{method} {path} -> {r.status_code}: {r.text}"
        return r.json() if r.text else None

    def post(self, path, **kw):
        return self.req("POST", path, kw.pop("expect", 200), **kw)

    def put(self, path, **kw):
        return self.req("PUT", path, kw.pop("expect", 200), **kw)

    def get(self, path, **kw):
        return self.req("GET", path, kw.pop("expect", 200), **kw)


@pytest.fixture(scope="module")
def host_env(tmp_path_factory):
    """独立实例：GITDASH_PIPELINE_EXEC=host（开启 host 执行）。"""
    if not GITDASH_BIN:
        pytest.skip("host mode test needs GITDASH_BIN")

    tmpdir = tmp_path_factory.mktemp("gitdash-host")
    http_port, ssh_port = _free_port(), _free_port()
    env = dict(os.environ)
    env.update(
        GITDASH_DATA=str(tmpdir / "data"),
        GITDASH_DISABLE_RATE_LIMIT="1",
        GITDASH_HTTP_ADDR=f"127.0.0.1:{http_port}",
        GITDASH_SSH_ADDR=f"127.0.0.1:{ssh_port}",
        GITDASH_PIPELINE_EXEC="host",
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
        pytest.fail(f"host-mode server not ready; see {tmpdir / 'server.log'}")

    try:
        yield base
    finally:
        proc.terminate()
        try:
            proc.wait(timeout=10)
        except subprocess.TimeoutExpired:
            proc.kill()
        log.close()


@pytest.fixture
def host_repo(host_env):
    username = f"ph-{_uuid()}"
    token = _Mini(host_env).post(
        "/auth/register", json={"username": username, "password": "test-pass-123456"}, expect=201
    )["token"]
    c = _Mini(host_env, token)
    repo = f"host-{_uuid()}"
    c.post("/repos", json={"name": repo, "template": "readme"}, expect=201)
    yield username, repo, c
    try:
        c.delete(f"/repos/{repo}", expect=204)
    except Exception:
        pass


def test_host_run_success(host_repo):
    """省略 image：宿主 sh 直接执行，env 注入与工作区均可用，运行成功。"""
    username, repo, c = host_repo
    c.put(f"/users/{username}/repos/{repo}/pipeline", json={"enabled": True}, expect=200)
    _commit(c, username, repo, ".gitdash.yml", HOST_YAML)

    run = c.post(f"/users/{username}/repos/{repo}/pipeline/runs", json={}, expect=201)
    assert run["steps_total"] == 2

    run = _wait_terminal(c, username, repo, run["id"])
    assert run["status"] == "success", run
    assert "hello-host" in run.get("log", ""), "env should be injected into host step"
    assert "workspace-ok" in run.get("log", ""), "steps should run in the checked-out workspace"
    assert "pipeline success" in run.get("log", "")


def test_host_run_step_failure(host_repo):
    """宿主步骤失败（非零退出码）→ 运行 failed，错误含退出码。"""
    username, repo, c = host_repo
    c.put(f"/users/{username}/repos/{repo}/pipeline", json={"enabled": True}, expect=200)
    _commit(c, username, repo, ".gitdash.yml", "steps:\n  - name: fail\n    run: echo boom; exit 7\n")

    run = c.post(f"/users/{username}/repos/{repo}/pipeline/runs", json={}, expect=201)
    run = _wait_terminal(c, username, repo, run["id"])
    assert run["status"] == "failed"
    assert "exit code 7" in (run.get("error") or "")
    assert "boom" in run.get("log", "")


def test_host_run_push_trigger(host_repo):
    """host 模式下 push 触发同样可用。"""
    username, repo, c = host_repo
    c.put(f"/users/{username}/repos/{repo}/pipeline", json={"enabled": True}, expect=200)
    _commit(c, username, repo, ".gitdash.yml", HOST_YAML + "# updated\n")

    p = f"/users/{username}/repos/{repo}/pipeline/runs"
    deadline = time.time() + 15
    runs = []
    while time.time() < deadline:
        runs = c.get(p, expect=200)
        if runs:
            break
        time.sleep(1)
    assert runs, "push should trigger a host-mode pipeline run"
    final = _wait_terminal(c, username, repo, runs[0]["id"])
    assert final["status"] == "success"


def test_host_repo_env_vars_injected(host_repo):
    """仓库级环境变量注入 host 步骤。"""
    username, repo, c = host_repo
    c.put(
        f"/users/{username}/repos/{repo}/env",
        json={"key": "GREETING", "value": "repo-level"},
        expect=200,
    )
    c.put(f"/users/{username}/repos/{repo}/pipeline", json={"enabled": True}, expect=200)
    _commit(c, username, repo, ".gitdash.yml", "steps:\n  - name: hello\n    run: echo G=$GREETING\n")

    run = c.post(f"/users/{username}/repos/{repo}/pipeline/runs", json={}, expect=201)
    run = _wait_terminal(c, username, repo, run["id"])
    assert run["status"] == "success", run
    assert "G=repo-level" in run.get("log", ""), run.get("log")


def test_host_repo_env_dsl_override(host_repo):
    """同 key 时 DSL env 覆盖仓库级环境变量。"""
    username, repo, c = host_repo
    c.put(
        f"/users/{username}/repos/{repo}/env",
        json={"key": "GREETING", "value": "repo-level"},
        expect=200,
    )
    c.put(f"/users/{username}/repos/{repo}/pipeline", json={"enabled": True}, expect=200)
    _commit(
        c,
        username,
        repo,
        ".gitdash.yml",
        "env:\n  - GREETING=dsl-level\nsteps:\n  - name: hello\n    run: echo G=$GREETING\n",
    )

    run = c.post(f"/users/{username}/repos/{repo}/pipeline/runs", json={}, expect=201)
    run = _wait_terminal(c, username, repo, run["id"])
    assert run["status"] == "success", run
    assert "G=dsl-level" in run.get("log", ""), run.get("log")


def test_host_disabled_rejects_no_image(base_url, user_factory):
    """默认实例（未开启 host 执行）：省略 image 的流水线记 failed 并说明原因。"""
    if not GITDASH_BIN and not os.environ.get("GITDASH_API_URL"):
        pytest.skip("needs a server instance")
    username, _, c = user_factory("pd")
    repo = f"nohost-{_uuid()}"
    c.post("/repos", json={"name": repo, "template": "readme"}, expect=201)
    try:
        c.put(f"/users/{username}/repos/{repo}/pipeline", json={"enabled": True}, expect=200)
        _commit(c, username, repo, ".gitdash.yml", NO_IMAGE_YAML)
        run = c.post(f"/users/{username}/repos/{repo}/pipeline/runs", json={}, expect=201).json()
        run = _wait_terminal(c, username, repo, run["id"], timeout=30)
        assert run["status"] == "failed"
        assert "host execution is disabled" in (run.get("error") or ""), run.get("error")
    finally:
        try:
            c.delete(f"/repos/{repo}", expect=204)
        except Exception:
            pass
