"""全站通知（/api/announcement）：公开读接口 + 管理端配置。

该接口为公开路由（登录与否都可读），也是黑盒端点覆盖率门禁要求的命中点。
"""

from __future__ import annotations


def test_announcement_disabled_by_default(anon):
    r = anon.get("/announcement", expect=200)
    body = r.json()
    assert body.get("enabled") is False


def test_announcement_enabled_and_payload(admin, anon):
    orig = admin.get("/admin/settings", expect=200).json()
    try:
        admin.post(
            "/admin/settings",
            json={
                "announcement_enabled": True,
                "announcement_level": "warning",
                "announcement_title": "Scheduled maintenance",
                "announcement_message": "Tonight 22:00 UTC",
            },
            expect=200,
        )
        body = anon.get("/announcement", expect=200).json()
        assert body["enabled"] is True
        assert body["level"] == "warning"
        assert body["title"] == "Scheduled maintenance"
        assert body["message"] == "Tonight 22:00 UTC"
        assert body.get("id")  # 内容指纹，非空
    finally:
        # 恢复原值，避免影响其他用例（admin 为会话级 fixture）。
        admin.post(
            "/admin/settings",
            json={
                "announcement_enabled": orig.get("announcement_enabled", False),
                "announcement_level": orig.get("announcement_level", ""),
                "announcement_title": orig.get("announcement_title", ""),
                "announcement_message": orig.get("announcement_message", ""),
            },
            expect=200,
        )
