"""Issue/PR 评论 API 黑盒测试 + 权限隔离 + 简写路由。"""

import os
import shutil
import subprocess
import uuid as _uuid

import pytest

pytestmark = pytest.mark.skipif(
    any(shutil.which(b) is None for b in ("git", "ssh", "ssh-keygen")),
    reason="git/ssh/ssh-keygen required (PR creation needs a real branch)",
)

LONG_BODY = "x" * 10001


def _git(workdir, key_path, args, *, check=True):
    env = dict(os.environ)
    env["GIT_SSH_COMMAND"] = (
        f"ssh -i {key_path} -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null"
    )
    r = subprocess.run(["git", *args], cwd=workdir, env=env, capture_output=True, text=True)
    if check and r.returncode != 0:
        raise AssertionError(f"git {' '.join(args)} failed rc={r.returncode}: {r.stderr}")
    return r


def _issue_path(owner, repo, suffix=""):
    return f"/users/{owner}/repos/{repo}/issues{suffix}"


def _pull_path(owner, repo, suffix=""):
    return f"/users/{owner}/repos/{repo}/pulls{suffix}"


@pytest.fixture(scope="module")
def comment_env(base_url, ssh_port, tmp_path_factory):
    """owner + 带 main/feat 分支的私有仓库，可建 issue 和 PR。"""
    from conftest import ApiClient

    client = ApiClient(base_url)
    owner = f"u-{_uuid.uuid4().hex[:10]}"
    client.token = client.post(
        "/auth/register", json={"username": owner, "password": "test-pass-123456"}, expect=201
    ).json()["token"]
    repo = f"c-{_uuid.uuid4().hex[:8]}"
    client.post("/repos", json={"name": repo}, expect=201)

    d = tmp_path_factory.mktemp("git")
    key = str(d / "id")
    subprocess.run(["ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-f", key], check=True)
    client.post(
        "/keys", json={"name": "k", "public_key": open(key + ".pub").read().strip()}, expect=201
    )

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
    open(os.path.join(work, "feat.txt"), "w").write("feat\n")
    commit("feat")
    _git(work, key, ["push", "-q", "-u", "origin", "feat"])

    env = {"client": client, "owner": owner, "repo": repo}
    yield env
    try:
        client.delete(f"/repos/{repo}", expect=204)
    except Exception:
        pass


# ---- 1. issue 评论 CRUD ----

def test_issue_comments_crud(comment_env):
    c = comment_env["client"]
    owner, repo = comment_env["owner"], comment_env["repo"]
    base = _issue_path(owner, repo)

    issue = c.post(f"{base}", json={"title": "comment target"}, expect=201).json()
    n = issue["number"]

    c1 = c.post(f"{base}/{n}/comments", json={"body": "first"}, expect=201).json()
    c2 = c.post(f"{base}/{n}/comments", json={"body": "second"}, expect=201).json()
    assert c1["body"] == "first" and c2["body"] == "second"
    assert c1["author"] == owner and c2["author"] == owner
    assert c1["id"] != c2["id"]
    assert c1["number"] == n and "created_at" in c1 and "updated_at" in c1

    comments = c.get(f"{base}/{n}/comments", expect=200).json()
    assert [x["id"] for x in comments] == sorted([c1["id"], c2["id"]])
    assert [x["body"] for x in comments] == ["first", "second"]

    # bad body
    c.post(f"{base}/{n}/comments", json={"body": ""}, expect=400)
    c.post(f"{base}/{n}/comments", json={"body": LONG_BODY}, expect=400)

    # 删除自己的评论
    c.delete(f"/users/{owner}/repos/{repo}/comments/{c1['id']}", expect=204)
    c.delete(f"/users/{owner}/repos/{repo}/comments/{c1['id']}", expect=404)
    comments = c.get(f"{base}/{n}/comments", expect=200).json()
    assert [x["id"] for x in comments] == [c2["id"]]

    # 不存在的 issue number
    c.get(f"{base}/999/comments", expect=200).json() == []
    c.post(f"{base}/999/comments", json={"body": "x"}, expect=404)


# ---- 2. PR 评论 ----

