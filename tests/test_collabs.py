"""仓库协作者 / 共享仓库 —— happy path + bad path（含角色权限与同名仓库）。"""

import pytest


def _p(owner, repo, suffix=""):
    return f"/users/{owner}/repos/{repo}{suffix}"


def _list_repos(client):
    return client.get("/repos", expect=200).json()


def _uuid() -> str:
    import uuid

    return uuid.uuid4().hex[:10]


@pytest.fixture
def collab_env(user_factory):
    """alice 建专属仓库并共享给 bob（write）；结束时删除仓库。"""
    an, _, alice = user_factory("alice")
    bn, _, bob = user_factory("bob")
    repo = f"team-{_uuid()}"
    alice.post("/repos", json={"name": repo}, expect=201)
    alice.post(
        _p(an, repo, "/collabs"), json={"username": bn, "permission": "write"}, expect=200
    )
    yield an, bn, alice, bob, repo
    try:
        alice.delete(f"/repos/{repo}", expect=204)
    except Exception:
        pass


def test_collab_add_list_update_remove(collab_env):
    an, bn, alice, _, repo = collab_env

    collabs = alice.get(_p(an, repo, "/collabs"), expect=200).json()
    assert len(collabs) == 1
    c = collabs[0]
    assert c["username"] == bn and c["permission"] == "write" and c["owner"] == an
    assert "created_at" in c

    # 重复添加可更新权限（upsert）
    alice.post(
        _p(an, repo, "/collabs"), json={"username": bn, "permission": "read"}, expect=200
    )
    assert alice.get(_p(an, repo, "/collabs"), expect=200).json()[0]["permission"] == "read"

    # 移除
    alice.delete(_p(an, repo, f"/collabs/{bn}"), expect=204)
    assert alice.get(_p(an, repo, "/collabs"), expect=200).json() == []


def test_collab_write_can_read_and_write(collab_env):
    an, bn, alice, bob, repo = collab_env

    # write：可读可写（owner 限定 + 旧式单段路由都能解析）
    bob.get(_p(an, repo, "/issues"), expect=200)
    bob.post(_p(an, repo, "/issues"), json={"title": "from bob"}, expect=201)
    bob.get(f"/repos/{repo}/issues", expect=200)

    # bob 不能删除 / 管理协作者
    bob.delete(_p(an, repo), expect=404)
    bob.get(_p(an, repo, "/collabs"), expect=404)
    bob.post(
        _p(an, repo, "/collabs"), json={"username": "x", "permission": "read"}, expect=404
    )

    # 降级 read：仍可读，但不能写
    alice.post(
        _p(an, repo, "/collabs"), json={"username": bn, "permission": "read"}, expect=200
    )
    bob.get(_p(an, repo, "/issues"), expect=200)
    bob.post(_p(an, repo, "/issues"), json={"title": "no"}, expect=404)
    bob.patch(_p(an, repo, "/issues/1"), json={"state": "closed"}, expect=404)
    # read 协作者也不在“可写”仓库列表中暴露 write 权限


def test_collab_list_repos_includes_shared_with_role(collab_env):
    an, _, alice, bob, repo = collab_env

    bob_repos = [(r["owner"], r["name"], r.get("role")) for r in _list_repos(bob)]
    assert (an, repo, "write") in bob_repos

    alice_repos = [(r["owner"], r["name"], r.get("role")) for r in _list_repos(alice)]
    assert (an, repo, "owner") in alice_repos


def test_collab_bad_inputs(collab_env):
    an, _, alice, bob, repo = collab_env

    alice.post(
        _p(an, repo, "/collabs"), json={"username": "x", "permission": "admin"}, expect=400
    )
    alice.post(
        _p(an, repo, "/collabs"), json={"username": "ghost-xx", "permission": "read"}, expect=404
    )
    alice.post(
        _p(an, repo, "/collabs"), json={"username": an, "permission": "read"}, expect=400
    )
    alice.post(
        _p(an, repo, "/collabs"), json={"username": "INVALID USER!", "permission": "read"}, expect=400
    )
    alice.delete(_p(an, repo, "/collabs/carol"), expect=404)
    alice.get(_p(an, "nope", "/collabs"), expect=404)
    bob.delete(_p(an, repo, "/collabs/nobody"), expect=404)  # 非 owner 不能移除


def test_collab_same_name_repos(user_factory):
    an, _, alice = user_factory("alice")
    bn, _, bob = user_factory("bob")
    repo = f"app-{_uuid()}"

    bob.post("/repos", json={"name": repo}, expect=201)
    alice.post("/repos", json={"name": repo}, expect=201)
    alice.post(
        _p(an, repo, "/collabs"), json={"username": bn, "permission": "write"}, expect=200
    )

    # 旧式路由解析到 bob 自己的同名仓库
    bob.get(f"/repos/{repo}/issues", expect=200)
    bob.post(f"/repos/{repo}/issues", json={"title": "mine"}, expect=201)
    # owner 限定路由访问 alice 的仓库
    bob.get(_p(an, repo, "/issues"), expect=200)
    bob.post(_p(an, repo, "/issues"), json={"title": "shared"}, expect=201)

    # 可访问列表包含两个同名不同 owner 的仓库
    shared = [(r["owner"], r["name"]) for r in _list_repos(bob)]
    assert (an, repo) in shared and (bn, repo) in shared


def test_collab_delete_repo_cascades(collab_env):
    an, _, alice, bob, repo = collab_env
    alice.delete(f"/repos/{repo}", expect=204)
    assert all(r["name"] != repo for r in _list_repos(bob))
    bob.get(_p(an, repo), expect=404)


def test_collab_requires_auth(anon, collab_env):
    an, _, _, _, repo = collab_env
    anon.get(_p(an, repo, "/collabs"), expect=401)
    anon.post(_p(an, repo, "/collabs"), json={"username": "x"}, expect=401)
    anon.get(_p(an, repo, "/issues"), expect=401)
