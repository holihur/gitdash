"""PR review + 行内评论 黑盒 API 测试（需要 git/ssh，BIN 自启实例模式；外部实例模式跳过）。

覆盖：approve / request_changes / comment 三种状态、重复提交保留历史、
summary 按 reviewer 最新状态汇总、行内评论（file_path/line/line_side）创建与列表、
非法 state 400、无权限（非协作者）404。
"""

import os
import shutil
import subprocess
import uuid as _uuid

import pytest

pytestmark = pytest.mark.skipif(
    any(shutil.which(b) is None for b in ("git", "ssh", "ssh-keygen")),
    reason="git/ssh/ssh-keygen required (PR creation needs a real branch)",
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
def review_env(base_url, ssh_port, tmp_path_factory):
    """owner + 协作者 + 带 main/feat 分支的仓库，PR #1 已建好。"""
    from conftest import ApiClient

    c = ApiClient(base_url)
    owner = f"u-{_uuid.uuid4().hex[:10]}"
    c.token = c.post(
        "/auth/register", json={"username": owner, "password": "test-pass-123456"}, expect=201
    ).json()["token"]
    repo = f"rv-{_uuid.uuid4().hex[:8]}"
    c.post("/repos", json={"name": repo}, expect=201)

    d = tmp_path_factory.mktemp("git")
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
    open(os.path.join(work, "feat.txt"), "w").write("feat\n")
    commit("feat")
    _git(work, key, ["push", "-q", "-u", "origin", "feat"])

    pr = c.post(
        _pull(owner, repo),
        json={"title": "reviewable", "source_branch": "feat", "target_branch": "main"},
        expect=201,
    ).json()
    number = pr["number"]

    # 协作者 bob（可写，可提交 review）
    bob = f"u-{_uuid.uuid4().hex[:10]}"
    bobc = ApiClient(base_url)
    bobc.token = bobc.post(
        "/auth/register", json={"username": bob, "password": "test-pass-123456"}, expect=201
    ).json()["token"]
    c.post(_pull(owner, repo).rsplit("/pulls", 1)[0] + "/collabs", json={"username": bob, "permission": "write"}, expect=200)

    # 无关用户 carol（非协作者）
    carol = f"u-{_uuid.uuid4().hex[:10]}"
    carolc = ApiClient(base_url)
    carolc.token = carolc.post(
        "/auth/register", json={"username": carol, "password": "test-pass-123456"}, expect=201
    ).json()["token"]

    env = {
        "client": c, "bob": bobc, "carol": carolc,
        "owner": owner, "repo": repo, "number": number, "bob_name": bob,
    }
    yield env
    try:
        c.delete(f"/repos/{repo}", expect=204)
    except Exception:
        pass


def test_reviews_three_states_and_summary(review_env):
    c, bobc = review_env["client"], review_env["bob"]
    owner, repo, number = review_env["owner"], review_env["repo"], review_env["number"]
    base = _pull(owner, repo, f"/{number}/reviews")

    r1 = c.post(base, json={"state": "approve", "body": "looks good"}, expect=201).json()
    assert r1["state"] == "approve" and r1["reviewer"] == owner and r1["number"] == number
    r2 = bobc.post(base, json={"state": "request_changes", "body": "please fix"}, expect=201).json()
    assert r2["state"] == "request_changes" and r2["reviewer"] == review_env["bob_name"]

    out = c.get(base, expect=200).json()
    assert [x["state"] for x in out["reviews"]] == ["approve", "request_changes"]
    assert out["summary"] == {"approvals": 1, "request_changes": 1}

    # 同一 reviewer 重复提交插入新行，汇总取最新一条
    bobc.post(base, json={"state": "approve", "body": "fixed now"}, expect=201)
    out = c.get(base, expect=200).json()
    assert len(out["reviews"]) == 3
    assert out["summary"] == {"approvals": 2, "request_changes": 0}

    # comment 不计入 approvals / request_changes；owner 最新一条变为 comment，汇总随最新变化
    c.post(base, json={"state": "comment", "body": "question?"}, expect=201)
    out = c.get(base, expect=200).json()
    assert len(out["reviews"]) == 4
    assert out["summary"] == {"approvals": 1, "request_changes": 0}


def test_reviews_bad_state_and_permissions(review_env):
    c, carolc = review_env["client"], review_env["carol"]
    owner, repo, number = review_env["owner"], review_env["repo"], review_env["number"]
    base = _pull(owner, repo, f"/{number}/reviews")

    c.post(base, json={"state": "lgtm"}, expect=400)
    c.post(base, json={}, expect=400)

    # 非协作者不可读写（一律 404 隐藏仓库存在性）
    carolc.post(base, json={"state": "approve"}, expect=404)
    carolc.get(base, expect=404)

    c.get(_pull(owner, repo, "/999/reviews"), expect=404)


def test_inline_comments(review_env):
    c = review_env["client"]
    owner, repo, number = review_env["owner"], review_env["repo"], review_env["number"]
    base = _pull(owner, repo, f"/{number}/comments")

    ic = c.post(base, json={"body": "rename this", "file_path": "feat.txt", "line": 1, "line_side": "new"}, expect=201).json()
    assert ic["file_path"] == "feat.txt"
    assert ic["line"] == 1
    assert ic["line_side"] == "new"

    old = c.post(base, json={"body": "old side", "file_path": "feat.txt", "line": 2, "line_side": "old"}, expect=201).json()
    assert old["line_side"] == "old" and old["line"] == 2

    # 普通评论不受影响
    normal = c.post(base, json={"body": "plain"}, expect=201).json()
    assert normal["file_path"] is None and normal["line"] is None and normal["line_side"] == ""

    comments = c.get(base, expect=200).json()
    by_id = {x["id"]: x for x in comments}
    assert by_id[ic["id"]]["file_path"] == "feat.txt"
    assert by_id[old["id"]]["line"] == 2
    assert by_id[normal["id"]]["file_path"] is None

    # 行内评论校验：line >= 1，line_side 必须为 old/new
    c.post(base, json={"body": "x", "file_path": "feat.txt", "line": 0, "line_side": "new"}, expect=400)
    c.post(base, json={"body": "x", "file_path": "feat.txt", "line": 1, "line_side": "both"}, expect=400)
    c.post(base, json={"body": "x", "file_path": "feat.txt", "line": 1}, expect=400)

    # 行内评论只对 PR 生效，issue 路由不受影响
    issue = c.post(f"/users/{owner}/repos/{repo}/issues", json={"title": "plain issue"}, expect=201).json()
    c.post(f"/users/{owner}/repos/{repo}/issues/{issue['number']}/comments", json={"body": "ok"}, expect=201)
