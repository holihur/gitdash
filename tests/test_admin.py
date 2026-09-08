"""Admin 面板黑盒测试：登录/会话、设置、用户管理（重置密码撤会话、删除用户）。

BIN 自启实例已启用 admin（见 conftest）；外部实例未启用时整组跳过。
"""

import uuid

import pytest


def _uuid() -> str:
    return uuid.uuid4().hex[:10]


def test_admin_login_bad_credentials(admin):
    other = type(admin)(admin.base)  # 不复用会话 cookie
    r = other.post("/admin/login", json={"username": "gitdash-admin", "password": "wrong"})
    assert r.status_code == 401
    other.get("/admin/me", expect=401)


def test_admin_pat_forbidden(admin, user_factory):
    _, _, client = user_factory()
    tok = client.post("/tokens", json={"name": "t", "scopes": ["repo"]}, expect=201).json()["token"]
    from conftest import ApiClient
    pat = ApiClient(admin.base, tok)
    r = pat.post("/admin/login", json={"username": "gitdash-admin", "password": "admin-test-pass-123456"})
    assert r.status_code == 403
    pat.get("/admin/users", expect=401)


def test_admin_settings_roundtrip(admin):
    orig = admin.get("/admin/settings", expect=200).json()
    try:
        admin.post("/admin/settings",
                   json={"oidc_enabled": True, "oidc_name": "TestOIDC",
                         "oidc_issuer": "https://sso.example.com", "oidc_client_id": "cid"},
                   expect=200)
        s = admin.get("/admin/settings", expect=200).json()
        assert s["oidc_enabled"] is True and s["oidc_name"] == "TestOIDC"
        assert s["oidc_issuer"] == "https://sso.example.com"
        # secret 只写不读
        admin.post("/admin/settings", json={"oidc_client_secret": "sec"}, expect=200)
        assert admin.get("/admin/settings", expect=200).json()["oidc_has_secret"] is True
    finally:
        admin.post("/admin/settings",
                   json={"oidc_enabled": orig["oidc_enabled"], "oidc_name": "",
                         "oidc_issuer": "", "oidc_client_id": ""}, expect=200)


def test_admin_create_user_and_reset_password_revokes_sessions(admin, client_factory):
    uname = f"adm-{_uuid()}"
    admin.post("/admin/users", json={"username": uname, "password": "init-pass-123456"}, expect=201)

    # 重名 409 / 弱密码 400 / 坏用户名 400
    admin.post("/admin/users", json={"username": uname, "password": "init-pass-123456"}, expect=409)
    admin.post("/admin/users", json={"username": f"x-{_uuid()}", "password": "short"}, expect=400)
    admin.post("/admin/users", json={"username": "bad..name", "password": "init-pass-123456"}, expect=400)

    # 用户登录拿 token
    c = client_factory()
    tok = c.post("/auth/login", json={"username": uname, "password": "init-pass-123456"},
                 expect=200).json()["token"]
    user = client_factory(tok)
    user.get("/me", expect=200)

    # admin ?q= 能搜到该用户
    found = admin.get(f"/admin/users?q={uname}", expect=200).json()
    assert uname in [u["username"] for u in found]

    # 重置密码 → 旧会话立刻失效，新密码可登录
    admin.post(f"/admin/users/{uname}/reset_password",
               json={"password": "new-pass-123456"}, expect=204)
    user.get("/me", expect=401)
    client_factory().post("/auth/login",
                          json={"username": uname, "password": "new-pass-123456"}, expect=200)

    # 删除用户 → 404，重复删除 404
    admin.delete(f"/admin/users/{uname}", expect=204)
    admin.delete(f"/admin/users/{uname}", expect=404)
    client_factory().post("/auth/login",
                          json={"username": uname, "password": "new-pass-123456"}, expect=401)


def test_admin_requires_auth(admin, anon):
    anon.get("/admin/users", expect=401)
    anon.get("/admin/settings", expect=401)
    anon.post("/admin/users", json={"username": "x", "password": "x-pass-123456"}, expect=401)
