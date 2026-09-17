"""入站邮件（reply-by-email）黑盒测试。

邮件回复通过签名 token 实现无状态路由：出站通知邮件的 Reply-To 为
`reply+<payload>.<sig>@<domain>`，本用例直接按同一算法构造 token 并投递到
`POST /api/mail/inbound`，断言评论落库、正文清洗与鉴权隔离。
"""

from __future__ import annotations

import base64
import hashlib
import hmac
import os
import time
import uuid

import pytest

MAIL_SECRET = os.environ.get("GITDASH_MAIL_SECRET", "test-mail-secret")
INBOUND_SECRET = os.environ.get("GITDASH_MAIL_INBOUND_SECRET", "test-mail-inbound-secret")
REPLY_DOMAIN = os.environ.get("GITDASH_MAIL_REPLY_DOMAIN", "gitdash.test")

pytestmark = pytest.mark.skipif(
    bool(os.environ.get("GITDASH_API_URL")) and not os.environ.get("GITDASH_MAIL_SECRET"),
    reason="external instance: set GITDASH_MAIL_SECRET to run mail inbound tests",
)


def _b64(raw: bytes) -> str:
    return base64.urlsafe_b64encode(raw).rstrip(b"=").decode()


def reply_token(owner: str, repo: str, kind: str, number: int, username: str, *, ttl: int = 3600) -> str:
    payload = "|".join([owner, repo, kind, str(number), username, str(int(time.time()) + ttl)])
    body = _b64(payload.encode())
    sig = _b64(hmac.new(MAIL_SECRET.encode(), body.encode(), hashlib.sha256).digest()[:10])
    return f"{body}.{sig}"


def reply_to(owner: str, repo: str, kind: str, number: int, username: str, **kw) -> str:
    return f"reply+{reply_token(owner, repo, kind, number, username, **kw)}@{REPLY_DOMAIN}"


@pytest.fixture
def mail_env(user_factory):
    """owner + 公开仓库（含 main），并创建一个 issue。"""
    owner, _, client = user_factory("mail")
    repo = f"m-{uuid.uuid4().hex[:8]}"
    client.post("/repos", json={"name": repo, "template": "readme"}, expect=201)
    client.post(f"/users/{owner}/repos/{repo}/visibility", json={"private": False}, expect=200)
    client.post(f"/users/{owner}/repos/{repo}/issues", json={"title": "hello"}, expect=201)
    yield owner, repo, client
    try:
        client.delete(f"/repos/{repo}", expect=204)
    except Exception:
        pass


def _inbound(anon, to, text="reply body", *, secret=INBOUND_SECRET, expect=201, **extra):
    payload = {"to": to, "from": "user@example.com", "subject": "Re: [x]", "text": text}
    payload.update(extra)
    return anon.post(
        f"/mail/inbound?secret={secret}",
        json=payload,
        expect=expect,
    )


def test_reply_creates_comment(mail_env, anon):
    owner, repo, client = mail_env
    to = reply_to(owner, repo, "issue", 1, owner)
    r = _inbound(anon, to, "Looks good to me!").json()
    assert r["author"] == owner and r["number"] == 1
    assert r["body"] == "Looks good to me!"

    comments = client.get(f"/users/{owner}/repos/{repo}/issues/1/comments", expect=200).json()
    assert len(comments) == 1 and comments[0]["body"] == "Looks good to me!"
    assert comments[0]["author"] == owner


def test_reply_body_cleanup(mail_env, anon):
    owner, repo, client = mail_env
    to = reply_to(owner, repo, "issue", 1, owner)
    text = (
        "Top reply line\n"
        "second line\n"
        "\n"
        "> quoted old text\n"
        "> more quote\n"
        "On Mon, Jan 1 2024, someone wrote:\n"
        "> still quoted\n"
        "-- \n"
        "My Signature\n"
    )
    _inbound(anon, to, text)
    comments = client.get(f"/users/{owner}/repos/{repo}/issues/1/comments", expect=200).json()
    body = comments[-1]["body"]
    assert body == "Top reply line\nsecond line"
    assert "quoted" not in body and "Signature" not in body


