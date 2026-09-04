"""仓库 API —— happy path + bad path（含多用户隔离）。"""

import pytest


def _list_repos(client) -> list[dict]:
    return client.get("/repos", expect=200).json()


def test_repo_create_list_get_delete(user_factory):
    username, _, client = user_factory()
    name = f"r-{uuid4hex()}"
    client.post("/repos", json={"name": name}, expect=201)

    # list 包含刚创建的仓库
    names = [r["name"] for r in _list_repos(client)]
    assert name in names

    # get 返回完整字段
    repo = client.get(f"/repos/{name}", expect=200).json()
    assert repo["name"] == name
    assert repo["owner"] == username
    assert "created_at" in repo

    # 删除后不可见、不可取
    client.delete(f"/repos/{name}", expect=204)
    assert name not in [r["name"] for r in _list_repos(client)]
    client.get(f"/repos/{name}", expect=404)


def test_repo_description_trimmed_and_owner(user_factory):
    username, _, client = user_factory()
    name = f"r-{uuid4hex()}"
    client.post(
        "/repos", json={"name": name, "description": "  my first repo  "}, expect=201
    )
    repo = client.get(f"/repos/{name}", expect=200).json()
    assert repo["owner"] == username
    assert repo["description"] == "my first repo"
    client.delete(f"/repos/{name}", expect=204)


def test_repo_duplicate_409(repo_factory):
    name, client = repo_factory()
    client.post("/repos", json={"name": name}, expect=409)


@pytest.mark.parametrize(
    "name",
    ["", "-lead", ".hidden", "has space", "a/b"],
)
def test_repo_invalid_name_400(user_factory, name):
    _, _, client = user_factory()
    client.post("/repos", json={"name": name}, expect=400)


def test_repo_get_missing_404(user_factory):
    _, _, client = user_factory()
    client.get(f"/repos/nope-{uuid4hex()}", expect=404)


def test_repo_delete_twice(user_factory):
    """删除不存在的仓库返回 404。"""
    _, _, client = user_factory()
    name = f"r-{uuid4hex()}"
    client.post("/repos", json={"name": name}, expect=201)
    client.delete(f"/repos/{name}", expect=204)
    client.delete(f"/repos/{name}", expect=404)


def test_repo_user_isolation(user_factory):
    _, _, alice = user_factory("alice")
    _, _, bob = user_factory("bob")
    repo = f"shared-{uuid4hex()}"

    alice.post("/repos", json={"name": repo}, expect=201)

    # bob 看不到、取不到、删不掉 alice 的仓库
    bob.get(f"/repos/{repo}", expect=404)
    bob.delete(f"/repos/{repo}", expect=404)

    # 列表隔离
    assert repo not in [r["name"] for r in _list_repos(bob)]
    assert repo in [r["name"] for r in _list_repos(alice)]

    # bob 可以建同名仓库，互不影响
    bob.post("/repos", json={"name": repo}, expect=201)
    assert len(_list_repos(bob)) == 1
    alice.delete(f"/repos/{repo}", expect=204)
    # alice 删除自己的不影响 bob 的
    assert repo in [r["name"] for r in _list_repos(bob)]
    bob.delete(f"/repos/{repo}", expect=204)


def test_repo_empty_description_ok(user_factory):
    _, _, client = user_factory()
    name = f"r-{uuid4hex()}"
    client.post("/repos", json={"name": name, "description": ""}, expect=201)
    client.delete(f"/repos/{name}", expect=204)


def uuid4hex() -> str:
    import uuid

    return uuid.uuid4().hex[:10]
