"""原始文件查看（图片 / PDF 直链）黑盒测试。

验证 /raw 端点以正确的 Content-Type 返回字节，并允许同源内联预览。
"""
from __future__ import annotations

import uuid


def _uuid() -> str:
    return uuid.uuid4().hex[:10]


def test_raw_file_preview(user_factory):
    owner, _token, c = user_factory("rw")
    repo = f"raw-{_uuid()}"
    c.post("/repos", json={"name": repo}, expect=201)
    c.post(
        f"/users/{owner}/repos/{repo}/commits",
        json={
            "message": "add assets",
            "changes": [
                {"path": "pic.png", "action": "create", "content": "fake-png-bytes"},
                {"path": "doc.pdf", "action": "create", "content": "%PDF-1.4 fake"},
                {"path": "notes.txt", "action": "create", "content": "plain text"},
            ],
        },
        expect=201,
    )
    try:
        # 图片：image/png + inline
        img = c.get(f"/users/{owner}/repos/{repo}/raw?ref=main&path=pic.png", expect=200)
        assert img.headers["Content-Type"].startswith("image/png")
        assert img.headers["Content-Disposition"] == "inline"
        assert img.headers.get("X-Frame-Options") == "SAMEORIGIN"
        assert img.content == b"fake-png-bytes"

        # PDF：application/pdf + inline
        pdf = c.get(f"/users/{owner}/repos/{repo}/raw?path=doc.pdf", expect=200)
        assert pdf.headers["Content-Type"].startswith("application/pdf")
        assert pdf.headers["Content-Disposition"] == "inline"
        assert pdf.content == b"%PDF-1.4 fake"

        # 非预览类型强制下载
        txt = c.get(f"/users/{owner}/repos/{repo}/raw?path=notes.txt", expect=200)
        assert txt.headers["Content-Disposition"] == "attachment"

        # 简写路由（/api/repos/{name}/raw）
        c.get(f"/repos/{repo}/raw?ref=main&path=pic.png", expect=200)

        # 不存在的文件 -> 404
        c.get(f"/users/{owner}/repos/{repo}/raw?path=nope.png", expect=404)
    finally:
        try:
            c.delete(f"/repos/{repo}", expect=204)
        except Exception:
            pass
