"""CODEOWNERS 合并门禁黑盒 API 测试（需要 git/ssh，BIN 自启实例模式）。

覆盖：解析 CODEOWNERS 得到变更文件的 owner、require_codeowners 时未批准不可合并、
code owner 批准后可合并、gate 暴露 codeowners 状态。
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
def co_env(base_url, ssh_port, tmp_path_factory):
    from conftest import ApiClient

    c = ApiClient(base_url)
    owner = f"u-{_uuid.uuid4().hex[:10]}"
    c.token = c.post(
        "/auth/register", json={"username": owner, "password": "test-pass-123456"}, expect=201
    ).json()["token"]
    repo = f"co-{_uuid.uuid4().hex[:8]}"
    c.post("/repos", json={"name": repo}, expect=201)

    # 协作者 bob（写权限），同时是 CODEOWNERS 声明的所有者
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

    d = tmp_path_factory.mktemp("co")
    key = str(d / "id")
    subprocess.run(["ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-f", key], check=True)
    c.post("/keys", json={"name": "k", "public_key": open(key + ".pub").read().strip()}, expect=201)
    work = str(d / "w")
    _git(str(d), key, ["clone", "-q", f"ssh://git@127.0.0.1:{ssh_port}/{repo}.git", "w"])

    def commit(msg):
        _git(work, key, ["add", "-A"])
        _git(work, key, ["-c", "user.name=u", "-c", "user.email=u@example.com", "commit", "-q", "-m", msg])

    _git(work, key, ["checkout", "-q", "-b", "main"])
    open(os.path.join(work, "CODEOWNERS"), "w").write(f"*.txt @{bob}\n")
    commit("codeowners")
    _git(work, key, ["push", "-q", "-u", "origin", "main"])

    _git(work, key, ["checkout", "-q", "-b", "feat"])
    open(os.path.join(work, "feat.txt"), "w").write("feature\n")
    commit("feat")
    _git(work, key, ["push", "-q", "-u", "origin", "feat"])
    _git(work, key, ["checkout", "-q", "main"])

    pr = c.post(
        _pull(owner, repo),
        json={"title": "co", "source_branch": "feat", "target_branch": "main"},
        expect=201,
    ).json()

    # 开启 CODEOWNERS 门禁
    c.put(
        f"/users/{owner}/repos/{repo}/branch-protections/main",
        json={
            "min_approvals": 0,
            "require_ci": False,
            "require_codeowners": True,
            "block_deletion": False,
            "block_force_push": False,
        },
        expect=200,
    )

    env = {"client": c, "bob": bobc, "owner": owner, "repo": repo, "number": pr["number"], "bob_name": bob}
    yield env
    try:
        c.delete(f"/repos/{repo}", expect=204)
    except Exception:
        pass


def test_codeowners_merge_gate(co_env):
    c, bobc = co_env["client"], co_env["bob"]
    owner, repo, number = co_env["owner"], co_env["repo"], co_env["number"]
    bob = co_env["bob_name"]
    base = _pull(owner, repo, f"/{number}")

    st = c.get(f"{base}/codeowners", expect=200).json()
    assert st["owners"] == [bob], st
    assert st["missing"] == [bob] and st["satisfied"] is False, st
    assert any(f["path"] == "feat.txt" for f in st["files"]), st

    # 未批准 → 合并被门禁拦截
    body = c.post(f"{base}/merge", expect=409).json()
    assert body["code"] == "codeowners_required", body

    # gate 暴露 CODEOWNERS 状态
    gate = c.get(f"{base}/reviews", expect=200).json()["gate"]
    assert gate["codeowners_required"] is True
    assert gate["codeowners_missing"] == [bob]
    assert gate["mergeable"] is False

    # code owner 批准 → 可合并
    bobc.post(f"{base}/reviews", json={"state": "approve", "body": "lgtm"}, expect=201)
    st = c.get(f"{base}/codeowners", expect=200).json()
    assert st["satisfied"] is True and st["missing"] == [], st

    merged = c.post(f"{base}/merge", expect=200).json()
    assert merged["state"] == "merged", merged
