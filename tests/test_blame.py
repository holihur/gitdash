"""blame（文件逐行追溯）—— happy path + bad path + 权限。"""

import pytest


def _uuid() -> str:
    import uuid

    return uuid.uuid4().hex[:10]


def _p(owner, repo):
    return f"/users/{owner}/repos/{repo}"


@pytest.fixture
def repo_env(user_factory):
    an, _, c = user_factory("bl")
    repo = f"blame-{_uuid()}"
    c.post("/repos", json={"name": repo}, expect=201)
    c.post(
        _p(an, repo) + "/commits",
        json={
            "message": "add source",
            "changes": [
                {"path": "src/app.ts", "action": "create", "content": "line one\nline two\n"},
                {"path": "README.md", "action": "create", "content": "# hello\n"},
            ],
        },
        expect=201,
    )
    yield an, c, repo
    try:
        c.delete(f"/repos/{repo}", expect=204)
    except Exception:
        pass


def test_blame_lines_and_commits(repo_env):
    an, c, repo = repo_env
    data = c.get(_p(an, repo) + "/blame?ref=main&path=src/app.ts", expect=200).json()

    assert data["path"] == "src/app.ts"
    lines = data["lines"]
    assert [ln["line"] for ln in lines] == [1, 2]
    assert [ln["content"] for ln in lines] == ["line one", "line two"]

    # 所有行的 commit 字段都能在 commits 映射中找到，且元数据完整
    commits = data["commits"]
    assert commits, "commits should not be empty"
    for ln in lines:
        assert ln["commit"] in commits
        meta = commits[ln["commit"]]
        assert meta["sha"] == ln["commit"]
        assert meta["author"] == an
        assert meta["message"] == "add source"
        assert meta["date"]


def test_blame_single_file_single_line(repo_env):
    an, c, repo = repo_env
    data = c.get(_p(an, repo) + "/blame?ref=main&path=README.md", expect=200).json()
    assert data["lines"][0]["content"] == "# hello"


def test_blame_after_update_has_two_commits(repo_env):
    an, c, repo = repo_env
    c.post(
        _p(an, repo) + "/commits",
        json={
            "message": "edit line",
            "changes": [{"path": "src/app.ts", "action": "update", "content": "line one\nchanged\n"}],
        },
        expect=201,
    )
    data = c.get(_p(an, repo) + "/blame?ref=main&path=src/app.ts", expect=200).json()
    shas = {ln["commit"] for ln in data["lines"]}
    assert len(shas) == 2
    messages = {m["message"] for m in data["commits"].values()}
    assert {"add source", "edit line"} <= messages
    # 只有第 2 行属于新提交
    by_line = {ln["line"]: ln for ln in data["lines"]}
    assert by_line[1]["commit"] != by_line[2]["commit"]


def test_blame_missing_file_400(repo_env):
    an, c, repo = repo_env
    c.get(_p(an, repo) + "/blame?ref=main&path=missing.txt", expect=400)


def test_blame_bad_ref_400(repo_env):
    an, c, repo = repo_env
    c.get(_p(an, repo) + "/blame?ref=nope&path=src/app.ts", expect=400)


def test_blame_bad_path_400(repo_env):
    an, c, repo = repo_env
    c.get(_p(an, repo) + "/blame?ref=main&path=../escape", expect=400)


def test_blame_requires_auth(client_factory):
    client_factory().get(f"/users/anyone/repos/anything/blame", expect=401)


def test_blame_forbidden_for_stranger(user_factory, repo_env):
    an, c, repo = repo_env
    _, _, stranger = user_factory("s")
    stranger.get(_p(an, repo) + "/blame?ref=main&path=src/app.ts", expect=404)
