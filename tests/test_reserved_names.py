"""注册保留名黑名单：名单内用户名禁止自助注册，但管理员可创建。"""

import uuid


def _name() -> str:
    return f"res-{uuid.uuid4().hex[:8]}"


def test_reserved_names_lifecycle(admin, anon):
    name = _name()

    # 初始列表可读
    listed = admin.get("/admin/reserved-names", expect=200).json()
    assert isinstance(listed.get("names"), list)

    # 新增
    admin.post("/admin/reserved-names", json={"name": name}, expect=201)
    listed = admin.get("/admin/reserved-names", expect=200).json()
    assert name in listed["names"]

    # 重复 → 409；非法名 → 400
    admin.post("/admin/reserved-names", json={"name": name}, expect=409)
    admin.post("/admin/reserved-names", json={"name": "Bad Name!"}, expect=400)

    # 自助注册被拒（403 username_reserved）
    r = anon.post(
        "/auth/register",
        json={"username": name, "password": "test-pass-123456"},
    )
    assert r.status_code == 403, r.text
    assert r.json().get("code") == "username_reserved"

    # 管理员仍可创建该用户
    admin.post(
        "/admin/users",
        json={"username": name, "password": "test-pass-123456"},
        expect=201,
    )

    # 移除保留名 → 该名字恢复可注册（用户已存在，这里只验证 204/404）
    admin.delete(f"/admin/reserved-names/{name}", expect=204)
    admin.delete(f"/admin/reserved-names/{name}", expect=404)

    # 清理用户
    admin.delete(f"/admin/users/{name}", expect=204)
