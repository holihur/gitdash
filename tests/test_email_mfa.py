"""Email MFA（邮箱验证码两步验证）—— 黑盒 happy path + bad path。

两类用例：
- bad path（无需 SMTP）：SMTP 未配置时的拒绝、未认证访问、伪造 mfa_token。
- happy path（需本地 SMTP sink）：以环境变量 GITDASH_SMTP_HOST / GITDASH_SMTP_PORT
  指向本文件启动的极简 SMTP sink（仅本模块启动，端口须与 GITDASH_SMTP_PORT 一致），
  从"收到的邮件"里解析验证码与邮箱验证链接，走完绑定 → 激活 → 登录 → 关闭全流程。
  未设置这两个环境变量时 happy path 自动跳过。
"""

from __future__ import annotations

import os
import re
import socket
import threading
import time
import uuid

import pytest


def _uuid() -> str:
    return uuid.uuid4().hex[:10]


# ---- bad path（默认环境：未配置 SMTP）----


@pytest.fixture
def user(user_factory):
    return user_factory("em")


def test_email_mfa_enroll_requires_smtp(user):
    if os.environ.get("GITDASH_SMTP_HOST", "").strip():
        pytest.skip("SMTP configured; bad-path case applies only without SMTP")
    _, _, c = user
    r = c.post("/me/mfa/email/enroll", expect=400).json()
    assert r["code"] == "smtp_not_configured"


def test_email_mfa_endpoints_require_auth(anon):
    anon.post("/me/mfa/email/enroll", expect=401)
    anon.post("/me/mfa/email/activate", json={"code": "123456"}, expect=401)
    anon.post("/me/mfa/email/send", expect=401)


def test_mfa_email_resend_bogus_token(client_factory):
    c = client_factory()
    r = c.post("/auth/mfa-email/resend", json={"mfa_token": "bogus-" + _uuid()}, expect=401).json()
    assert r["code"] == "mfa_challenge_expired"


# ---- 极简 SMTP sink（只收 DATA，不校验）----

class SMTPSink:
    def __init__(self, port: int):
        self.messages: list[dict] = []
        self._sock = socket.socket()
        self._sock.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        self._sock.bind(("127.0.0.1", port))
        self._sock.listen(8)
        threading.Thread(target=self._serve, daemon=True).start()

    def _serve(self):
        while True:
            try:
                conn, _ = self._sock.accept()
            except OSError:
                return
            try:
                self._handle(conn)
            except Exception:
                pass
            finally:
                conn.close()

    def _handle(self, conn):
        f = conn.makefile("rb")
        conn.sendall(b"220 sink\r\n")
        data_mode, body = False, []
        while True:
            line = f.readline()
            if not line:
                return
            if data_mode:
                if line.strip() == b".":
                    self.messages.append(_parse_message("\n".join(body)))
                    conn.sendall(b"250 ok\r\n")
                    data_mode, body = False, []
                else:
                    body.append(line.decode("utf-8", "replace").rstrip("\r\n"))
            else:
                cmd = line.strip().upper()
                if cmd.startswith(b"DATA"):
                    conn.sendall(b"354 go\r\n")
                    data_mode = True
                elif cmd.startswith(b"QUIT"):
                    conn.sendall(b"221 bye\r\n")
                    return
                else:
                    conn.sendall(b"250 ok\r\n")


def _parse_message(raw: str) -> dict:
    raw = raw.replace("\r\n", "\n")
    head, _, text = raw.partition("\n\n")
    text = text.strip()
    subject = ""
    for ln in head.splitlines():
        if ln.lower().startswith("subject:"):
            subject = ln.split(":", 1)[1].strip()
    return {"subject": subject, "body": text}


def _wait_for_mail(sink: SMTPSink, subject_part: str, timeout: float = 10.0) -> dict:
    deadline = time.time() + timeout
    while time.time() < deadline:
        for m in reversed(sink.messages):  # 重发场景：取最新一封同主题邮件
            if subject_part in m["subject"]:
                return m
        time.sleep(0.2)
    raise AssertionError(f"no mail with subject ~ {subject_part!r}")


