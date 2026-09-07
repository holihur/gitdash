"""Runner（自托管 CI agent）黑盒 API 测试。

覆盖：注册 token 签发与边界（权限/一次性/过期）、runner 注册与重名、
列表与删除、WS 凭证校验、org scope 边界、runs-on 无匹配 agent 的运行失败。
无 Redis 的默认实例：注册返回 503（runner 功能需 Redis）、runs-on 立即失败；
Redis 实例（需 GITDASH_BIN + redis-server，缺一则跳过）：完整生命周期。
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

REG_TOKEN = "/runners/registration-token"


def _uuid() -> str:
    return uuid.uuid4().hex[:10]


# ---- 无 Redis（默认 memory 实例）----

def test_runner_register_disabled_without_redis(base_url, user_factory):
    """默认实例未启用 Redis：注册 503；token 签发与列表仍可用。"""
    _, _, c = user_factory("rdis")

    r = c.post(REG_TOKEN, json={"scope": "user"}, expect=201)
    token = r.json()["token"]
    assert token and r.json()["expires_at"]

    r = c.get("/runners", expect=200)
    assert isinstance(r.json(), list)

    r = c.post(
        "/runner/register",
        json={"name": f"rr-{_uuid()}", "labels": ["docker"], "token": token},
        expect=None,
    )
    if r.status_code == 503:
        assert r.json()["code"] == "runner_disabled"
        c.post(
            "/runner/register",
            json={"name": f"rr-{_uuid()}", "token": "whatever"},
            expect=503,
        )
    else:
        pytest.skip("instance has redis enabled; covered by redis tests")


def test_runner_token_scopes_and_auth(user_factory, anon):
    """token 签发边界：未登录 401、scope 非法 400、org 非 owner 404。"""
    _, _, c = user_factory("rtok")
    anon.post(REG_TOKEN, json={"scope": "user"}, expect=401)
    c.post(REG_TOKEN, json={"scope": "global"}, expect=400)
    c.post(REG_TOKEN, json={"scope": "org", "org": f"no-org-{_uuid()}"}, expect=404)


def test_runner_org_scope_requires_owner(user_factory, client_factory):
    """org owner 可签发组织 scope token；普通成员不可。"""
    _, owner_name, owner = user_factory("rorg")
    username, _, member = user_factory("rmem")
    org = f"rorg-{_uuid()}"
    owner.post("/orgs", json={"name": org, "display": "Org"}, expect=201)
    owner.post(f"/orgs/{org}/members", json={"username": username, "role": "member"}, expect=200)

    member.post(REG_TOKEN, json={"scope": "org", "org": org}, expect=404)
    r = owner.post(REG_TOKEN, json={"scope": "org", "org": org}, expect=201)
    assert r.json()["scope"] == f"org:{org}"


def test_runner_token_one_shot_invalid(base_url, user_factory):
    """无效/已消费 token 注册被拒（503 场景除外 —— 无 redis 时注册不校验 token）。"""
    _, _, c = user_factory("rbad")
    r = c.post(REG_TOKEN, json={"scope": "user"}, expect=201)
    token = r.json()["token"]

    r = c.post(
        "/runner/register",
        json={"name": f"rb-{_uuid()}", "token": "bogus-token"},
        expect=None,
    )
    if r.status_code == 503:
        pytest.skip("instance has no redis; token validation covered by redis tests")

    # 有效 token 消费一次后失效
    r2 = c.post("/runner/register", json={"name": f"rb-{_uuid()}", "token": token}, expect=None)
    assert r2.status_code == 201
    c.post("/runner/register", json={"name": f"rb-{_uuid()}", "token": token}, expect=403)


def test_runner_delete_not_found(user_factory):
    _, _, c = user_factory("rdel")
    c.delete(f"/runners/ghost-{_uuid()}", expect=404)


def test_runson_no_agent_fails_immediately(base_url, user_factory, repo_factory):
    """runs-on 指定标签但无在线 agent：运行立即记 failed（memory 实例为 disabled）。"""
    name, c = repo_factory("rono")
    owner = c.get("/me", expect=200).json()["username"]

    yaml = "image: alpine:3.19\nruns-on: [docker]\nsteps:\n  - name: build\n    run: echo ok\n"
    c.put(f"/users/{owner}/repos/{name}/pipeline", json={"enabled": True}, expect=200)
    c.post(
        f"/users/{owner}/repos/{name}/commits",
        json={
            "message": "add pipeline",
            "changes": [{"path": ".gitdash.yml", "action": "create", "content": yaml}],
        },
        expect=201,
    )
    run = c.post(f"/users/{owner}/repos/{name}/pipeline/runs", json={}, expect=201).json()

    deadline = time.time() + 30
    detail = None
    while time.time() < deadline:
        detail = c.get(
            f"/users/{owner}/repos/{name}/pipeline/runs/{run['id']}", expect=200
        ).json()
        if detail["status"] == "failed":
            break
        time.sleep(0.3)
    assert detail and detail["status"] == "failed", detail
    assert "runner" in detail["error"].lower() or "disabled" in detail["error"].lower(), detail


# ---- Redis 实例（完整生命周期）----

def _free_port() -> int:
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as s:
        s.bind(("127.0.0.1", 0))
        return s.getsockname()[1]


def _which(name: str) -> str | None:
    for d in os.environ.get("PATH", "").split(os.pathsep):
        p = Path(d) / name
        if p.is_file():
            return str(p)
    return None


@pytest.fixture(scope="module")
def runner_env(tmp_path_factory):
    """独立实例：GITDASH_QUEUE=redis + 临时 redis-server（runner 功能启用）。"""
    if not GITDASH_BIN:
        pytest.skip("runner tests need GITDASH_BIN")
    redis_bin = _which("redis-server") or (
        "/usr/bin/redis-server" if Path("/usr/bin/redis-server").is_file() else None
    )
    if not redis_bin:
        pytest.skip("redis-server not found")

    rport = _free_port()
    rproc = subprocess.Popen(
        [redis_bin, "--port", str(rport), "--save", "", "--appendonly", "no"],
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
    )
    deadline = time.time() + 5
    while time.time() < deadline:
        try:
            socket.create_connection(("127.0.0.1", rport), timeout=1).close()
            break
        except OSError:
            if rproc.poll() is not None:
                pytest.skip("redis-server exited immediately")
            time.sleep(0.1)
    else:
        rproc.kill()
        pytest.skip("redis not reachable")

    tmpdir = tmp_path_factory.mktemp("gitdash-runners")
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
    proc = subprocess.Popen(
        [GITDASH_BIN, "serve"], env=env, stdout=log, stderr=subprocess.STDOUT
    )
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
        pytest.fail(f"runner-mode server not ready; see {tmpdir / 'server.log'}")

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


def _client(base: str, token: str | None = None):
    s = requests.Session()

    def req(method, path, expect=None, **kw):
        headers = {"Authorization": f"Bearer {token}"} if token else {}
        r = s.request(method, f"{base}/api{path}", headers=headers, timeout=15, **kw)
        if expect is not None:
            assert r.status_code == expect, f"{method} {path} -> {r.status_code}: {r.text}"
        return r

    class C:
        @staticmethod
        def request(method, path, expect=None, **kw):
            return req(method, path, expect, **kw)

        @staticmethod
        def get(path, expect=None, **kw):
            return req("GET", path, expect, **kw)

        @staticmethod
        def post(path, expect=None, **kw):
            return req("POST", path, expect, **kw)

        @staticmethod
        def put(path, expect=None, **kw):
            return req("PUT", path, expect, **kw)

        @staticmethod
        def delete(path, expect=None, **kw):
            return req("DELETE", path, expect, **kw)

    return C()


def test_runner_full_lifecycle(runner_env):
    """注册 → 列表 → WS 凭证校验 → 删除；重名冲突；token 一次性；跨用户隔离。"""
    base = runner_env

    def new_user(prefix):
        username = f"{prefix}-{_uuid()}"
        token = _client(base).post(
            "/auth/register", json={"username": username, "password": "test-pass-123456"}, expect=201
        ).json()["token"]
        return username, _client(base, token)

    _, c = new_user("rlc")

    r = c.post(REG_TOKEN, json={"scope": "user"}, expect=201)
    token = r.json()["token"]
    rname = f"rlc-{_uuid()}"

    # 注册成功：secret 只返回一次
    r = c.post(
        "/runner/register", json={"name": rname, "labels": ["docker", "go1.22"], "token": token}, expect=201
    ).json()
    secret = r["secret"]
    assert secret and r["runner"]["name"] == rname
    assert r["runner"]["scope"] == f"user:{c.get('/me', expect=200).json()['username']}"
    assert r["runner"]["labels"] == ["docker", "go1.22"]
    assert r["runner"]["status"] == "offline"

    # 同名再注册 → 409（token 已在注册流程中被消费）；重放 token → 403
    r2 = c.post(REG_TOKEN, json={"scope": "user"}, expect=201).json()["token"]
    c.post("/runner/register", json={"name": rname, "token": r2}, expect=409)
    c.post("/runner/register", json={"name": rname + "-x", "token": r2}, expect=403)

    # 列表可见（个人 scope）
    runners = c.get("/runners", expect=200).json()
    assert any(x["name"] == rname for x in runners)

    # WS：无凭证 401、坏 secret 401
    _client(base).get("/runner/ws", expect=401)
    r = requests.get(
        f"{base}/api/runner/ws",
        headers={"Authorization": f"Bearer {rname}:wrong-secret"},
        timeout=15,
    )
    assert r.status_code == 401

    # 删除后列表消失
    c.delete(f"/runners/{rname}", expect=200)
    runners = c.get("/runners", expect=200).json()
    assert not any(x["name"] == rname for x in runners)

    # 他人不可见/不可删我的 runner
    _, other = new_user("rlc2")
    r3 = c.post(REG_TOKEN, json={"scope": "user"}, expect=201).json()["token"]
    rname2 = f"rlc2-{_uuid()}"
    c.post("/runner/register", json={"name": rname2, "token": r3}, expect=201)
    others = other.get("/runners", expect=200).json()
    assert not any(x["name"] == rname2 for x in others)
    other.delete(f"/runners/{rname2}", expect=404)


# ---- 真实 agent 黑盒（gitdash-runner 二进制端到端）----

def _agent_bin(tmpdir: Path) -> str:
    """agent 二进制：GITDASH_RUNNER_BIN 优先；否则若源码可用则现场构建（一次性）。"""
    env_bin = os.environ.get("GITDASH_RUNNER_BIN", "").strip()
    if env_bin:
        return env_bin
    backend = Path(__file__).resolve().parent.parent / "backend"
    src = backend / "cmd" / "gitdash-runner" / "main.go"
    go_bin = _which("go")
    if src.is_file() and go_bin:
        out = tmpdir / "gitdash-runner"
        r = subprocess.run([go_bin, "build", "-o", str(out), "./cmd/gitdash-runner"],
                           cwd=backend, capture_output=True, text=True)
        if r.returncode == 0 and out.is_file():
            return str(out)
    pytest.skip("agent binary not available: set GITDASH_RUNNER_BIN (or keep backend/ sources + go)")


def _wait_online(base: str, c, name: str, timeout: float = 15.0) -> None:
    deadline = time.time() + timeout
    while time.time() < deadline:
        for r in c.get("/runners", expect=200).json():
            if r["name"] == name and r["status"] == "online":
                return
        time.sleep(0.3)
    pytest.fail(f"runner {name} did not come online in {timeout}s")


def _spawn_agent(base: str, agent_bin: str, home: Path, name: str, labels: str, token: str):
    """注册并启动一个 agent 进程（HOME 隔离，配置不落到开发机）。"""
    env = dict(os.environ)
    env["HOME"] = str(home)
    r = subprocess.run(
        [agent_bin, "register", "-server", base, "-name", name,
         "-labels", labels, "-token", token],
        env=env, capture_output=True, text=True, timeout=30,
    )
    assert r.returncode == 0, f"register failed: {r.stdout} {r.stderr}"
    log = open(home / "agent.log", "wb")
    proc = subprocess.Popen([agent_bin, "run"], env=env, stdout=log, stderr=subprocess.STDOUT)
    return proc, log


def _wait_run(base, c, owner, repo, run_id: int, status: str, timeout: float):
    deadline = time.time() + timeout
    detail = None
    while time.time() < deadline:
        detail = c.get(
            f"/users/{owner}/repos/{repo}/pipeline/runs/{run_id}", expect=200
        ).json()
        if detail["status"] == status:
            return detail
        time.sleep(0.5)
    pytest.fail(f"run {run_id} not {status} in {timeout}s: {detail}")


def test_remote_pipeline_end_to_end(runner_env, tmp_path):
    """真实 agent 端到端：注册 → 领取任务 → 容器执行 → 日志/状态/runner 名回传。"""
    base = runner_env
    agent_bin = _agent_bin(tmp_path)

    username = f"re2e-{_uuid()}"
    c = _client(base).post(
        "/auth/register", json={"username": username, "password": "test-pass-123456"}, expect=201
    )
    c = _client(base, c.json()["token"])
    repo = f"re2e-{_uuid()}"
    c.post("/repos", json={"name": repo, "template": "readme"}, expect=201)

    token = c.post(REG_TOKEN, json={"scope": "user"}, expect=201).json()["token"]
    rname = f"re2e-{_uuid()}"
    agent_proc, agent_log = _spawn_agent(base, agent_bin, tmp_path / "agent", rname, "docker", token)
    try:
        _wait_online(base, c, rname)

        yaml = (
            "image: alpine:3.19\n"
            "runs-on: [docker]\n"
            "steps:\n"
            "  - name: build\n    run: echo hello-from-agent\n"
            "  - name: test\n    run: echo tests-ok\n"
        )
        c.put(f"/users/{username}/repos/{repo}/pipeline", json={"enabled": True}, expect=200)
        c.post(
            f"/users/{username}/repos/{repo}/commits",
            json={
                "message": "add pipeline",
                "changes": [{"path": ".gitdash.yml", "action": "create", "content": yaml}],
            },
            expect=201,
        )
        run = c.post(f"/users/{username}/repos/{repo}/pipeline/runs", json={}, expect=201).json()

        detail = _wait_run(base, c, username, repo, run["id"], "success", timeout=180)
        assert detail["runner_name"] == rname
        assert detail["steps_done"] == detail["steps_total"] == 2
        assert "hello-from-agent" in detail["log"]
        assert "tests-ok" in detail["log"]

        # runner 列表显示在线
        online = [r for r in c.get("/runners", expect=200).json() if r["name"] == rname]
        assert online and online[0]["status"] == "online"
        assert online[0]["labels"] == ["docker"]
    finally:
        agent_proc.terminate()
        try:
            agent_proc.wait(timeout=10)
        except subprocess.TimeoutExpired:
            agent_proc.kill()
        agent_log.close()


def test_remote_pipeline_cancel(runner_env, tmp_path):
    """运行中取消：agent 杀容器，run 记 failed（cancelled）。"""
    base = runner_env
    agent_bin = _agent_bin(tmp_path)

    username = f"rcxl-{_uuid()}"
    tok = _client(base).post(
        "/auth/register", json={"username": username, "password": "test-pass-123456"}, expect=201
    ).json()["token"]
    c = _client(base, tok)
    repo = f"rcxl-{_uuid()}"
    c.post("/repos", json={"name": repo, "template": "readme"}, expect=201)

    token = c.post(REG_TOKEN, json={"scope": "user"}, expect=201).json()["token"]
    rname = f"rcxl-{_uuid()}"
    agent_proc, agent_log = _spawn_agent(base, agent_bin, tmp_path / "agent", rname, "docker", token)
    try:
        _wait_online(base, c, rname)
        yaml = (
            "image: alpine:3.19\n"
            "runs-on: [docker]\n"
            "steps:\n  - name: sleep\n    run: sleep 120\n"
        )
        c.put(f"/users/{username}/repos/{repo}/pipeline", json={"enabled": True}, expect=200)
        c.post(
            f"/users/{username}/repos/{repo}/commits",
            json={
                "message": "add pipeline",
                "changes": [{"path": ".gitdash.yml", "action": "create", "content": yaml}],
            },
            expect=201,
        )
        run = c.post(f"/users/{username}/repos/{repo}/pipeline/runs", json={}, expect=201).json()
        _wait_run(base, c, username, repo, run["id"], "running", timeout=120)

        c.post(f"/users/{username}/repos/{repo}/pipeline/runs/{run['id']}/cancel", expect=200)
        detail = _wait_run(base, c, username, repo, run["id"], "failed", timeout=60)
        assert "cancel" in detail["error"].lower(), detail
    finally:
        agent_proc.terminate()
        try:
            agent_proc.wait(timeout=10)
        except subprocess.TimeoutExpired:
            agent_proc.kill()
        agent_log.close()


def test_agent_offline_fails_running(runner_env, tmp_path):
    """agent 执行中掉线（进程被杀）：运行在 sweeper 周期内记 failed (offline)。"""
    base = runner_env
    agent_bin = _agent_bin(tmp_path)

    username = f"roff-{_uuid()}"
    tok = _client(base).post(
        "/auth/register", json={"username": username, "password": "test-pass-123456"}, expect=201
    ).json()["token"]
    c = _client(base, tok)
    repo = f"roff-{_uuid()}"
    c.post("/repos", json={"name": repo, "template": "readme"}, expect=201)

    token = c.post(REG_TOKEN, json={"scope": "user"}, expect=201).json()["token"]
    rname = f"roff-{_uuid()}"
    agent_proc, agent_log = _spawn_agent(base, agent_bin, tmp_path / "agent", rname, "docker", token)
    try:
        _wait_online(base, c, rname)
        yaml = (
            "image: alpine:3.19\n"
            "runs-on: [docker]\n"
            "steps:\n  - name: sleep\n    run: sleep 300\n"
        )
        c.put(f"/users/{username}/repos/{repo}/pipeline", json={"enabled": True}, expect=200)
        c.post(
            f"/users/{username}/repos/{repo}/commits",
            json={
                "message": "add pipeline",
                "changes": [{"path": ".gitdash.yml", "action": "create", "content": yaml}],
            },
            expect=201,
        )
        run = c.post(f"/users/{username}/repos/{repo}/pipeline/runs", json={}, expect=201).json()
        _wait_run(base, c, username, repo, run["id"], "running", timeout=120)

        agent_proc.kill()  # 模拟机器断电（非优雅退出）
        agent_proc.wait(timeout=10)

        detail = _wait_run(base, c, username, repo, run["id"], "failed", timeout=60)
        assert "offline" in detail["error"].lower(), detail
    finally:
        if agent_proc.poll() is None:
            agent_proc.kill()
        agent_log.close()
