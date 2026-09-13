"""用户头像黑盒 API 测试：上传 / 读取 / 校验 / 删除。

与后端完全隔离，仅通过 HTTP API 验证。
"""
from __future__ import annotations

# 仅需前 8 字节即被 http.DetectContentType 识别为 image/png
PNG = b"\x89PNG\r\n\x1a\n" + b"fake-png-body"


def test_avatar_lifecycle(user_factory):
    username, _token, c = user_factory("av")

    # 初始无头像
    me = c.get("/me", expect=200).json()
    assert me.get("avatar_url", "") == ""
    c.get(f"/users/{username}/avatar", expect=404)

    # 上传
    r = c.post("/me/avatar", files={"avatar": ("a.png", PNG, "image/png")}, expect=200).json()
    assert r["avatar_url"].endswith(f"/users/{username}/avatar")

    # /me 与用户主页均暴露头像地址
    assert c.get("/me", expect=200).json()["avatar_url"]
    assert c.get(f"/users/{username}", expect=200).json()["avatar_url"]

    # 读取头像
    img = c.get(f"/users/{username}/avatar", expect=200)
    assert img.headers["Content-Type"].startswith("image/png")
    assert img.content == PNG

    # 非图片被拒绝
    c.post("/me/avatar", files={"avatar": ("x.txt", b"hello world", "text/plain")}, expect=400)

    # 删除
    c.delete("/me/avatar", expect=200)
    c.get(f"/users/{username}/avatar", expect=404)
    assert c.get("/me", expect=200).json().get("avatar_url", "") == ""


def test_avatar_requires_upload_field(user_factory):
    _username, _token, c = user_factory("avf")
    c.post("/me/avatar", data={"x": "y"}, expect=400)
