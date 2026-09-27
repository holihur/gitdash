"""Markdown 编辑器上传（图片 / 附件）黑盒测试。"""

import struct
import zlib


def _png_bytes() -> bytes:
    """构造一个最小的 1x1 PNG。"""

    def chunk(kind: bytes, data: bytes) -> bytes:
        return (
            struct.pack(">I", len(data))
            + kind
            + data
            + struct.pack(">I", zlib.crc32(kind + data) & 0xFFFFFFFF)
        )

    sig = b"\x89PNG\r\n\x1a\n"
    ihdr = struct.pack(">IIBBBBB", 1, 1, 8, 2, 0, 0, 0)
    raw = b"\x00\xff\x00\x00"
    return sig + chunk(b"IHDR", ihdr) + chunk(b"IDAT", zlib.compress(raw)) + chunk(b"IEND", b"")


def test_upload_requires_auth(anon):
    anon.post("/uploads", files={"file": ("a.png", _png_bytes(), "image/png")}, expect=401)


def test_upload_and_fetch(user_factory):
    _, _, client = user_factory("up")
    data = _png_bytes()
    resp = client.post(
        "/uploads",
        files={"file": ("pixel.png", data, "image/png")},
        expect=201,
    )
    body = resp.json()
    assert body["url"].startswith("/api/uploads/")
    assert body["content_type"] == "image/png"
    assert body["size"] == len(data)

    fetched = client.session.get(client.base + body["url"], timeout=15)
    assert fetched.status_code == 200
    assert fetched.headers["content-type"].startswith("image/png")
    assert fetched.content == data

    # 随机 key 不存在
    missing = client.session.get(client.base + "/api/uploads/does-not-exist", timeout=15)
    assert missing.status_code == 404


def test_upload_bad_paths(user_factory):
    _, _, client = user_factory("upbad")
    # 缺少 file 字段
    client.post("/uploads", data={"x": "y"}, expect=400)
    # 不支持的类型（可执行/未知二进制）
    client.post(
        "/uploads",
        files={"file": ("evil.bin", b"\x7fELF\x02\x01\x01\x00" + b"\x00" * 32, "application/octet-stream")},
        expect=400,
    )
