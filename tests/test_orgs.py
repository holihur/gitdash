"""组织 API —— 创建 / 列表 / 成员 / 仓库 / 删除。"""

import pytest


def _uuid() -> str:
    import uuid
    return uuid.uuid4().hex[:10]


@pytest.fixture
def user(user_factory):
    return user_factory("o")


def test_org_lifecycle(user_factory, user):
    owner, _, c = user
    other, _, c2 = user_factory("ox")
    stranger, _, c3 = user_factory("os")

    # 创建
    org = c.post("/orgs", json={"name": f"org-{_uuid()}", "display": "Org"}, expect=201).json()
    name = org["name"]
    assert c.get(f"/orgs/{name}", expect=200).json()["role"] == "owner"

    # 列表包含该组织
    assert any(o["name"] == name for o in c.get("/orgs", expect=200).json())

    # 坏路径：非法名 / 重复名
    c.post("/orgs", json={"name": "a", "display": ""}, expect=400)
    c.post("/orgs", json={"name": name, "display": ""}, expect=409)

    # 非成员视角：get / members 全部 404
    c2.get(f"/orgs/{name}", expect=404)
    c2.get(f"/orgs/{name}/members", expect=404)
    # 非成员不能删除
    c2.delete(f"/orgs/{name}", expect=404)

    # 非 owner 不能加人
    c2.post(f"/orgs/{name}/members", json={"username": other}, expect=404)

    # owner 添加成员
    c.post(f"/orgs/{name}/members", json={"username": other, "role": "member"}, expect=200)
    members = c.get(f"/orgs/{name}/members", expect=200).json()
    assert {m["username"] for m in members} >= {owner, other}
    assert any(m["username"] == other and m["role"] == "member" for m in members)

    # 添加成员坏路径：用户不存在 / 非法角色
    c.post(f"/orgs/{name}/members", json={"username": f"no-{_uuid()}"}, expect=404)
    c.post(f"/orgs/{name}/members", json={"username": other, "role": "admin"}, expect=400)

    # last owner 保护：不能移除唯一的 owner（自己移除自己）
    c.delete(f"/orgs/{name}/members/{owner}", expect=400)

    # 组织仓库：成员可把仓库建到组织下
    repo_name = f"repo-{_uuid()}"
    c.post("/repos", json={"name": repo_name, "namespace": name, "private": False}, expect=201)
    repos = c.get(f"/orgs/{name}/repos", expect=200).json()
    assert repos["role"] == "owner"
    assert [r["name"] for r in repos["repos"]] == [repo_name]

    # 非成员不能在组织下建仓库
    c3.post("/repos", json={"name": f"repo-{_uuid()}", "namespace": name}, expect=403)

    # 组织非空：不能删除
    c.delete(f"/orgs/{name}", expect=409)

    # 删除仓库后可删除组织
    c.delete(f"/users/{name}/repos/{repo_name}", expect=204)
    c.delete(f"/orgs/{name}", expect=204)
    c.get(f"/orgs/{name}", expect=404)
    c.get(f"/orgs/{name}/members", expect=404)
    assert stranger is not None


def test_org_requires_auth(anon):
    anon.post("/orgs", json={"name": "no-auth-org", "display": ""}, expect=401)
    anon.get("/orgs", expect=401)
