"""仓库默认分支 / issue 开关 / 引用比较（compare）—— happy path + 权限。"""

import pytest


def _uuid() -> str:
    import uuid

    return uuid.uuid4().hex[:10]


def _r(owner, repo):
    return f"/users/{owner}/repos/{repo}"


@pytest.fixture
def repo_env(user_factory):
    an, _, c = user_factory("feat")
    repo = f"feat-{_uuid()}"
    c.post("/repos", json={"name": repo}, expect=201)
    # 在默认分支 main 上制造一个提交
    c.post(
        _r(an, repo) + "/commits",
        json={
            "message": "init",
            "changes": [{"path": "a.txt", "action": "create", "content": "1"}],
        },
        expect=201,
    )
    yield an, c, repo
    try:
        c.delete(f"/repos/{repo}", expect=204)
    except Exception:
        pass


def test_repo_feature_defaults(repo_env):
    an, c, repo = repo_env
    r = c.get(_r(an, repo), expect=200).json()
    assert r["default_branch"] == "main"
    assert r["has_issues"] is True


def test_set_default_branch(repo_env):
    an, c, repo = repo_env
    c.post(
        _r(an, repo) + "/refs",
        json={"type": "branch", "name": "develop", "from": "main"},
        expect=201,
    )
    r = c.post(_r(an, repo) + "/default-branch", json={"branch": "develop"}, expect=200).json()
    assert r["default_branch"] == "develop"

    # HEAD 标记随之切换
    branches = c.get(f"/repos/{repo}/branches", expect=200).json()
    assert [b["name"] for b in branches if b["is_head"]] == ["develop"]

    # 默认分支不可删除
    c.delete(_r(an, repo) + "/refs/branch/develop", expect=409)

    # 未指定 branch 的提交落到新的默认分支
    c.post(
        _r(an, repo) + "/commits",
        json={"message": "on develop", "changes": [{"path": "b.txt", "action": "create", "content": "2"}]},
        expect=201,
    )
    commits = c.get(_r(an, repo) + "/commits?ref=develop", expect=200).json()
    assert any(x["message"] == "on develop" for x in commits)


def test_set_default_branch_bad_path(repo_env):
    an, c, repo = repo_env
    c.post(_r(an, repo) + "/default-branch", json={}, expect=400)
    c.post(_r(an, repo) + "/default-branch", json={"branch": "ghost"}, expect=400)
    c.post(_r(an, repo) + "/default-branch", json={"branch": "bad..name"}, expect=400)


def test_set_default_branch_requires_owner(repo_env, user_factory):
    an, c, repo = repo_env
    _, _, stranger = user_factory("s")
    stranger.post(_r(an, repo) + "/default-branch", json={"branch": "main"}, expect=404)


def test_issue_toggle(repo_env):
    an, c, repo = repo_env
    r = c.post(_r(an, repo) + "/issues-enabled", json={"has_issues": False}, expect=200).json()
    assert r["has_issues"] is False
    # 关闭后创建 issue 被拒
    c.post(_r(an, repo) + "/issues", json={"title": "x"}, expect=403)
    # 重新开启后可创建
    c.post(_r(an, repo) + "/issues-enabled", json={"has_issues": True}, expect=200)
    assert c.post(_r(an, repo) + "/issues", json={"title": "hello"}, expect=201).json()["title"] == "hello"


def test_issue_toggle_bad_path(repo_env):
    an, c, repo = repo_env
    c.post(_r(an, repo) + "/issues-enabled", json={}, expect=400)


def test_issue_toggle_requires_owner(repo_env, user_factory):
    an, c, repo = repo_env
    _, _, stranger = user_factory("s")
    stranger.post(_r(an, repo) + "/issues-enabled", json={"has_issues": False}, expect=404)


def test_compare_refs(repo_env):
    an, c, repo = repo_env
    c.post(
        _r(an, repo) + "/refs",
        json={"type": "branch", "name": "feature", "from": "main"},
        expect=201,
    )
    c.post(
        _r(an, repo) + "/commits",
        json={
            "branch": "feature",
            "message": "feat",
            "changes": [{"path": "b.txt", "action": "create", "content": "2"}],
        },
        expect=201,
    )
    r = c.get(_r(an, repo) + "/compare?base=main&head=feature", expect=200).json()
    assert r["base_sha"] and r["head_sha"] and r["base_sha"] != r["head_sha"]
    assert [f["path"] for f in r["files"]] == ["b.txt"]
    assert "b.txt" in r["patch"]

    # bad path
    c.get(_r(an, repo) + "/compare?base=main", expect=400)
    c.get(_r(an, repo) + "/compare?base=ghost&head=main", expect=400)
