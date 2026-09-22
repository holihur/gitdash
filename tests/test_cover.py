"""用户 / 组织封面黑盒 API 测试：上传 / 读取 / 校验 / 删除。

与后端完全隔离，仅通过 HTTP API 验证。
"""
from __future__ import annotations

import uuid

# 仅需前 8 字节即被 http.DetectContentType 识别为 image/png
PNG = b"\x89PNG\r\n\x1a\n" + b"fake-png-body"


def test_user_cover_lifecycle(user_factory):
    username, _token, c = user_factory("cv")

    # 初始无封面
    assert c.get("/me", expect=200).json().get("cover_url", "") == ""
    c.get(f"/users/{username}/cover", expect=404)

    # 上传
    r = c.post("/me/cover", files={"cover": ("c.png", PNG, "image/png")}, expect=200).json()
    assert r["cover_url"].endswith(f"/users/{username}/cover")

    # /me 与用户主页均暴露封面地址
    assert c.get("/me", expect=200).json()["cover_url"]
    assert c.get(f"/users/{username}", expect=200).json()["cover_url"]

    # 读取封面
    img = c.get(f"/users/{username}/cover", expect=200)
    assert img.headers["Content-Type"].startswith("image/png")
    assert img.content == PNG

    # 非图片被拒绝；缺失字段被拒绝
    c.post("/me/cover", files={"cover": ("x.txt", b"hello world", "text/plain")}, expect=400)
    c.post("/me/cover", data={"x": "y"}, expect=400)

    # 删除
    c.delete("/me/cover", expect=200)
    c.get(f"/users/{username}/cover", expect=404)
    assert c.get("/me", expect=200).json().get("cover_url", "") == ""


def test_org_cover_lifecycle(user_factory):
    _owner, _token, c = user_factory("cvo")
    _other, _token2, c2 = user_factory("cvx")

    name = c.post(
        "/orgs",
        json={"name": f"cover-{uuid.uuid4().hex[:10]}", "display": ""},
        expect=201,
    ).json()["name"]

    # 初始无封面
    assert c.get(f"/orgs/{name}/profile", expect=200).json().get("cover_url", "") == ""
    c.get(f"/orgs/{name}/cover", expect=404)

    # 非 owner 不能上传 / 删除
    c2.post(f"/orgs/{name}/cover", files={"cover": ("c.png", PNG, "image/png")}, expect=404)
    c2.delete(f"/orgs/{name}/cover", expect=404)

    # owner 上传
    r = c.post(f"/orgs/{name}/cover", files={"cover": ("c.png", PNG, "image/png")}, expect=200).json()
    assert r["cover_url"].endswith(f"/orgs/{name}/cover")
    assert c.get(f"/orgs/{name}/profile", expect=200).json()["cover_url"]

    img = c.get(f"/orgs/{name}/cover", expect=200)
    assert img.content == PNG

    # 删除
    c.delete(f"/orgs/{name}/cover", expect=200)
    c.get(f"/orgs/{name}/cover", expect=404)

    # 清理：测试夹具关闭同名仓库自动创建，直接删除组织
    c.delete(f"/orgs/{name}", expect=204)
