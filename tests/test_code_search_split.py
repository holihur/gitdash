"""代码搜索拆服务黑盒测试：API 节点（remote 模式）+ 独立 codeindex worker + Redis。

验证：
- push 后 API 侧只生产 asynq 任务，worker 消费并建索引；
- API 用 remote 检索到索引内容（indexed_repos > 0），本机不打开索引；
- 私有仓库经拆分链路仍不跨用户泄漏。
"""
from __future__ import annotations

import os
import shutil
import socket
import subprocess
import time
import uuid
from pathlib import Path
from urllib.parse import urlencode

import pytest
import requests

from conftest import GITDASH_BIN, ApiClient, free_port


def _uuid() -> str:
    return uuid.uuid4().hex[:10]


def _p(owner, repo, suffix=""):
    return f"/users/{owner}/repos/{repo}{suffix}"


def _write(client, owner, repo, path, content):
    client.post(
        _p(owner, repo, "/commits"),
        json={"message": "add", "changes": [{"path": path, "action": "create", "content": content}]},
        expect=201,
    )


def _wait(fn, timeout=40.0, desc="ready"):
    deadline = time.time() + timeout
    last = None
    while time.time() < deadline:
        try:
            last = fn()
            if last:
                return last
        except requests.RequestException:
            pass
        time.sleep(0.3)
    pytest.fail(f"{desc} not reached in time (last={last!r})")


def _tcp_up(port: int) -> bool:
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as s:
        s.settimeout(0.5)
        return s.connect_ex(("127.0.0.1", port)) == 0


def _spawn(binary: Path, env: dict, log_path: Path, ready, desc: str):
    log = open(log_path, "wb")
    proc = subprocess.Popen([str(binary), "serve"], env=env, stdout=log, stderr=subprocess.STDOUT)
    try:
        _wait(lambda: proc.poll() is not None or ready(), timeout=40, desc=desc)
    except BaseException:
        if proc.poll() is not None:
            log.close()
            raise RuntimeError(
                f"{desc} exited rc={proc.returncode}; log:\n{log_path.read_text(errors='replace')}"
            )
        proc.kill()
        log.close()
        raise
    if proc.poll() is not None:
        log.close()
        raise RuntimeError(f"{desc} exited rc={proc.returncode}; log:\n{log_path.read_text(errors='replace')}")
    return proc


@pytest.fixture(scope="module")
def split_env(tmp_path_factory):
    if not GITDASH_BIN:
        pytest.skip("split code-search tests need a self-spawned instance (GITDASH_BIN)")
    binary = Path(GITDASH_BIN)
    if not binary.is_file():
        pytest.fail(f"GITDASH_BIN is not a file: {binary}")
    if not shutil.which("redis-server"):
        pytest.skip("redis-server not available")

    root = tmp_path_factory.mktemp("gitdash-split")
    data = root / "data"
    data.mkdir()

    redis_port = free_port()
    redis_log = (root / "redis.log").open("wb")
    redis = subprocess.Popen(
        ["redis-server", "--port", str(redis_port), "--bind", "127.0.0.1",
         "--save", "", "--appendonly", "no"],
        stdout=redis_log, stderr=subprocess.STDOUT,
    )
    _wait(lambda: subprocess.run(
        ["redis-cli", "-p", str(redis_port), "ping"], capture_output=True
    ).stdout.strip() == b"PONG", timeout=15, desc="redis")

    search_port, api_port, ssh_port = free_port(), free_port(), free_port()
    token = f"split-{uuid.uuid4().hex}"
    common = dict(os.environ)
    common.update(
        GITDASH_DATA=str(data),
        GITDASH_QUEUE="redis",
        GITDASH_REDIS_ADDR=f"127.0.0.1:{redis_port}",
        GITDASH_DISABLE_RATE_LIMIT="1",
        GITDASH_PROFILE_REPO="0",
        GITDASH_SEARCH_TOKEN=token,
    )

    worker_env = dict(common)
    worker_env.update(
        GITDASH_ROLE="codeindex",
        GITDASH_SEARCH_LISTEN=f"127.0.0.1:{search_port}",
    )
    worker = _spawn(binary, worker_env, root / "worker.log", lambda: _tcp_up(search_port), "index worker")

    api_env = dict(common)
    api_env.update(
        GITDASH_HTTP_ADDR=f"127.0.0.1:{api_port}",
        GITDASH_SSH_ADDR=f"127.0.0.1:{ssh_port}",
        GITDASH_CODE_SEARCH="remote",
        GITDASH_SEARCH_URL=f"http://127.0.0.1:{search_port}",
    )
    api = _spawn(
        binary, api_env, root / "api.log",
        lambda: requests.get(f"http://127.0.0.1:{api_port}/api/health", timeout=1).status_code == 200,
        "api",
    )

    try:
        yield f"http://127.0.0.1:{api_port}", token
    finally:
        for p in (api, worker, redis):
            p.terminate()
        for p in (api, worker, redis):
            try:
                p.wait(timeout=10)
            except subprocess.TimeoutExpired:
                p.kill()
        redis_log.close()


def _new_user(base: str, prefix: str) -> tuple[str, ApiClient]:
    c = ApiClient(base)
    name = f"{prefix}-{_uuid()}"
    c.token = c.post(
        "/auth/register", json={"username": name, "password": "test-pass-123456"}, expect=201
    ).json()["token"]
    return name, c


def _search(client, q):
    return client.get(f"/search/code?{urlencode({'q': q})}", expect=200).json()


def test_split_indexes_and_serves_public_code(split_env):
    base, _ = split_env
    alice, alice_c = _new_user(base, "alice")
    _, bob_c = _new_user(base, "bob")
    repo = f"split-{_uuid()[:8]}"
    token = f"SPLITTOKEN{uuid.uuid4().hex[:8]}"
    alice_c.post("/repos", json={"name": repo}, expect=201)
    alice_c.post(_p(alice, repo, "/visibility"), json={"private": False}, expect=200)
    _write(alice_c, alice, repo, "src/main.go", f"package main\n// {token}\n")

    r: dict = {}
    deadline = time.time() + 40
    while time.time() < deadline:
        r = _search(bob_c, token)
        if r.get("indexed_repos", 0) > 0 and r.get("results"):
            break
        time.sleep(0.5)
    assert r.get("indexed_repos", 0) > 0, f"API did not serve from the remote index: {r}"
    assert {h["repo"] for h in r["results"]} == {repo}

    alice_c.delete(f"/repos/{repo}", expect=204)


def test_split_does_not_leak_private_repo(split_env):
    base, _ = split_env
    alice, alice_c = _new_user(base, "alice")
    _, bob_c = _new_user(base, "bob")
    repo = f"splitpriv-{_uuid()[:8]}"
    token = f"SPLITSECRET{uuid.uuid4().hex[:8]}"
    alice_c.post("/repos", json={"name": repo}, expect=201)  # 默认私有
    _write(alice_c, alice, repo, "secret.go", f"package secret\n// {token}\n")

    # alice 自己能搜到（证明已索引）。
    r = _wait(lambda: (_search(alice_c, token) or {}).get("results") or None, desc="owner indexed")
    assert {h["repo"] for h in r} == {repo}

    # bob 搜不到。
    assert _search(bob_c, token)["results"] == []

    alice_c.delete(f"/repos/{repo}", expect=204)
