"""用户资料 / 密码修改 / MFA(TOTP) —— happy path + bad path。"""

import base64
import hashlib
import hmac
import struct
import time

import pytest


def _uuid() -> str:
    import uuid

    return uuid.uuid4().hex[:10]


def _totp(secret: str, t: float | None = None) -> str:
    """与服务端同机生成 TOTP 验证码（RFC 6238, SHA1, 6 位, 30s）。"""
    key = base64.b32decode(secret.upper())
    counter = int((t if t is not None else time.time()) // 30)
    digest = hmac.new(key, struct.pack(">Q", counter), hashlib.sha1).digest()
    offset = digest[-1] & 0x0F
    value = (struct.unpack(">I", digest[offset : offset + 4])[0] & 0x7FFFFFFF) % 1_000_000
    return f"{value:06d}"


@pytest.fixture
def user(user_factory):
    return user_factory("p")


def test_profile_me_fields(user):
    _, _, c = user
    me = c.get("/me", expect=200).json()
    assert set(me) == {"username", "created_at", "mfa_enabled"}
    assert me["mfa_enabled"] is False
    assert me["created_at"]


def test_password_change_flow(user):
    username, _, c = user
    # 改密前旧密码可登录
    assert c.post(
        "/auth/login", json={"username": username, "password": "test-pass-123456"}, expect=200
    ).json()["token"]

    # 错误旧密码 / 弱新密码
    c.post(
        "/me/password",
        json={"current_password": "wrong-old", "new_password": "x-new-pass-1"},
        expect=401,
    )
    c.post(
        "/me/password",
        json={"current_password": "test-pass-123456", "new_password": "short"},
        expect=400,
    )

    # 改密成功
    c.post(
        "/me/password",
        json={"current_password": "test-pass-123456", "new_password": "fresh-pass-123456"},
        expect=204,
    )
    # 旧密码失效、新密码生效
    c.post("/auth/login", json={"username": username, "password": "test-pass-123456"}, expect=401)
    assert c.post(
        "/auth/login", json={"username": username, "password": "fresh-pass-123456"}, expect=200
    ).json()["token"]


def test_mfa_enable_login_disable(user):
    _, _, c = user
    username = c.get("/me", expect=200).json()["username"]

    assert c.get("/me/mfa", expect=200).json()["enabled"] is False

    # 注册 secret：重复调用幂等
    e1 = c.post("/me/mfa/enroll", expect=200).json()
    secret = e1["secret"]
    assert secret and e1["otpauth_url"].startswith("otpauth://totp/")
    e2 = c.post("/me/mfa/enroll", expect=200).json()
    assert e2["secret"] == secret

    # 错误验证码不能激活
    c.post("/me/mfa/activate", json={"code": "000000"}, expect=400)
    # 未激活时状态显示 pending（页面刷新后仍可继续）
    st = c.get("/me/mfa", expect=200).json()
    assert st["enabled"] is False and st["pending_secret"] == secret

    # 激活
    c.post("/me/mfa/activate", json={"code": _totp(secret)}, expect=204)
    assert c.get("/me/mfa", expect=200).json()["enabled"] is True
    c.post("/me/mfa/enroll", expect=409)

    # 登录两步验证
    login = c.post("/auth/login", json={"username": username, "password": "test-pass-123456"}, expect=200).json()
    assert login["mfa_required"] is True and login["mfa_token"] and "token" not in login
    mfa_token = login["mfa_token"]

    # 错误 code 401，且令牌保留（可重试）
    c.post("/auth/mfa-verify", json={"mfa_token": mfa_token, "code": "000000"}, expect=401)
    ok = c.post("/auth/mfa-verify", json={"mfa_token": mfa_token, "code": _totp(secret)}, expect=200).json()
    assert ok["token"] and ok["username"] == username
    # 一次性
    c.post("/auth/mfa-verify", json={"mfa_token": mfa_token, "code": _totp(secret)}, expect=401)
    # 伪造 token
    c.post("/auth/mfa-verify", json={"mfa_token": "bogus", "code": _totp(secret)}, expect=401)

    # 禁用需要密码+code
    c.post("/me/mfa/disable", json={"password": "wrong", "code": _totp(secret)}, expect=401)
    c.post("/me/mfa/disable", json={"password": "test-pass-123456", "code": "000000"}, expect=400)
    c.post("/me/mfa/disable", json={"password": "test-pass-123456", "code": _totp(secret)}, expect=204)
    assert c.get("/me/mfa", expect=200).json()["enabled"] is False
    # 禁用后登录无需验证码
    r = c.post("/auth/login", json={"username": username, "password": "test-pass-123456"}, expect=200).json()
    assert "token" in r and not r.get("mfa_required")


def test_mfa_does_not_affect_others(user_factory):
    u1, _, c1 = user_factory("pa")
    _, _, c2 = user_factory("pb")

    e = c1.post("/me/mfa/enroll", expect=200).json()
    c1.post("/me/mfa/activate", json={"code": _totp(e["secret"])}, expect=204)

    # c2 无 MFA，直接登录
    c2name = c2.get("/me", expect=200).json()["username"]
    r = c2.post("/auth/login", json={"username": c2name, "password": "test-pass-123456"}, expect=200).json()
    assert "token" in r
    # c2 不能看 c1 的 mfa 状态/操作
    c2.post("/me/mfa/enroll", expect=200)  # 自己独立的，可以


def test_mfa_endpoints_require_auth(anon):
    anon.get("/me", expect=401)
    anon.get("/me/mfa", expect=401)
    anon.post("/me/mfa/enroll", expect=401)
    anon.post("/me/password", json={}, expect=401)
