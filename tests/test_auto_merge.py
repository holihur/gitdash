"""PR 自动合并黑盒 API 测试（需要 git/ssh，BIN 自启实例模式）。

覆盖：开启自动合并后门禁未满足不合并；reviewer approve 后自动合并；
关闭自动合并；非 open PR 拒绝开启。
"""

import os
import shutil
import subprocess
import uuid as _uuid

import pytest

pytestmark = pytest.mark.skipif(
    any(shutil.which(b) is None for b in ("git", "ssh", "ssh-keygen")),
    reason="git/ssh/ssh-keygen required",
)


def _git(workdir, key_path, args, *, check=True):
    env = dict(os.environ)
    env["GIT_SSH_COMMAND"] = (
        f"ssh -i {key_path} -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null"
    )
    r = subprocess.run(["git", *args], cwd=workdir, env=env, capture_output=True, text=True)
    if check and r.returncode != 0:
        raise AssertionError(f"git {' '.join(args)} failed rc={r.returncode}: {r.stderr}")
    return r


def _pull(owner, repo, suffix=""):
    return f"/users/{owner}/repos/{repo}/pulls{suffix}"


@pytest.fixture(scope="module")
def auto_env(base_url, ssh_port, tmp_path_factory):
    from conftest import ApiClient

    c = ApiClient(base_url)
    owner = f"u-{_uuid.uuid4().hex[:10]}"
    c.token = c.post(
        "/auth/register", json={"username": owner, "password": "test-pass-123456"}, expect=201
    ).json()["token"]
    repo = f"am-{_uuid.uuid4().hex[:8]}"
    c.post("/repos", json={"name": repo}, expect=201)

    bob = f"u-{_uuid.uuid4().hex[:10]}"
    bobc = ApiClient(base_url)
    bobc.token = bobc.post(
        "/auth/register", json={"username": bob, "password": "test-pass-123456"}, expect=201
    ).json()["token"]
    c.post(
        f"/users/{owner}/repos/{repo}/collabs",
        json={"username": bob, "permission": "write"},
        expect=200,
    )

    d = tmp_path_factory.mktemp("am")
    key = str(d / "id")
    subprocess.run(["ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-f", key], check=True)
    c.post("/keys", json={"name": "k", "public_key": open(key + ".pub").read().strip()}, expect=201)
    work = str(d / "w")
    _git(str(d), key, ["clone", "-q", f"ssh://git@127.0.0.1:{ssh_port}/{repo}.git", "w"])

    def commit(msg):
        _git(work, key, ["add", "-A"])
        _git(work, key, ["-c", "user.name=u", "-c", "user.email=u@example.com", "commit", "-q", "-m", msg])

    _git(work, key, ["checkout", "-q", "-b", "main"])
    open(os.path.join(work, "base.txt"), "w").write("base\n")
    commit("base")
    _git(work, key, ["push", "-q", "-u", "origin", "main"])
    _git(work, key, ["checkout", "-q", "-b", "feat"])
    open(os.path.join(work, "feat.txt"), "w").write("feature\n")
    commit("feat")
    _git(work, key, ["push", "-q", "-u", "origin", "feat"])
    _git(work, key, ["checkout", "-q", "main"])

    # 门禁：至少 1 个 approve
    c.put(
        f"/users/{owner}/repos/{repo}/branch-protections/main",
        json={
            "min_approvals": 1,
            "require_ci": False,
            "require_codeowners": False,
            "block_deletion": False,
            "block_force_push": False,
        },
        expect=200,
    )

    pr = c.post(
        _pull(owner, repo),
        json={"title": "auto", "source_branch": "feat", "target_branch": "main"},
        expect=201,
    ).json()

    env = {"client": c, "bob": bobc, "owner": owner, "repo": repo, "number": pr["number"], "bob_name": bob}
    yield env
    try:
        c.delete(f"/repos/{repo}", expect=204)
    except Exception:
        pass


def test_auto_merge_waits_for_gate(auto_env):
    c, bobc = auto_env["client"], auto_env["bob"]
    owner, repo, number = auto_env["owner"], auto_env["repo"], auto_env["number"]
    base = _pull(owner, repo, f"/{number}")

    # 开启自动合并：门禁未满足（0 approve），PR 仍 open
    pr = c.post(f"{base}/auto-merge", json={"enabled": True, "method": "squash"}, expect=200).json()
    assert pr["auto_merge"] is True and pr["state"] == "open", pr

    # 非法 method → 400
    c.post(f"{base}/auto-merge", json={"enabled": True, "method": "bogus"}, expect=400)

    # reviewer approve → 触发自动合并
    bobc.post(f"{base}/reviews", json={"state": "approve", "body": "lgtm"}, expect=201)
    pr = c.get(base, expect=200).json()
    assert pr["state"] == "merged", pr
    assert pr["merged_by"] == owner, pr

    # 已合并后不可再改自动合并
    c.post(f"{base}/auto-merge", json={"enabled": False}, expect=400)


def test_auto_merge_disable(auto_env):
    """关闭自动合并：open PR 上可开关。"""
    c = auto_env["client"]
    owner, repo = auto_env["owner"], auto_env["repo"]
    # 新建一个不满足门禁的 PR
    pr = c.post(
        _pull(owner, repo),
        json={"title": "toggle", "source_branch": "feat", "target_branch": "main"},
        expect=201,
    ).json()
    base = _pull(owner, repo, f"/{pr['number']}")
    assert c.post(f"{base}/auto-merge", json={"enabled": True}, expect=200).json()["auto_merge"] is True
    assert c.post(f"{base}/auto-merge", json={"enabled": False}, expect=200).json()["auto_merge"] is False
