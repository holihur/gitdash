"""patch-by-email 黑盒测试：POST mbox → 自动建分支 + 开 PR。

需要 git/ssh/ssh-keygen；用 `git format-patch` 造 mbox，再走 `POST .../patches`。
"""

from __future__ import annotations

import os
import shutil
import subprocess
import uuid

import pytest

pytestmark = pytest.mark.skipif(
    any(shutil.which(b) is None for b in ("git", "ssh", "ssh-keygen")),
    reason="git/ssh/ssh-keygen required (patch submission needs a real repo)",
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


def _p(owner, repo, suffix=""):
    return f"/users/{owner}/repos/{repo}{suffix}"


@pytest.fixture(scope="module")
def patch_env(base_url, ssh_port, tmp_path_factory):
    """owner + 模板仓库 + 本地 clone（main 上可生成 patch）。"""
    from conftest import ApiClient

    client = ApiClient(base_url)
    owner = f"u-{uuid.uuid4().hex[:10]}"
    client.token = client.post(
        "/auth/register", json={"username": owner, "password": "test-pass-123456"}, expect=201
    ).json()["token"]
    repo = f"p-{uuid.uuid4().hex[:8]}"
    client.post("/repos", json={"name": repo, "template": "readme"}, expect=201)

    d = tmp_path_factory.mktemp("patch")
    key = str(d / "id")
    subprocess.run(["ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-f", key], check=True)
    client.post(
        "/keys", json={"name": "k", "public_key": open(key + ".pub").read().strip()}, expect=201
    )
    work = str(d / "w")
    _git(str(d), key, ["clone", "-q", f"ssh://git@127.0.0.1:{ssh_port}/{repo}.git", "w"])
    _git(work, key, ["config", "user.name", "tester"])
    _git(work, key, ["config", "user.email", "tester@example.com"])
    _git(work, key, ["checkout", "-q", "main"])

    yield {"client": client, "owner": owner, "repo": repo, "key": key, "work": work}

    try:
        client.delete(f"/repos/{repo}", expect=204)
    except Exception:
        pass


def _commit_file(env, name, content):
    path = os.path.join(env["work"], name)
    parent = os.path.dirname(path)
    if parent:
        os.makedirs(parent, exist_ok=True)
    with open(path, "w") as fh:
        fh.write(content)
    _git(env["work"], env["key"], ["add", name])
    _git(env["work"], env["key"], ["commit", "-q", "-m", f"add {name}"])


def _reset_main(env):
    """把本地 main 重置到 origin/main，保证每个用例的 patch 都基于干净基线。"""
    _git(env["work"], env["key"], ["checkout", "-q", "-B", "main", "origin/main"])


def _format_patch(env, count=1):
    return _git(env["work"], env["key"], ["format-patch", f"-{count}", "--stdout"]).stdout


def test_single_patch_opens_pr(patch_env):
    env = patch_env
    client, owner, repo = env["client"], env["owner"], env["repo"]
    _reset_main(env)
    _commit_file(env, "feature.txt", "hello patch\n")
    mbox = _format_patch(env, 1)

    pr = client.post(_p(owner, repo, "/patches"), data=mbox.encode(), expect=201).json()
    assert pr["state"] == "open"
    assert pr["target_branch"] == "main"
    assert pr["source_branch"].startswith("patches/")
    assert pr["title"] == "add feature.txt"
    assert pr["body"] and "add feature.txt" in pr["body"]

    diff = client.get(_p(owner, repo, f"/pulls/{pr['number']}/diff"), expect=200).json()
    paths = [f["path"] for f in diff["files"]]
    assert "feature.txt" in paths
    assert "hello patch" in diff["patch"]


def test_multi_patch_series(patch_env):
    env = patch_env
    client, owner, repo = env["client"], env["owner"], env["repo"]
    _reset_main(env)
    _commit_file(env, "a.txt", "a\n")
    _commit_file(env, "b.txt", "b\n")
    mbox = _format_patch(env, 2)

    pr = client.post(_p(owner, repo, "/patches"), data=mbox.encode(), expect=201).json()
    # 默认标题取第一封补丁的 Subject
    assert pr["title"] == "add a.txt"
    assert "add a.txt" in pr["body"] and "add b.txt" in pr["body"]
    diff = client.get(_p(owner, repo, f"/pulls/{pr['number']}/diff"), expect=200).json()
    paths = {f["path"] for f in diff["files"]}
    assert {"a.txt", "b.txt"} <= paths


def test_custom_title_and_target(patch_env):
    env = patch_env
    client, owner, repo = env["client"], env["owner"], env["repo"]
    _reset_main(env)
    _commit_file(env, "c.txt", "c\n")
    mbox = _format_patch(env, 1)
    pr = client.post(
        _p(owner, repo, "/patches") + "?title=My%20Series&target=main",
        data=mbox.encode(),
        expect=201,
    ).json()
    assert pr["title"] == "My Series" and pr["target_branch"] == "main"


def test_invalid_patch_rejected(patch_env):
    env = patch_env
    client, owner, repo = env["client"], env["owner"], env["repo"]
    r = client.post(_p(owner, repo, "/patches"), data=b"this is not a patch\n", expect=400).json()
    assert r["code"] == "patch_failed"


def test_empty_patch_rejected(patch_env):
    env = patch_env
    client, owner, repo = env["client"], env["owner"], env["repo"]
    r = client.post(_p(owner, repo, "/patches"), data=b"   \n", expect=400).json()
    assert r["code"] == "empty_patch"


def test_bad_target_branch_rejected(patch_env):
    env = patch_env
    client, owner, repo = env["client"], env["owner"], env["repo"]
    _reset_main(env)
    _commit_file(env, "d.txt", "d\n")
    mbox = _format_patch(env, 1)
    r = client.post(
        _p(owner, repo, "/patches") + "?target=does-not-exist", data=mbox.encode(), expect=400
    ).json()
    assert r["code"] == "patch_failed"


def test_patch_requires_write_permission(patch_env, user_factory):
    env = patch_env
    _, _, bob = user_factory("bob")
    r = bob.post(
        _p(env["owner"], env["repo"], "/patches"), data=b"From 0000\nSubject: [PATCH] x\n",
    )
    assert r.status_code == 404


def test_patch_requires_auth(patch_env, anon):
    env = patch_env
    anon.post(_p(env["owner"], env["repo"], "/patches"), data=b"x", expect=401)