@pytest.fixture(scope="module")
def smtp_sink():
    host = os.environ.get("GITDASH_SMTP_HOST", "").strip()
    port = os.environ.get("GITDASH_SMTP_PORT", "").strip()
    if not host or not port:
        pytest.skip("set GITDASH_SMTP_HOST/PORT to run email MFA happy path tests")
    sink = SMTPSink(int(port))
    yield sink


# ---- happy path ----


def test_email_mfa_full_flow(base_url, client_factory, smtp_sink):
    username = f"em-{_uuid()}"
    email = f"{username}@example.com"
    c = client_factory()
    c.post("/auth/register", json={"username": username, "password": "em-pass-123456"}, expect=201)

    # 设置邮箱 → 从 sink 收验证邮件 → 提取 token 完成验证
    c.post("/me/profile", json={"email": email}, expect=200)
    mail = _wait_for_mail(smtp_sink, "verify your email")
    token = re.search(r"verify_email=([A-Za-z0-9]+)", mail["body"]).group(1)
    c.post("/me/email/verify", json={"token": token}, expect=200)

    # 绑定：发送激活码
    assert c.post("/me/mfa/email/enroll", expect=200).json()["sent"] is True
    code = re.search(r"\b(\d{6})\b", _wait_for_mail(smtp_sink, "enable email two-factor")["body"]).group(1)
    c.post("/me/mfa/email/activate", json={"code": "000000"}, expect=400)
    c.post("/me/mfa/email/activate", json={"code": code}, expect=204)
    c.post("/me/mfa/email/enroll", expect=409)
    m = c.get("/me/mfa", expect=200).json()
    assert m["enabled"] is True and m["method"] == "email"

    # 登录：进入邮箱验证码二次验证
    r = c.post("/auth/login", json={"username": username, "password": "em-pass-123456"}, expect=200).json()
    assert r["mfa_required"] is True and r["mfa_method"] == "email"
    tok = r["mfa_token"]
    c.post("/auth/mfa-verify", json={"mfa_token": tok, "code": "000000"}, expect=401)

    # 重发：新码有效、旧码作废
    assert c.post("/auth/mfa-email/resend", json={"mfa_token": tok}, expect=200).json()["sent"] is True
    code2 = re.search(r"\b(\d{6})\b", _wait_for_mail(smtp_sink, "sign-in verification")["body"]).group(1)
    c.post("/auth/mfa-verify", json={"mfa_token": tok, "code": code}, expect=401)
    ok = c.post("/auth/mfa-verify", json={"mfa_token": tok, "code": code2}, expect=200).json()
    assert ok["token"]
    # mfa_token 一次性
    c.post("/auth/mfa-verify", json={"mfa_token": tok, "code": code2}, expect=401)

    # 关闭：密码 + sink 下发的验证码
    c.post("/me/mfa/email/send", expect=204)
    dcode = re.search(r"\b(\d{6})\b", _wait_for_mail(smtp_sink, "disable email two-factor")["body"]).group(1)
    c.post("/me/mfa/disable", json={"password": "wrong", "code": dcode}, expect=401)
    c.post("/me/mfa/disable", json={"password": "em-pass-123456", "code": "000000"}, expect=400)
    c.post("/me/mfa/disable", json={"password": "em-pass-123456", "code": dcode}, expect=204)
    assert c.get("/me/mfa", expect=200).json()["enabled"] is False
    r2 = c.post("/auth/login", json={"username": username, "password": "em-pass-123456"}, expect=200).json()
    assert r2.get("token") and not r2.get("mfa_required")


def test_email_mfa_requires_verified_email(base_url, client_factory, smtp_sink):
    c = client_factory()
    username = f"emv-{_uuid()}"
    c.post("/auth/register", json={"username": username, "password": "em-pass-123456"}, expect=201)
    # 未设置邮箱
    c.post("/me/mfa/email/enroll", expect=400)
    # 邮箱已设置但未验证
    c.post("/me/profile", json={"email": f"{username}@example.com"}, expect=200)
    r = c.post("/me/mfa/email/enroll", expect=400).json()
    assert r["code"] == "email_not_verified"
