"""Pipeline（CI）黑盒 API 测试 —— 配置开关 / 手动与 push 触发 / 运行记录 / 级联 / 队列模式。

与后端完全隔离：只通过 HTTP API 验证行为。
- docker 缺失时执行型用例以 failed("docker not available") 快速终态，同样可断言；
- asynq/redis 队列模式需要 GITDASH_BIN + redis-server，两者缺一则跳过。
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

PIPELINE_YAML = (
    "image: gitdash-ci-nonexistent-image-x\n"
    "env:\n"
    "  - CGO_ENABLED=0\n"
    "steps:\n"
    "  - name: build\n"
    "    run: echo build\n"
    "  - name: test\n"
    "    run: echo test\n"
)

BAD_PIPELINE_YAML = "foo: bar\n"


def _uuid() -> str:
    return uuid.uuid4().hex[:10]


def _p(owner, repo, suffix=""):
    return f"/users/{owner}/repos/{repo}/pipeline{suffix}"


def _commit(c, owner, repo, path, content, action="create"):
    c.post(
        f"/users/{owner}/repos/{repo}/commits",
        json={
            "message": f"add {path}",
            "changes": [{"path": path, "action": action, "content": content}],
        },
        expect=201,
    )


def _wait_terminal(c, owner, repo, run_id, timeout=150):
    """轮询到终态（success/failed）；超时返回当前状态由调用方断言。"""
    deadline = time.time() + timeout
    while time.time() < deadline:
        run = c.get(_p(owner, repo, f"/runs/{run_id}"), expect=200).json()
        if run["status"] in ("success", "failed"):
            return run
        time.sleep(0.5)
    return run


@pytest.fixture
def pl_env(user_factory):
    an, _, alice = user_factory("pa")
    bn, _, bob = user_factory("pb")
    repo = f"ci-{_uuid()}"
    # 公开仓库：便于验证非 owner 只读行为
    alice.post("/repos", json={"name": repo, "private": False, "template": "readme"}, expect=201)
    yield an, bn, alice, bob, repo
    try:
        alice.delete(f"/repos/{repo}", expect=204)
    except Exception:
        pass


# ---- 配置开关 ----

def test_pipeline_default_off_and_toggle(pl_env):
    an, _, alice, _, repo = pl_env

    cfg = alice.get(_p(an, repo), expect=200).json()
    assert cfg["enabled"] is False
    assert cfg["file"] == ".gitdash.yml"

    assert alice.put(_p(an, repo), json={"enabled": True}, expect=200).json()["enabled"] is True
    assert alice.get(_p(an, repo), expect=200).json()["enabled"] is True

    assert alice.put(_p(an, repo), json={"enabled": False}, expect=200).json()["enabled"] is False
    assert alice.get(_p(an, repo), expect=200).json()["enabled"] is False


def test_pipeline_permissions(pl_env, anon):
    an, bn, alice, bob, repo = pl_env

    # 公开仓库：非 owner 可读配置与运行列表，但不可写
    assert bob.get(_p(an, repo), expect=200).json()["enabled"] is False
    bob.put(_p(an, repo), json={"enabled": True}, expect=404)
    bob.post(_p(an, repo, "/runs"), json={}, expect=404)

    # 转私有后：非 owner 读也被拒
    alice.post(f"/users/{an}/repos/{repo}/visibility", json={"private": True}, expect=200)
    bob.get(_p(an, repo), expect=404)
    bob.get(_p(an, repo, "/runs"), expect=404)

    # 不存在的仓库
    alice.get(_p(an, "nope"), expect=404)
    # 未登录
    anon.get(_p(an, repo), expect=401)
    anon.put(_p(an, repo), json={"enabled": True}, expect=401)


def test_pipeline_visibility_route(pl_env):
    an, _, alice, _, repo = pl_env
    alice.post(f"/users/{an}/repos/{repo}/visibility", json={"private": True}, expect=200)
    assert alice.get(f"/users/{an}/repos/{repo}", expect=200).json()["private"] is True


# ---- 手动触发与运行记录 ----

def test_pipeline_trigger_requires_dsl_file(pl_env):
    an, _, alice, _, repo = pl_env
    resp = alice.post(_p(an, repo, "/runs"), json={}, expect=400).json()
    assert resp["code"] == "pipeline_file_missing"


def test_pipeline_bad_dsl_recorded_failed(pl_env):
    an, _, alice, _, repo = pl_env
    _commit(alice, an, repo, ".gitdash.yml", BAD_PIPELINE_YAML)

    run = alice.post(_p(an, repo, "/runs"), json={}, expect=201).json()
    assert run["id"] >= 1 and run["ref"] == "main"
    assert run["steps_total"] == 0

    run = _wait_terminal(alice, an, repo, run["id"], timeout=15)
    assert run["status"] == "failed"
    assert run["error"]  # DSL 错误信息可见

    # 运行列表
    runs = alice.get(_p(an, repo, "/runs"), expect=200).json()
    assert [r["id"] for r in runs] == [run["id"]]

    # 不存在的运行
    alice.get(_p(an, repo, "/runs/999999"), expect=404)
    alice.get(_p(an, repo, "/runs/not-a-number"), expect=400)


def test_pipeline_run_execution(pl_env):
    an, _, alice, _, repo = pl_env
    _commit(alice, an, repo, ".gitdash.yml", PIPELINE_YAML)

    run = alice.post(_p(an, repo, "/runs"), json={}, expect=201).json()
    assert run["status"] in ("pending", "running", "failed")  # 入队即返回
    assert run["steps_total"] == 2
    assert run["sha"]
    assert run["ref"] == "main"
    assert "trigger_by" in run

    run = _wait_terminal(alice, an, repo, run["id"])
    assert run["status"] == "failed"  # 镜像拉取失败 / docker 缺失，均为快速终态
    assert run["finished_at"]
    assert isinstance(run.get("log", ""), str)


def test_pipeline_push_trigger(pl_env):
    an, _, alice, _, repo = pl_env

    # 未开启：push 不触发
    _commit(alice, an, repo, ".gitdash.yml", PIPELINE_YAML)
    time.sleep(6)
    assert alice.get(_p(an, repo, "/runs"), expect=200).json() == []

    # 开启后再 push：自动出现一条运行
    alice.put(_p(an, repo), json={"enabled": True}, expect=200)
    _commit(alice, an, repo, ".gitdash.yml", PIPELINE_YAML + "# updated\n", action="update")

    deadline = time.time() + 15
    runs = []
    while time.time() < deadline:
        runs = alice.get(_p(an, repo, "/runs"), expect=200).json()
        if runs:
            break
        time.sleep(1)
    assert runs, "push should trigger a pipeline run when enabled"
    assert runs[0]["ref"] == "main"
    final = _wait_terminal(alice, an, repo, runs[0]["id"])
    assert final["status"] == "failed"


def test_pipeline_repo_delete_cascades(pl_env):
    an, _, alice, _, repo = pl_env
    alice.put(_p(an, repo), json={"enabled": True}, expect=200)
    _commit(alice, an, repo, ".gitdash.yml", BAD_PIPELINE_YAML)
    run = alice.post(_p(an, repo, "/runs"), json={}, expect=201).json()

    alice.delete(f"/repos/{repo}", expect=204)
    alice.get(_p(an, repo), expect=404)
    alice.get(_p(an, repo, "/runs"), expect=404)
    alice.get(_p(an, repo, f"/runs/{run['id']}"), expect=404)

    # 重建同名仓库：默认关闭、无残留运行
    alice.post("/repos", json={"name": repo}, expect=201)
    assert alice.get(_p(an, repo), expect=200).json()["enabled"] is False
    assert alice.get(_p(an, repo, "/runs"), expect=200).json() == []
    alice.delete(f"/repos/{repo}", expect=204)


# ---- 队列模式（asynq + redis）----

def _free_port() -> int:
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as s:
        s.bind(("127.0.0.1", 0))
        return s.getsockname()[1]


class _Mini:
    """队列模式专用极简客户端（不与主夹具耦合）。"""

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
def queue_env(tmp_path_factory):
    """独立实例：GITDASH_QUEUE=redis + 临时 redis-server。"""
    if not GITDASH_BIN:
        pytest.skip("queue mode test needs GITDASH_BIN")
    redis_bin = _which("redis-server")
    if not redis_bin and Path("/usr/bin/redis-server").is_file():
        redis_bin = "/usr/bin/redis-server"
    if not redis_bin:
        pytest.skip("redis-server not found")

    rport = _free_port()
    rproc = subprocess.Popen(
        [redis_bin, "--port", str(rport), "--save", "", "--appendonly", "no"],
        stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
    )
    deadline = time.time() + 5
    while time.time() < deadline:
        try:
            c = socket.create_connection(("127.0.0.1", rport), timeout=1)
            c.close()
            break
        except OSError:
            if rproc.poll() is not None:
                pytest.skip("redis-server exited immediately")
            time.sleep(0.1)
    else:
        rproc.kill()
        pytest.skip("redis not reachable")

    tmpdir = tmp_path_factory.mktemp("gitdash-queue")
    http_port, ssh_port = _free_port(), _free_port()
    env = dict(os.environ)
    env.update(
        GITDASH_DATA=str(tmpdir / "data"),
        GITDASH_HTTP_ADDR=f"127.0.0.1:{http_port}",
        GITDASH_SSH_ADDR=f"127.0.0.1:{ssh_port}",
        GITDASH_QUEUE="redis",
        GITDASH_REDIS_ADDR=f"127.0.0.1:{rport}",
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
        rproc.kill()
        log.close()
        pytest.fail(f"queue-mode server not ready; see {tmpdir / 'server.log'}")

    try:
        yield base
    finally:
        proc.terminate()
        try:
            proc.wait(timeout=10)
        except subprocess.TimeoutExpired:
            proc.kill()
        rproc.kill()
        log.close()


def _which(name: str) -> str | None:
    for d in os.environ.get("PATH", "").split(os.pathsep):
        p = Path(d) / name
        if p.is_file():
            return str(p)
    return None


def test_pipeline_run_via_redis_queue(queue_env):
    username = f"pq-{_uuid()}"
    token = _Mini(queue_env).post(
        "/auth/register", json={"username": username, "password": "test-pass-123456"}, expect=201
    )["token"]
    c = _Mini(queue_env, token)
    repo = f"ciq-{_uuid()}"
    c.post("/repos", json={"name": repo, "template": "readme"}, expect=201)

    c.put(f"/users/{username}/repos/{repo}/pipeline", json={"enabled": True}, expect=200)
    c.post(
        f"/users/{username}/repos/{repo}/commits",
        json={
            "message": "add pipeline",
            "changes": [{"path": ".gitdash.yml", "action": "create", "content": PIPELINE_YAML}],
        },
        expect=201,
    )

    run = c.post(f"/users/{username}/repos/{repo}/pipeline/runs", json={}, expect=201)
    assert run["id"] >= 1

    deadline = time.time() + 150
    while time.time() < deadline:
        run = c.get(f"/users/{username}/repos/{repo}/pipeline/runs/{run['id']}", expect=200)
        if run["status"] in ("success", "failed"):
            break
        time.sleep(0.5)
    assert run["status"] == "failed", "job should be consumed from redis and executed"
    assert run["steps_total"] == 2