def test_reply_to_pull(mail_env, anon):
    owner, repo, client = mail_env
    client.post(
        f"/users/{owner}/repos/{repo}/refs",
        json={"type": "branch", "name": "feat", "from": "main"},
        expect=201,
    )
    client.post(
        f"/users/{owner}/repos/{repo}/pulls",
        json={"title": "pr", "body": "b", "source_branch": "feat", "target_branch": "main"},
        expect=201,
    )
    to = reply_to(owner, repo, "pull", 1, owner)
    r = _inbound(anon, to, "LGTM via email").json()
    assert r["number"] == 1

    comments = client.get(f"/users/{owner}/repos/{repo}/pulls/1/comments", expect=200).json()
    assert any(c["body"] == "LGTM via email" for c in comments)


def test_display_name_address_is_parsed(mail_env, anon):
    owner, repo, _ = mail_env
    to = f"gitdash <{reply_to(owner, repo, 'issue', 1, owner)}>"
    r = _inbound(anon, to).json()
    assert r["number"] == 1


def test_invalid_signature_rejected(mail_env, anon):
    owner, repo, _ = mail_env
    token = reply_token(owner, repo, "issue", 1, owner)
    body, _sig = token.rsplit(".", 1)
    forged = f"{body}.AAAAAAAAAAAAAA"
    _inbound(anon, f"reply+{forged}@{REPLY_DOMAIN}", expect=400)


def test_expired_token_rejected(mail_env, anon):
    owner, repo, _ = mail_env
    to = reply_to(owner, repo, "issue", 1, owner, ttl=-60)
    _inbound(anon, to, expect=400)


def test_wrong_inbound_secret_rejected(mail_env, anon):
    owner, repo, _ = mail_env
    to = reply_to(owner, repo, "issue", 1, owner)
    _inbound(anon, to, secret="not-the-secret", expect=401)


def test_header_secret_accepted(mail_env, anon):
    owner, repo, _ = mail_env
    to = reply_to(owner, repo, "issue", 1, owner)
    resp = anon.session.post(
        f"{anon.base}/api/mail/inbound",
        headers={"X-Gitdash-Mail-Secret": INBOUND_SECRET},
        json={"to": to, "from": "user@example.com", "subject": "Re: [x]", "text": "via header"},
        timeout=15,
    )
    assert resp.status_code == 201, resp.text


def test_reply_message_id_idempotent_and_threaded(mail_env, anon):
    owner, repo, client = mail_env
    to = reply_to(owner, repo, "issue", 1, owner)
    extra = {
        "message_id": "<reply-1@example.com>",
        "in_reply_to": "<notify-0@example.com>",
    }
    first = _inbound(anon, to, "first delivery", **extra).json()
    assert first["message_id"] == "<reply-1@example.com>"
    assert first["in_reply_to"] == "<notify-0@example.com>"

    # 同一封邮件重复投递（MTA 重试/回放）不应重复落库
    again = _inbound(anon, to, "first delivery", expect=200, **extra).json()
    assert again["id"] == first["id"]
    comments = client.get(f"/users/{owner}/repos/{repo}/issues/1/comments", expect=200).json()
    assert len(comments) == 1


def test_reply_references_fallback(mail_env, anon):
    owner, repo, _ = mail_env
    to = reply_to(owner, repo, "issue", 1, owner)
    r = _inbound(
        anon, to, "via references",
        message_id="<reply-2@example.com>",
        references="<a@x> <b@x> <orig@x>",
    ).json()
    assert r["in_reply_to"] == "<orig@x>"


def test_web_comment_has_message_id(mail_env):
    owner, repo, client = mail_env
    c = client.post(
        f"/users/{owner}/repos/{repo}/issues/1/comments",
        json={"body": "from web"}, expect=201,
    ).json()
    assert c["message_id"]


def test_bad_to_address_rejected(mail_env, anon):
    _, _, _ = mail_env
    _inbound(anon, "not-a-reply@example.com", expect=400)
    _inbound(anon, f"reply+garbage@{REPLY_DOMAIN}", expect=400)


def test_empty_body_after_cleanup_rejected(mail_env, anon):
    owner, repo, _ = mail_env
    to = reply_to(owner, repo, "issue", 1, owner)
    _inbound(anon, to, "> only quoted\n-- \nsig", expect=400)


def test_unknown_host_returns_404(mail_env, anon):
    owner, _, _ = mail_env
    to = reply_to(owner, "no-such-repo", "issue", 1, owner)
    _inbound(anon, to, expect=404)