def test_pull_comments(comment_env):
    c = comment_env["client"]
    owner, repo = comment_env["owner"], comment_env["repo"]

    pr = c.post(
        _pull_path(owner, repo),
        json={"title": "pr with comments", "source_branch": "feat", "target_branch": "main"},
        expect=201,
    ).json()
    n = pr["number"]

    c1 = c.post(f"{_pull_path(owner, repo, f'/{n}/comments')}", json={"body": "lgtm"}, expect=201).json()
    assert c1["body"] == "lgtm" and c1["author"] == owner

    comments = c.get(f"{_pull_path(owner, repo, f'/{n}/comments')}", expect=200).json()
    assert [x["id"] for x in comments] == [c1["id"]]

    # issue 与 PR 的 number 独立：PR 评论不泄漏到 issue 编号空间
    sep = c.post(f"{_issue_path(owner, repo)}", json={"title": "separate"}, expect=201).json()
    assert c.get(_issue_path(owner, repo, "/%d/comments" % sep["number"]), expect=200).json() == []

    c.get(_pull_path(owner, repo, "/999/comments"), expect=200).json() == []
    c.post(_pull_path(owner, repo, "/999/comments"), json={"body": "x"}, expect=404)


# ---- 3. 权限 ----

def test_comments_permission(comment_env, user_factory):
    c = comment_env["client"]
    owner, repo = comment_env["owner"], comment_env["repo"]
    stranger_name, _, stranger = user_factory("stranger")
    outsider_name, _, outsider = user_factory("outsider")

    issue = c.post(f"{_issue_path(owner, repo)}", json={"title": "perm"}, expect=201).json()
    n = issue["number"]

    # 私有仓库：stranger 非协作者 → 404
    stranger.get(f"{_issue_path(owner, repo, f'/{n}/comments')}", expect=404)
    stranger.post(f"{_issue_path(owner, repo, f'/{n}/comments')}", json={"body": "hi"}, expect=404)
    outsider_name  # noqa: B018 — 仅需要 token 客户端

    # owner 授予 stranger 写权限后：stranger 可评论
    c.post(
        f"/users/{owner}/repos/{repo}/collabs",
        json={"username": stranger_name, "permission": "write"},
        expect=200,
    )
    sc = stranger.post(
        f"{_issue_path(owner, repo, f'/{n}/comments')}", json={"body": "mine"}, expect=201
    ).json()
    assert sc["author"] == stranger_name

    # stranger 删除自己的评论 → 204
    stranger.delete(f"/users/{owner}/repos/{repo}/comments/{sc['id']}", expect=204)
    stranger.delete(f"/users/{owner}/repos/{repo}/comments/{sc['id']}", expect=404)

    # 有写权限的非作者可删他人评论（spec：作者本人或写权限）→ 204
    own = c.post(f"{_issue_path(owner, repo, f'/{n}/comments')}", json={"body": "owner note"}, expect=201).json()
    stranger.delete(f"/users/{owner}/repos/{repo}/comments/{own['id']}", expect=204)

    # owner（写权限）可删除 stranger 的评论 → 204
    sc2 = stranger.post(
        f"{_issue_path(owner, repo, f'/{n}/comments')}", json={"body": "again"}, expect=201
    ).json()
    c.delete(f"/users/{owner}/repos/{repo}/comments/{sc2['id']}", expect=204)

    # 转公开后：普通用户（有读权限、无写权限）发评论 → 404（addComment 需 write）
    c.post(
        f"/users/{owner}/repos/{repo}/visibility", json={"private": False}, expect=200
    )
    r = outsider.post(
        f"{_issue_path(owner, repo, f'/{n}/comments')}", json={"body": "hi"}, expect=None
    )
    if r.status_code != 404:
        raise AssertionError(
            f"public repo reader without write should get 404 on add, got {r.status_code}: {r.text}"
        )
    # 同样的读者删除他人评论 → 403（deleteComment 走读权限 + 作者/写权限判定）
    target = c.post(
        f"{_issue_path(owner, repo, f'/{n}/comments')}", json={"body": "keep me"}, expect=201
    ).json()
    r = outsider.delete(f"/users/{owner}/repos/{repo}/comments/{target['id']}", expect=None)
    if r.status_code != 403:
        raise AssertionError(
            f"public repo reader deleting another's comment should get 403, got {r.status_code}: {r.text}"
        )


# ---- 4. 简写路由 ----

def test_comments_short_route(comment_env):
    c = comment_env["client"]
    owner, repo = comment_env["owner"], comment_env["repo"]

    issue = c.post(f"{_issue_path(owner, repo)}", json={"title": "short"}, expect=201).json()
    n = issue["number"]

    c1 = c.post(f"/repos/{repo}/issues/{n}/comments", json={"body": "via short"}, expect=201).json()
    comments = c.get(f"/repos/{repo}/issues/{n}/comments", expect=200).json()
    assert [x["id"] for x in comments] == [c1["id"]]
    # 简写路由以“当前用户为 owner”解析；删除经 /users 路由
    c.delete(f"/users/{owner}/repos/{repo}/comments/{c1['id']}", expect=204)
    assert c.get(f"/repos/{repo}/issues/{n}/comments", expect=200).json() == []
