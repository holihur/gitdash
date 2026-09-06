"""仓库 push mirror（同步到第三方）—— 配置 / 手动同步 / 权限 / 删除。"""

import socket
import subprocess
import time
import uuid

import pytest


def _free_port() -> int:
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as s:
        s.bind(("127.0.0.1", 0))
        return s.getsockname()[1]


def _uuid() -> str:
    return uuid.uuid4().hex[:10]


@pytest.fixture
def git_daemon_push(tmp_path):
    """本地 git:// 服务器（支持 receive-pack），返回 (mirror_url, mirror_bare_path)。"""
    base = tmp_path / "mirror-src"
    base.mkdir()
    mirror = base / "mirror.git"
    subprocess.run(
        ["git", "init", "--bare", "--initial-branch=main", str(mirror)],
        check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
    )
    port = _free_port()
    proc = subprocess.Popen(
        ["git", "daemon", "--export-all", "--enable=receive-pack",
         f"--base-path={base}", f"--port={port}", "--reuseaddr", "--listen=127.0.0.1"],
        stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
    )
    try:
        time.sleep(0.5)
        yield f"git://127.0.0.1:{port}/mirror.git", str(mirror)
    finally:
        proc.terminate()
        try:
            proc.wait(timeout=5)
        except subprocess.TimeoutExpired:
            proc.kill()


def _p(owner, repo, suffix=""):
    return f"/users/{owner}/repos/{repo}{suffix}"


def test_mirror_set_get_sync(user_factory, git_daemon_push):
    username, _, c = user_factory()
    url, mirror_bare = git_daemon_push
    repo = f"m-{_uuid()}"
    c.post("/repos", json={"name": repo, "template": "readme"}, expect=201)

    # 初始无配置
    assert c.get(_p(username, repo, "/mirror"), expect=200).json()["url"] == ""

    # 配置
    r = c.put(_p(username, repo, "/mirror"), json={"url": url}, expect=200).json()
    assert r["url"] == url
    assert c.get(_p(username, repo, "/mirror"), expect=200).json()["url"] == url

    # 手动同步（异步队列）：排队后轮询 status，目标 bare 仓库收到 main 分支
    r = c.post(_p(username, repo, "/mirror/sync"), expect=202).json()
    assert r["status"] == "queued"
    deadline = time.time() + 30
    while time.time() < deadline:
        st = c.get(_p(username, repo, "/mirror"), expect=200).json()["status"]
        if st == "synced":
            break
        assert st != "failed", "mirror sync failed"
        time.sleep(0.3)
    else:
        raise AssertionError("mirror sync not finished in 30s")
    refs = subprocess.run(
        ["git", "--git-dir", mirror_bare, "for-each-ref", "--format=%(refname:short)"],
        check=True, capture_output=True, text=True,
    ).stdout
    assert "main" in refs.split()

    c.delete(f"/repos/{repo}", expect=204)


def test_mirror_delete_and_sync_not_configured(user_factory, git_daemon_push):
    username, _, c = user_factory()
    url, _ = git_daemon_push
    repo = f"m-{_uuid()}"
    c.post("/repos", json={"name": repo}, expect=201)

    # 未配置时同步 -> 400
    c.post(_p(username, repo, "/mirror/sync"), expect=400)

    # 配置 -> 删除
    c.put(_p(username, repo, "/mirror"), json={"url": url}, expect=200)
    c.delete(_p(username, repo, "/mirror"), expect=204)
    assert c.get(_p(username, repo, "/mirror"), expect=200).json()["url"] == ""
    c.delete(f"/repos/{repo}", expect=204)


def test_mirror_requires_owner(user_factory, git_daemon_push):
    an, _, alice = user_factory("alice")
    bn, _, bob = user_factory("bob")
    url, _ = git_daemon_push
    repo = f"m-{_uuid()}"
    alice.post("/repos", json={"name": repo}, expect=201)
    # 共享给 bob 可读，但 bob 不是 owner，不能配置 / 同步 / 删除
    alice.post(_p(an, repo, "/collabs"), json={"username": bn, "permission": "read"}, expect=200)

    bob.put(_p(an, repo, "/mirror"), json={"url": url}, expect=404)
    bob.post(_p(an, repo, "/mirror/sync"), expect=404)
    bob.delete(_p(an, repo, "/mirror"), expect=404)
    alice.delete(f"/repos/{repo}", expect=204)


def test_mirror_requires_auth(anon, user_factory, git_daemon_push):
    username, _, c = user_factory()
    url, _ = git_daemon_push
    repo = f"m-{_uuid()}"
    c.post("/repos", json={"name": repo}, expect=201)
    anon.get(_p(username, repo, "/mirror"), expect=401)
    anon.put(_p(username, repo, "/mirror"), json={"url": url}, expect=401)
    anon.post(_p(username, repo, "/mirror/sync"), expect=401)
    c.delete(f"/repos/{repo}", expect=204)


def test_mirror_bad_url(user_factory, git_daemon_push):
    username, _, c = user_factory()
    repo = f"m-{_uuid()}"
    c.post("/repos", json={"name": repo}, expect=201)
    c.put(_p(username, repo, "/mirror"), json={"url": "not a url"}, expect=400)
    c.delete(f"/repos/{repo}", expect=204)
