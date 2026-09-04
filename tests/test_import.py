"""仓库导入 —— 公开仓库 git:// 导入 + bad path（URL 校验、冲突、权限）。"""

import os
import socket
import subprocess
import tempfile
import time
import uuid

import pytest


def _free_port() -> int:
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as s:
        s.bind(("127.0.0.1", 0))
        return s.getsockname()[1]


def _run(cmd, **kw):
    subprocess.run(cmd, check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, **kw)


@pytest.fixture
def git_daemon():
    """本地 git:// 服务器，托管一个含 README 的 bare 仓库，返回其 URL。"""
    base = tempfile.mkdtemp(prefix="gitdash-import-src-")
    repo = os.path.join(base, "upstream.git")
    _run(["git", "init", "--bare", "--initial-branch=main", repo])

    work = tempfile.mkdtemp(prefix="gitdash-import-work-")
    _run(["git", "init", "--initial-branch=main", work])
    with open(os.path.join(work, "README.md"), "w") as f:
        f.write("# upstream\n")
    _run(["git", "-C", work, "config", "user.name", "test"])
    _run(["git", "-C", work, "config", "user.email", "t@example.com"])
    _run(["git", "-C", work, "add", "README.md"])
    _run(["git", "-C", work, "commit", "-m", "init"])
    _run(["git", "-C", work, "push", repo, "HEAD:refs/heads/main"])

    port = _free_port()
    proc = subprocess.Popen(
        ["git", "daemon", "--export-all", f"--base-path={base}",
         f"--port={port}", "--reuseaddr", "--listen=127.0.0.1"],
        stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
    )
    try:
        time.sleep(0.5)
        yield f"git://127.0.0.1:{port}/upstream.git"
    finally:
        proc.terminate()
        try:
            proc.wait(timeout=5)
        except subprocess.TimeoutExpired:
            proc.kill()


def _uuid() -> str:
    return uuid.uuid4().hex[:10]


def test_import_public_repo(user_factory, git_daemon):
    username, _, c = user_factory()
    name = f"imp-{_uuid()}"
    r = c.post("/imports", json={"url": git_daemon, "name": name}, expect=201).json()
    assert r["owner"] == username and r["name"] == name

    # 来源已记录
    repo = c.get(f"/repos/{name}", expect=200).json()
    assert repo["import_url"] == git_daemon

    # 分支与内容一致
    branches = c.get(f"/repos/{name}/branches", expect=200).json()
    assert [b["name"] for b in branches] == ["main"]
    blob = c.get(f"/repos/{name}/blob?ref=main&path=README.md", expect=200).json()
    assert blob["content"] == "# upstream\n"
    c.delete(f"/repos/{name}", expect=204)


def test_import_infers_name_and_conflict(user_factory, git_daemon):
    _, _, c = user_factory()
    # 名称从 URL 推断（upstream）
    r = c.post("/imports", json={"url": git_daemon}, expect=201).json()
    assert r["name"] == "upstream"

    # 同名再次导入冲突
    c.post("/imports", json={"url": git_daemon}, expect=409)
    c.delete("/repos/upstream", expect=204)


def test_import_bad_urls(user_factory):
    _, _, c = user_factory()
    c.post("/imports", json={"url": ""}, expect=400)
    c.post("/imports", json={"url": "not a url"}, expect=400)
    c.post("/imports", json={"url": "ftp://example.com/x.git"}, expect=400)
    # 无 body
    c.post("/imports", json={}, expect=400)


def test_import_requires_auth(anon, git_daemon):
    anon.post("/imports", json={"url": git_daemon}, expect=401)
