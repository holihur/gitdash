"""Git Smart HTTP（HTTPS 克隆/推送）—— 黑盒测试。

像 GitHub/GitLab 一样：`git clone http(s)://host/owner/repo.git`，用 PAT 作
Basic 密码（用户名任意）。实例为 HTTP（HTTPS 由服务端 TLS/反代终止，处理逻辑相同）。
"""

import os
import shutil
import subprocess
import uuid as _uuid
from urllib.parse import urlparse

import pytest

pytestmark = pytest.mark.skipif(shutil.which("git") is None, reason="git required")


def _git(cwd, args, *, check=True):
    env = dict(os.environ)
    env["GIT_TERMINAL_PROMPT"] = "0"  # 401 时不要交互式要凭据
    r = subprocess.run(["git", *args], cwd=str(cwd), env=env, capture_output=True, text=True)
    if check and r.returncode != 0:
        raise AssertionError(f"git {' '.join(args)} failed rc={r.returncode}: {r.stderr}")
    return r


def _commit(work, msg):
    _git(work, ["add", "-A"])
    _git(work, ["-c", "user.name=u", "-c", "user.email=u@e.c", "commit", "-q", "-m", msg])


def _url(base_url, owner, repo, user=None, password=None):
    u = urlparse(base_url)
    auth = f"{user}:{password}@" if user is not None else ""
    return f"{u.scheme}://{auth}{u.netloc}/{owner}/{repo}.git"


def test_http_clone_push_and_auth(user_factory, base_url, tmp_path):
    username, token, client = user_factory("http")
    repo = f"h-{_uuid.uuid4().hex[:8]}"
    client.post("/repos", json={"name": repo}, expect=201)
    try:
        # 匿名：git 拿到 401 并以非 0 退出
        assert _git(tmp_path, ["clone", "-q", _url(base_url, username, repo), "anon"], check=False).returncode != 0

        # 认证 clone（空仓库）→ 提交 → push
        work = tmp_path / "work"
        _git(tmp_path, ["clone", "-q", _url(base_url, username, repo, username, token), "work"])
        (work / "README.md").write_text("# http\n")
        (work / "main.go").write_text("package main\nfunc main(){}\n")
        _commit(work, "init")
        _git(work, ["push", "-q", "origin", "HEAD:main"])

        # 服务端立即可见（push 后 refs 缓存已失效）
        branches = client.get(f"/users/{username}/repos/{repo}/branches", expect=200).json()
        assert "main" in [b["name"] for b in branches]

        # 再 clone 校验内容
        again = tmp_path / "again"
        _git(tmp_path, ["clone", "-q", _url(base_url, username, repo, username, token), "again"])
        assert (again / "README.md").read_text() == "# http\n"

        # 错误凭据 → 401
        bad = _url(base_url, username, repo, username, "wrong-token")
        assert _git(tmp_path, ["clone", "-q", bad, "bad"], check=False).returncode != 0
    finally:
        client.delete(f"/repos/{repo}", expect=204)


def test_http_pat_arbitrary_username(user_factory, base_url, tmp_path):
    """GitHub/GitLab 风格：Basic 用户名任意，密码是 PAT。"""
    username, _, client = user_factory("http")
    repo = f"h-{_uuid.uuid4().hex[:8]}"
    client.post("/repos", json={"name": repo}, expect=201)
    try:
        pat = client.post("/tokens", json={"name": "cli", "scopes": ["repo"]}, expect=201).json()["token"]
        # 任意用户名 + PAT
        _git(tmp_path, ["clone", "-q", _url(base_url, username, repo, "anything", pat), "w"])
        work = tmp_path / "w"
        (work / "a.txt").write_text("x\n")
        _commit(work, "x")
        _git(work, ["push", "-q", "origin", "HEAD:main"])

        # scope 不含 repo 的 PAT：info/refs 403
        narrow = client.post("/tokens", json={"name": "keys-only", "scopes": ["keys"]}, expect=201).json()["token"]
        r = _git(tmp_path, ["clone", "-q", _url(base_url, username, repo, "x", narrow), "deny"], check=False)
        assert r.returncode != 0
    finally:
        client.delete(f"/repos/{repo}", expect=204)


def _clone_ok(tmp_path, url, dest):
    return _git(tmp_path, ["clone", "-q", url, dest], check=False).returncode == 0


def test_http_visibility_levels(user_factory, base_url, tmp_path):
    """private（默认）/ public（登录可读）/ anonymous（匿名可读）三档在 git 上生效。"""
    username, token, client = user_factory("httpvis")
    other, other_tok, _ = user_factory("httpother")
    repo = f"h-{_uuid.uuid4().hex[:8]}"
    client.post("/repos", json={"name": repo}, expect=201)
    base = f"/users/{username}/repos/{repo}"
    try:
        # owner 先推一个提交，保证 clone 有内容
        work = tmp_path / "work"
        _git(tmp_path, ["clone", "-q", _url(base_url, username, repo, username, token), "work"])
        (work / "f.txt").write_text("x\n")
        _commit(work, "x")
        _git(work, ["push", "-q", "origin", "HEAD:main"])

        anon_url = _url(base_url, username, repo)
        other_url = _url(base_url, username, repo, other, other_tok)

        # 默认 private：匿名与其它登录用户都不可读
        assert not _clone_ok(tmp_path, anon_url, "anon")
        assert not _clone_ok(tmp_path, other_url, "other")

        # public：登录可读，匿名仍不可读
        client.post(f"{base}/visibility", json={"visibility": "public"}, expect=200)
        assert not _clone_ok(tmp_path, anon_url, "anon2")
        assert _clone_ok(tmp_path, other_url, "other2")

        # anonymous：匿名可读
        client.post(f"{base}/visibility", json={"visibility": "anonymous"}, expect=200)
        assert _clone_ok(tmp_path, anon_url, "anon3")
    finally:
        client.delete(f"/repos/{repo}", expect=204)
