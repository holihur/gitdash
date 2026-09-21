"""第三方账号绑定（GitHub/GitLab/Gitea/Bitbucket）与批量导入。

这些平台默认未配置，因此这里主要验证「未启用 / 未绑定」时的安全降级与错误码，
并覆盖对应路由；完整的 OAuth 交换流程由后端 Go 集成测试
（backend/tests/connections_test.go）用 mock provider 覆盖。
"""


def test_list_connections_reports_providers(user_factory):
    _, _, c = user_factory("conn")
    rows = c.get("/connections", expect=200).json()
    providers = {r["provider"] for r in rows}
    assert {"github", "gitlab", "gitea", "bitbucket"} <= providers
    # 默认全部未启用、未绑定
    assert all(r["enabled"] is False and r["connected"] is False for r in rows)


def test_connection_routes_require_auth(anon):
    anon.get("/connections", expect=401)
    anon.get("/connections/gitlab/start", expect=401)
    anon.delete("/connections/gitlab", expect=401)
    anon.get("/connections/gitlab/repos", expect=401)


def test_connection_routes_disabled_provider(user_factory):
    _, _, c = user_factory("conn")
    # 未启用的 provider：start / repos / delete 均为 404
    c.get("/connections/gitlab/start", expect=404)
    c.get("/connections/gitlab/repos", expect=404)
    c.delete("/connections/gitlab", expect=404)
    # 回调：未启用时重定向回个人页并带错误信息（authOptional）
    r = c.session.get(c.base + "/api/connections/gitlab/callback", allow_redirects=False)
    assert r.status_code == 302
    assert "connect_error=" in r.headers.get("Location", "")


def test_batch_import_requires_connection(user_factory):
    _, _, c = user_factory("conn")
    # 未绑定第三方账号：批量导入 404 not_connected（provider 未启用时为 connect_disabled）
    c.post("/imports/batch", json={"provider": "gitlab", "repos": ["group/proj"]}, expect=404)
