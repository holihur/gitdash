"""网页文件/文件夹 CRUD(提交式) —— happy path + bad path + 权限。"""

import pytest


def _uuid() -> str:
    import uuid

    return uuid.uuid4().hex[:10]


def _p(owner, repo):
    return f"/users/{owner}/repos/{repo}"


@pytest.fixture
def repo_env(user_factory):
    an, _, c = user_factory("f")
    repo = f"fops-{_uuid()}"
    c.post("/repos", json={"name": repo}, expect=201)
    yield an, c, repo
    try:
        c.delete(f"/repos/{repo}", expect=204)
    except Exception:
        pass


def _commit(c, owner, repo, message, changes):
    return c.post(_p(owner, repo) + "/commits", json={"message": message, "changes": changes}, expect=201).json()


def test_file_crud_flow(repo_env):
    an, c, repo = repo_env

    # 新建嵌套文件（空仓库自动建 main）
    _commit(c, an, repo, "add docs", [
        {"path": "src/app.ts", "action": "create", "content": "export const a = 1;\n"},
        {"path": "README.md", "action": "create", "content": "# hello\n"},
    ])
    branches = c.get(f"/repos/{repo}/branches", expect=200).json()
    assert branches[0]["name"] == "main"

    tree = c.get(f"/repos/{repo}/tree?ref=main&path=src", expect=200).json()["entries"]
    assert [e["name"] for e in tree] == ["app.ts"]

    # 更新
    _commit(c, an, repo, "edit", [{"path": "src/app.ts", "action": "update", "content": "export const a = 2;\n"}])
    blob = c.get(f"/repos/{repo}/blob?ref=main&path=src/app.ts", expect=200).json()
    assert blob["content"] == "export const a = 2;\n"

    # 删除文件
    _commit(c, an, repo, "rm readme", [{"path": "README.md", "action": "delete"}])
    c.get(f"/repos/{repo}/blob?ref=main&path=README.md", expect=400)

    # 删除目录（递归）
    _commit(c, an, repo, "rm src", [{"path": "src", "action": "delete_tree"}])
    entries = c.get(f"/repos/{repo}/tree?ref=main&path=", expect=200).json()["entries"]
    assert entries == []


def test_file_ops_bad_paths(repo_env):
    an, c, repo = repo_env
    # 非法路径 / 空 message / 不存在的文件 / 非法 action
    c.post(_p(an, repo) + "/commits", json={
        "message": "x", "changes": [{"path": "../evil", "action": "create"}]}, expect=400)
    c.post(_p(an, repo) + "/commits", json={
        "message": "", "changes": [{"path": "a.txt", "action": "create"}]}, expect=400)
    c.post(_p(an, repo) + "/commits", json={
        "message": "x", "changes": [{"path": "missing.txt", "action": "delete"}]}, expect=400)
    c.post(_p(an, repo) + "/commits", json={
        "message": "x", "changes": [{"path": "a.txt", "action": "chmod"}]}, expect=400)
    c.post(_p(an, repo) + "/commits", json={"message": "x", "changes": []}, expect=400)
    # 指定分支（新分支 feature）
    c.post(_p(an, repo) + "/commits", json={
        "branch": "feature/x", "message": "feat",
        "changes": [{"path": "f.txt", "action": "create", "content": "1"}]}, expect=201)
    feat = c.get(f"/repos/{repo}/tree?ref=feature/x&path=", expect=200).json()["entries"]
    assert any(e["name"] == "f.txt" for e in feat)


def test_file_ops_permissions(repo_env, user_factory, anon):
    an, c, repo = repo_env
    # write 协作者可提交
    bn, _, bob = user_factory("bob")
    c.post(_p(an, repo) + "/collabs", json={"username": bn, "permission": "write"}, expect=200)
    bob.post(_p(an, repo) + "/commits", json={
        "message": "bob", "changes": [{"path": "b.txt", "action": "create", "content": "b"}]}, expect=201)
    # 降级 read 后不可提交
    c.post(_p(an, repo) + "/collabs", json={"username": bn, "permission": "read"}, expect=200)
    bob.post(_p(an, repo) + "/commits", json={
        "message": "no", "changes": [{"path": "c.txt", "action": "create"}]}, expect=404)
    # 非协作者 / 未认证
    _, _, other = user_factory("x")
    other.post(_p(an, repo) + "/commits", json={"message": "x"}, expect=404)
    anon.post(_p(an, repo) + "/commits", json={"message": "x"}, expect=401)
