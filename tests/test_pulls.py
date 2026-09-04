"""Pull Request 黑盒 API 测试（需要 git/ssh，BIN 自启实例模式；外部实例模式跳过）。

覆盖：创建/列表/diff/合并/关闭重开 + bad path（分叉不可合并、分支校验、权限隔离）。
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


def _p(owner, repo, suffix=""):
    return f"/users/{owner}/repos/{repo}/pulls{suffix}"


@pytest.fixture(scope="module")
def pr_env(base_url, ssh_port, tmp_path_factory):
    from conftest import ApiClient

    client = ApiClient(base_url)
    name = f"u-{_uuid.uuid4().hex[:10]}"
    client.token = client.post(
        "/auth/register", json={"username": name, "password": "test-pass-123456"}, expect=201
    ).json()["token"]
    repo = f"pr-{_uuid.uuid4().hex[:8]}"
    client.post("/repos", json={"name": repo}, expect=201)

    d = tmp_path_factory.mktemp("git")
    key = str(d / "id")
    subprocess.run(["ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-f", key], check=True)
    client.post("/keys", json={"name": "k", "public_key": open(key + ".pub").read().strip()}, expect=201)

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
    open(os.path.join(work, "feat.txt"), "w").write("hello feature\n")
    commit("feat")
    _git(work, key, ["push", "-q", "-u", "origin", "feat"])
    _git(work, key, ["checkout", "-q", "main"])

    env = {"client": client, "owner": name, "repo": repo, "work": work, "key": key}
    yield env
    try:
        client.delete(f"/repos/{repo}", expect=204)
    except Exception:
        pass


def test_pr_lifecycle_merge(pr_env):
    c = pr_env["client"]
    owner, repo = pr_env["owner"], pr_env["repo"]

    pr = c.post(
        _p(owner, repo),
        json={"title": "add feat", "body": "desc", "source_branch": "feat", "target_branch": "main"},
        expect=201,
    ).json()
    assert pr["number"] == 1 and pr["state"] == "open"
    assert pr["source_branch"] == "feat" and pr["head_sha"]

    pulls = c.get(_p(owner, repo), expect=200).json()
    assert len(pulls) == 1 and pulls[0]["number"] == 1

    diff = c.get(_p(owner, repo, "/1/diff"), expect=200).json()
    assert diff["files"] and diff["files"][0]["path"] == "feat.txt"
    assert "+hello feature" in diff["patch"]

    # 关闭/重开
    c.post(_p(owner, repo, "/1/state"), json={"state": "closed"}, expect=200)
    c.post(_p(owner, repo, "/1/state"), json={"state": "open"}, expect=200)

    merged = c.post(_p(owner, repo, "/1/merge"), expect=200).json()
    assert merged["state"] == "merged" and merged["merged_by"] == owner
    c.post(_p(owner, repo, "/1/merge"), expect=409)
    c.post(_p(owner, repo, "/1/state"), json={"state": "closed"}, expect=400)

    # 目标分支已含改动
    r = _git(pr_env["work"], pr_env["key"], ["show", "origin/main:feat.txt"], check=False)
    _git(pr_env["work"], pr_env["key"], ["fetch", "-q", "origin"])
    r = _git(pr_env["work"], pr_env["key"], ["show", "origin/main:feat.txt"])
    assert "hello feature" in r.stdout


def test_pr_bad_paths_and_isolation(pr_env, user_factory, anon):
    c = pr_env["client"]
    owner, repo = pr_env["owner"], pr_env["repo"]
    work, key = pr_env["work"], pr_env["key"]

    c.post(_p(owner, repo), json={"title": "t", "source_branch": "main", "target_branch": "main"}, expect=400)
    c.post(_p(owner, repo), json={"title": "  ", "source_branch": "feat", "target_branch": "main"}, expect=400)
    c.post(_p(owner, repo), json={"title": "t", "source_branch": "ghost", "target_branch": "main"}, expect=400)
    c.get(_p(owner, repo, "/999"), expect=404)

    # 分叉分支无法 fast-forward 合并（先同步 main 到远端 tip）
    _git(work, key, ["fetch", "-q", "origin"])
    _git(work, key, ["checkout", "-q", "-B", "main", "origin/main"])
    _git(work, key, ["checkout", "-q", "-b", "diverged"])
    open(os.path.join(work, "d.txt"), "w").write("d\n")
    _git(work, key, ["add", "-A"])
    _git(work, key, ["-c", "user.name=u", "-c", "user.email=u@e", "commit", "-q", "-m", "d"])
    _git(work, key, ["push", "-q", "-u", "origin", "diverged"])
    _git(work, key, ["checkout", "-q", "main"])
    open(os.path.join(work, "m2.txt"), "w").write("m2\n")
    _git(work, key, ["add", "-A"])
    _git(work, key, ["-c", "user.name=u", "-c", "user.email=u@e", "commit", "-q", "-m", "m2"])
    _git(work, key, ["push", "-q", "origin", "HEAD"])
    pr = c.post(
        _p(owner, repo),
        json={"title": "conflict", "source_branch": "diverged", "target_branch": "main"},
        expect=201,
    ).json()
    c.post(_p(owner, repo, f"/{pr['number']}/merge"), expect=409)

    # 其他用户（非协作者）不可见
    _, _, bob = user_factory("bob")
    bob.get(_p(owner, repo), expect=404)
    bob.post(_p(owner, repo), json={"title": "x"}, expect=404)
    anon.get(_p(owner, repo), expect=401)
    anon.post(_p(owner, repo), json={"title": "x"}, expect=401)
