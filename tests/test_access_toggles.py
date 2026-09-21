"""Admin 访问控制开关：Swagger/OpenAPI 与账号密码登录的开启/关闭。

两项默认开启；管理员可在 /api/admin/settings 中关闭。用例结束后必须恢复默认，
避免污染其它黑盒用例（同一实例会话内共享全局设置）。
"""

import uuid


def _uuid() -> str:
    return uuid.uuid4().hex[:10]


def _set_toggles(admin, *, swagger=None, password=None, version=None):
    body = {}
    if swagger is not None:
        body["swagger_enabled"] = swagger
    if password is not None:
        body["password_login_enabled"] = password
    if version is not None:
        body["version_visible"] = version
    if body:
        admin.post("/admin/settings", json=body, expect=200)


def test_admin_settings_exposes_toggles(admin):
    s = admin.get("/admin/settings", expect=200).json()
    assert isinstance(s["swagger_enabled"], bool)
    assert isinstance(s["password_login_enabled"], bool)
    assert isinstance(s["version_visible"], bool)


def test_swagger_toggle(admin, anon):
    orig = admin.get("/admin/settings", expect=200).json()["swagger_enabled"]
    try:
        _set_toggles(admin, swagger=False)
        anon.get("/openapi.json", expect=404)
        assert anon.session.get(f"{anon.base}/api/swagger/", timeout=10).status_code == 404
        assert anon.session.get(f"{anon.base}/api/swagger", timeout=10, allow_redirects=False).status_code == 404

        _set_toggles(admin, swagger=True)
        anon.get("/openapi.json", expect=200)
        assert anon.session.get(f"{anon.base}/api/swagger/", timeout=10).status_code == 200
    finally:
        _set_toggles(admin, swagger=orig)


def test_version_visibility_toggle(admin, anon):
    orig = admin.get("/admin/settings", expect=200).json()["version_visible"]
    try:
        _set_toggles(admin, version=False)
        assert anon.get("/version", expect=200).json()["version"] == ""
        assert anon.get("/instance", expect=200).json()["version"] == ""

        _set_toggles(admin, version=True)
        assert anon.get("/version", expect=200).json()["version"]
    finally:
        _set_toggles(admin, version=orig)


def test_password_login_toggle(admin, client_factory, anon):
    username = f"tog-{_uuid()}"
    password = "test-pass-123456"
    client_factory().post("/auth/register", json={"username": username, "password": password}, expect=201)

    orig = admin.get("/admin/settings", expect=200).json()["password_login_enabled"]
    try:
        _set_toggles(admin, password=False)

        # providers 明确告知密码登录已关闭
        prov = anon.get("/auth/providers", expect=200).json()
        assert prov["password"]["enabled"] is False
        assert prov["register"]["enabled"] is False
        assert prov["password_reset"]["enabled"] is False

        client_factory().post(
            "/auth/login", json={"username": username, "password": password}, expect=403
        )
        client_factory().post(
            "/auth/register", json={"username": f"x-{_uuid()}", "password": password}, expect=403
        )

        _set_toggles(admin, password=True)
        prov = anon.get("/auth/providers", expect=200).json()
        assert prov["password"]["enabled"] is True
        client_factory().post(
            "/auth/login", json={"username": username, "password": password}, expect=200
        )
    finally:
        _set_toggles(admin, password=orig)
