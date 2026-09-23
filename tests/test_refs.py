"""分支 / 标签管理 API —— happy path + bad path + 权限。"""

import pytest


def _uuid() -> str:
    import uuid

    return uuid.uuid4().hex[:10]


def _r(owner, repo):
    return f"/users/{owner}/repos/{repo}"


@pytest.fixture
def repo_env(user_factory):
    an, _, c = user_factory("r")
    repo = f"refs-{_uuid()}"
    c.post("/repos", json={"name": repo}, expect=201)
    # 制造 main 上的提交
    c.post(_r(an, repo) + "/commits", json={
        "message": "m1",
        "changes": [{"path": "a.txt", "action": "create", "content": "1"}]}, expect=201)
    c.post(_r(an, repo) + "/commits", json={
        "message": "m2",
        "changes": [{"path": "b.txt", "action": "create", "content": "2"}]}, expect=201)
    yield an, c, repo
    try:
        c.delete(f"/repos/{repo}", expect=204)
    except Exception:
        pass


def test_branch_tag_management(repo_env):
    an, c, repo = repo_env

    # 标签：空 → 创建 → 列表 → 重复 409
    assert c.get(_r(an, repo) + "/tags", expect=200).json() == []
    m = c.post(_r(an, repo) + "/refs",
               json={"type": "tag", "name": "v1.0", "from": "main"}, expect=201).json()
    assert m["name"] == "v1.0" and len(m["sha"]) == 40
    tags = c.get(_r(an, repo) + "/tags", expect=200).json()
    assert [t["name"] for t in tags] == ["v1.0"]
    c.post(_r(an, repo) + "/refs", json={"type": "tag", "name": "v1.0"}, expect=409)

    # 分支（含斜杠命名）
    c.post(_r(an, repo) + "/refs",
           json={"type": "branch", "name": "feature/dev", "from": "main"}, expect=201)
    branches = c.get(f"/repos/{repo}/branches", expect=200).json()
    assert "feature/dev" in [b["name"] for b in branches]

    # bad path
    c.post(_r(an, repo) + "/refs", json={"type": "branch", "name": "bad..name"}, expect=400)
    c.post(_r(an, repo) + "/refs", json={"type": "branch", "name": "x", "from": "ghost"}, expect=400)
    c.post(_r(an, repo) + "/refs", json={"type": "misc", "name": "x"}, expect=400)

    # 删除 tag/branch；HEAD 分支不可删
    c.delete(_r(an, repo) + "/refs/tag/v1.0", expect=204)
    c.delete(_r(an, repo) + "/refs/tag/v1.0", expect=404)
    c.delete(_r(an, repo) + "/refs/branch/feature%2Fdev", expect=204)
    c.delete(_r(an, repo) + "/refs/branch/main", expect=409)
    c.delete(_r(an, repo) + "/refs/branch/nope", expect=404)

    # 未认证
    c.get(_r(an, repo) + "/tags", expect=200)


def test_ref_note(repo_env):
    """分支/标签备注：存在性校验 + 设置 / 读取 / 清除。"""
    an, c, repo = repo_env
    base = _r(an, repo)
    c.put(f"{base}/refs/branch/ghost/note", json={"note": "x"}, expect=404)
    c.put(f"{base}/refs/misc/main/note", json={"note": "x"}, expect=400)
    got = c.put(f"{base}/refs/branch/main/note", json={"note": "primary"}, expect=200).json()
    assert got["kind"] == "branch" and got["name"] == "main" and got["note"] == "primary"
    # 复数写法归一化
    got = c.put(f"{base}/refs/branches/main/note", json={"note": "p2"}, expect=200).json()
    assert got["kind"] == "branch"
    # 空串清除
    got = c.put(f"{base}/refs/branch/main/note", json={"note": ""}, expect=200).json()
    assert got["note"] == ""


def test_refs_permissions(repo_env, user_factory):
    an, c, repo = repo_env
    # read 协作者可看 tags 不可创建
    bn, _, bob = user_factory("bob")
    c.post(_r(an, repo) + "/collabs", json={"username": bn, "permission": "read"}, expect=200)
    bob.get(_r(an, repo) + "/tags", expect=200)
    bob.post(_r(an, repo) + "/refs", json={"type": "tag", "name": "v1"}, expect=404)
    # write 协作者可创建；非协作者不可见
    c.post(_r(an, repo) + "/collabs", json={"username": bn, "permission": "write"}, expect=200)
    bob.post(_r(an, repo) + "/refs", json={"type": "tag", "name": "v1"}, expect=201)
    _, _, other = user_factory("x")
    other.get(_r(an, repo) + "/tags", expect=404)
