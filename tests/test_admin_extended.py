"""Admin 面板扩展黑盒测试：IP 黑名单、配额、runner 管理、登出、改密。

目的是把 admin 面板中单元测试难以覆盖的端点跑起来（管理端全量路由），
并验证安全语义（会话失效、自锁防护、配额覆盖）。
"""

import uuid

from conftest import ApiClient

ADMIN_USER = "gitdash-admin"
ADMIN_PASS = "admin-test-pass-123456"


def _uuid() -> str:
    return uuid.uuid4().hex[:10]


def test_admin_logout_invalidates_session(admin, base_url):
    """登出必须立即吊销当前管理端会话。"""
    other = ApiClient(base_url)
    other.post("/admin/login", json={"username": ADMIN_USER, "password": ADMIN_PASS}, expect=200)
    other.get("/admin/me", expect=200)
    other.post("/admin/logout", expect=204)
    other.get("/admin/me", expect=401)


def test_admin_ip_ban_crud(admin):
    assert isinstance(admin.get("/admin/ip-bans", expect=200).json(), list)

    # 自锁防护：覆盖当前来源 IP 的条目必须被拒
    admin.post("/admin/ip-bans", json={"cidr": "127.0.0.0/8", "note": "self"}, expect=400)

    # 新增一个不覆盖本机的 CIDR
    created = admin.post(
        "/admin/ip-bans", json={"cidr": "203.0.113.0/24", "note": "test"}, expect=201
    ).json()
    bid = created["id"]

    # 重复新增 409；非法 CIDR 400
    admin.post("/admin/ip-bans", json={"cidr": "203.0.113.0/24"}, expect=409)
    admin.post("/admin/ip-bans", json={"cidr": "not-a-cidr"}, expect=400)

    listed = admin.get("/admin/ip-bans", expect=200).json()
    assert any(b["id"] == bid for b in listed)

    admin.delete(f"/admin/ip-bans/{bid}", expect=204)
    admin.delete(f"/admin/ip-bans/{bid}", expect=404)
    # 非法 id 404
    admin.delete("/admin/ip-bans/abc", expect=404)


def test_admin_quota_roundtrip(admin, user_factory):
    original = admin.get("/admin/quota", expect=200).json()
    try:
        admin.post("/admin/quota", json={"max_repos_per_user": 7}, expect=200)
        got = admin.get("/admin/quota", expect=200).json()
        assert got["default"]["max_repos_per_user"] == 7

        uname, _, _ = user_factory("quota")
        admin.put(f"/admin/quota/user/{uname}", json={"max_repos_per_user": 3}, expect=200)
        overrides = admin.get("/admin/quota", expect=200).json()["overrides"]
        assert any(
            o["scope"] == "user" and o["name"] == uname and o["quota"]["max_repos_per_user"] == 3
            for o in overrides
        )

        # 非法 scope 400；不存在的用户 404
        admin.put("/admin/quota/team/x", json={}, expect=400)
        admin.put(f"/admin/quota/user/none-{_uuid()}", json={}, expect=404)

        admin.delete(f"/admin/quota/user/{uname}", expect=204)
        admin.delete("/admin/quota/team/x", expect=400)
    finally:
        admin.post("/admin/quota", json=original["default"], expect=200)


def test_admin_runner_management(admin):
    assert isinstance(admin.get("/admin/runners", expect=200).json(), list)

    token = admin.post("/admin/runners/registration-token", expect=201).json()
    assert token["token"]
    # 内部约定：scope 为空表示全局（前端把 "" 渲染为 global）
    assert token["scope"] == ""

    # 不存在的 runner 删除 → 404（存在路径需要 Redis，另有 Redis 用例覆盖）
    admin.delete(f"/admin/runners/nope-{_uuid()}", expect=404)


def test_admin_password_change_revokes_other_sessions(admin, base_url):
    """改密后，其它已登录的管理端会话必须失效（与用户改密语义一致）。"""
    other = ApiClient(base_url)
    other.post("/admin/login", json={"username": ADMIN_USER, "password": ADMIN_PASS}, expect=200)
    new_pass = "admin-new-pass-" + _uuid()
    other.post(
        "/admin/password",
        json={"current_password": ADMIN_PASS, "new_password": new_pass},
        expect=204,
    )
    try:
        stale = admin.get("/admin/me")
    finally:
        # 用仍在线的 other 会话改回原密码，并刷新 admin fixture 的会话
        other.post(
            "/admin/password",
            json={"current_password": new_pass, "new_password": ADMIN_PASS},
            expect=204,
        )
        admin.post("/admin/login", json={"username": ADMIN_USER, "password": ADMIN_PASS}, expect=200)

    assert stale.status_code == 401, "admin password change must invalidate other admin sessions"


def test_admin_change_password_bad_current(admin):
    admin.post(
        "/admin/password",
        json={"current_password": "definitely-wrong", "new_password": "whatever-pass-123"},
        expect=401,
    )
