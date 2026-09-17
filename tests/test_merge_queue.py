"""合并队列黑盒 API 测试（需要 git/ssh，BIN 自启实例模式）。

覆盖：启用 merge_queue 的分支上合并不立即执行而是入队（202），
门禁满足后由队列处理器合并；可从队列移除。
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
def mq_env(base_url, ssh_port, tmp_path_factory):
    from conftest import ApiClient

    c = ApiClient(base_url)
    owner = f"u-{_uuid.uuid4().hex[:10]}"
    c.token = c.post(
        "/auth/register", json={"username": owner, "password": "test-pass-123456"}, expect=201
    ).json()["token"]
    repo = f"mq-{_uuid.uuid4().hex[:8]}"
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

    d = tmp_path_factory.mktemp("mq")
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

    c.put(
        f"/users/{owner}/repos/{repo}/branch-protections/main",
        json={
            "min_approvals": 1,
            "require_ci": False,
            "require_codeowners": False,
            "merge_queue": True,
            "block_deletion": False,
            "block_force_push": False,
        },
        expect=200,
    )
    # 确认规则回读含 merge_queue
    prots = c.get(f"/users/{owner}/repos/{repo}/branch-protections", expect=200).json()
    assert any(p["branch"] == "main" and p["merge_queue"] for p in prots), prots

    env = {"client": c, "bob": bobc, "owner": owner, "repo": repo, "bob_name": bob}
    yield env
    try:
        c.delete(f"/repos/{repo}", expect=204)
    except Exception:
        pass


def test_merge_queue_enqueue_then_merge(mq_env):
    c, bobc = mq_env["client"], mq_env["bob"]
    owner, repo = mq_env["owner"], mq_env["repo"]

    pr = c.post(
        _pull(owner, repo),
        json={"title": "queued", "source_branch": "feat", "target_branch": "main"},
        expect=201,
    ).json()
    base = _pull(owner, repo, f"/{pr['number']}")

    # 合并 → 入队（202），PR 仍 open 且标记 merge_queued
    queued = c.post(f"{base}/merge", json={"method": "squash"}, expect=202).json()
    assert queued["state"] == "open" and queued.get("merge_queued") is True, queued
    assert c.get(base, expect=200).json().get("merge_queued") is True

    # reviewer approve → 队列处理器合并
    bobc.post(f"{base}/reviews", json={"state": "approve"}, expect=201)
    pr = c.get(base, expect=200).json()
    assert pr["state"] == "merged", pr


def test_merge_queue_dequeue(mq_env):
    c = mq_env["client"]
    owner, repo = mq_env["owner"], mq_env["repo"]

    pr = c.post(
        _pull(owner, repo),
        json={"title": "leave", "source_branch": "feat", "target_branch": "main"},
        expect=201,
    ).json()
    base = _pull(owner, repo, f"/{pr['number']}")

    c.post(f"{base}/merge", json={"method": "squash"}, expect=202)
    assert c.get(base, expect=200).json().get("merge_queued") is True

    assert c.delete(f"{base}/merge-queue", expect=200).json()["dequeued"] is True
    assert not c.get(base, expect=200).json().get("merge_queued")
    # 再次移除 → 404
    c.delete(f"{base}/merge-queue", expect=404)
