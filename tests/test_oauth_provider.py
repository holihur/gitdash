"""OAuth 2.0 provider（gitdash 作为授权服务器）黑盒测试。

覆盖：应用注册/列表/密钥重置/删除、授权码流程（consent → code → token → 调 API）、
设备流（device_code → 轮询 → 浏览器确认 → token）、以及安全边界（redirect_uri 校验、
错误 client_secret、code 重放、非法 scope、撤销授权）。

浏览器侧（/login/oauth/*）需要 `gitdash_session` cookie：register 响应会种下该 cookie，
因此这里用 `requests.Session` 直接请求（非 /api 路径不经 ApiClient 的 /api 前缀）。
"""
from __future__ import annotations

import uuid
from urllib.parse import parse_qs, urlparse

import requests

FIRST_PARTY_CLIENT_ID = "gitdash-cli"


def _session_user(base_url: str) -> tuple[str, requests.Session]:
    """注册一个用户，返回 (username, 带 gitdash_session cookie 的 session)。"""
    s = requests.Session()
    username = f"oauth-{uuid.uuid4().hex[:10]}"
    r = s.post(
        f"{base_url}/api/auth/register",
        json={"username": username, "password": "test-pass-123456"},
        timeout=15,
    )
    assert r.status_code == 201, r.text
    return username, s


def _create_app(s: requests.Session, base_url: str, *, callback: str = "https://app.example/cb") -> dict:
    r = s.post(
        f"{base_url}/api/applications",
        json={
            "name": f"App {uuid.uuid4().hex[:6]}",
            "homepage": "https://app.example",
            "description": "test app",
            "callback_url": callback,
        },
        timeout=15,
    )
    assert r.status_code == 201, r.text
    app = r.json()
    assert app["client_id"] and app["client_secret"], app
    assert app["callback_url"] == callback
    return app


def _authorize_code(s: requests.Session, base_url: str, app: dict, scope: str = "repo", state: str = "xyz") -> str:
    params = {
        "client_id": app["client_id"],
        "redirect_uri": app["callback_url"],
        "scope": scope,
        "state": state,
        "response_type": "code",
    }
    resp = s.post(
        f"{base_url}/login/oauth/authorize",
        data={**params, "action": "approve"},
        allow_redirects=False,
        timeout=15,
    )
    assert resp.status_code == 302, (resp.status_code, resp.text)
    q = parse_qs(urlparse(resp.headers["location"]).query)
    assert q.get("state") == [state], q
    assert q.get("code"), q
    return q["code"][0]


def _exchange(base_url: str, app: dict, code: str, *, secret: str | None = None) -> requests.Response:
    return requests.post(
        f"{base_url}/login/oauth/access_token",
        data={
            "grant_type": "authorization_code",
            "client_id": app["client_id"],
            "client_secret": app["client_secret"] if secret is None else secret,
            "code": code,
            "redirect_uri": app["callback_url"],
        },
        timeout=15,
    )


def _poll_device(base_url: str, device_code: str) -> requests.Response:
    return requests.post(
        f"{base_url}/login/oauth/access_token",
        data={
            "grant_type": "urn:ietf:params:oauth:grant-type:device_code",
            "client_id": FIRST_PARTY_CLIENT_ID,
            "device_code": device_code,
        },
        timeout=15,
    )


# ---- 应用管理 ----

def test_oauth_app_crud_and_secret_reset(base_url):
    _, s = _session_user(base_url)
    app = _create_app(s, base_url)

    lst = s.get(f"{base_url}/api/applications", timeout=15)
    assert lst.status_code == 200
    apps = lst.json()
    assert any(a["id"] == app["id"] for a in apps)
    # 列表不得泄漏 client_secret
    assert all("client_secret" not in a for a in apps)

    rs = s.post(f"{base_url}/api/applications/{app['id']}/reset_secret", timeout=15)
    assert rs.status_code == 200
    new_secret = rs.json()["client_secret"]
    assert new_secret and new_secret != app["client_secret"]

    de = s.delete(f"{base_url}/api/applications/{app['id']}", timeout=15)
    assert de.status_code == 204
    assert not any(
        a["id"] == app["id"] for a in s.get(f"{base_url}/api/applications", timeout=15).json()
    )


def test_oauth_app_create_validation(base_url):
    _, s = _session_user(base_url)
    # 非 http(s) 回调地址
    bad = s.post(
        f"{base_url}/api/applications",
        json={"name": "x", "callback_url": "ftp://nope"},
        timeout=15,
    )
    assert bad.status_code == 400
    # 缺 name
    noname = s.post(
        f"{base_url}/api/applications",
        json={"callback_url": "https://app.example/cb"},
        timeout=15,
    )
    assert noname.status_code == 400


# ---- 授权码流程 ----

def test_oauth_authorization_code_flow(base_url):
    _, s = _session_user(base_url)
    app = _create_app(s, base_url)

    params = {
        "client_id": app["client_id"],
        "redirect_uri": app["callback_url"],
        "scope": "repo",
        "state": "xyz",
        "response_type": "code",
    }
    # 已登录 → consent 页
    page = s.get(f"{base_url}/login/oauth/authorize", params=params, timeout=15)
    assert page.status_code == 200
    assert "Authorize" in page.text

    code = _authorize_code(s, base_url, app)

    tok = _exchange(base_url, app, code)
    assert tok.status_code == 200, tok.text
    body = tok.json()
    assert body["token_type"] == "bearer"
    assert body["scope"] == "repo"
    access = body["access_token"]

    # token 可访问受保护 API
    r = requests.get(
        f"{base_url}/api/repos",
        headers={"Authorization": f"Bearer {access}"},
        timeout=15,
    )
    assert r.status_code == 200

    # 授权列表可见，撤销后 token 立即失效
    auths = s.get(f"{base_url}/api/applications/authorizations", timeout=15).json()
    mine = [a for a in auths if a["app_id"] == app["id"]]
    assert mine, auths
    assert s.delete(
        f"{base_url}/api/applications/authorizations/{mine[0]['id']}", timeout=15
    ).status_code == 204
    assert requests.get(
        f"{base_url}/api/repos",
        headers={"Authorization": f"Bearer {access}"},
        timeout=15,
    ).status_code == 401


def test_oauth_deny_redirects_with_access_denied(base_url):
    _, s = _session_user(base_url)
    app = _create_app(s, base_url)
    resp = s.post(
        f"{base_url}/login/oauth/authorize",
        data={
            "client_id": app["client_id"],
            "redirect_uri": app["callback_url"],
            "scope": "repo",
            "state": "s1",
            "response_type": "code",
            "action": "deny",
        },
        allow_redirects=False,
        timeout=15,
    )
    assert resp.status_code == 302
    q = parse_qs(urlparse(resp.headers["location"]).query)
    assert q.get("error") == ["access_denied"]
    assert q.get("state") == ["s1"]


def test_oauth_scope_and_input_validation(base_url):
    _, s = _session_user(base_url)
    app = _create_app(s, base_url)

    base_params = {
        "client_id": app["client_id"],
        "redirect_uri": app["callback_url"],
        "response_type": "code",
    }
    # 非法 scope → 回跳带 invalid_scope
    r = s.get(
        f"{base_url}/login/oauth/authorize",
        params={**base_params, "scope": "repo,admin"},
        allow_redirects=False,
        timeout=15,
    )
    assert r.status_code == 302
    assert "error=invalid_scope" in r.headers["location"]

    # response_type 非 code → unsupported_response_type（回跳到已注册 callback）
    r = s.get(
        f"{base_url}/login/oauth/authorize",
        params={
            "client_id": app["client_id"],
            "redirect_uri": app["callback_url"],
            "response_type": "token",
        },
        allow_redirects=False,
        timeout=15,
    )
    assert r.status_code == 302
    assert "error=unsupported_response_type" in r.headers["location"]

    # redirect_uri 不匹配 → 400（绝不回跳到未注册地址）
    r = s.get(
        f"{base_url}/login/oauth/authorize",
        params={
            "client_id": app["client_id"],
            "redirect_uri": "https://evil.example/steal",
            "response_type": "code",
        },
        allow_redirects=False,
        timeout=15,
    )
    assert r.status_code == 400

    # 未知 client_id → 400
    r = s.get(
        f"{base_url}/login/oauth/authorize",
        params={
            "client_id": "does-not-exist",
            "redirect_uri": "https://app.example/cb",
            "response_type": "code",
        },
        allow_redirects=False,
        timeout=15,
    )
    assert r.status_code == 400


def test_oauth_token_security(base_url):
    _, s = _session_user(base_url)
    app = _create_app(s, base_url)
    code = _authorize_code(s, base_url, app)

    # 错误 client_secret
    assert _exchange(base_url, app, code, secret="wrong").status_code == 401

    # 正确兑换后，code 重放失败
    assert _exchange(base_url, app, code).status_code == 200
    replay = _exchange(base_url, app, code)
    assert replay.status_code == 400
    assert "invalid_grant" in replay.text

    # 未知 code
    assert _exchange(base_url, app, "0" * 64).status_code == 400


def test_oauth_delete_app_revokes_tokens(base_url):
    _, s = _session_user(base_url)
    app = _create_app(s, base_url)
    code = _authorize_code(s, base_url, app)
    access = _exchange(base_url, app, code).json()["access_token"]
    assert requests.get(
        f"{base_url}/api/repos",
        headers={"Authorization": f"Bearer {access}"},
        timeout=15,
    ).status_code == 200

    assert s.delete(f"{base_url}/api/applications/{app['id']}", timeout=15).status_code == 204
    assert requests.get(
        f"{base_url}/api/repos",
        headers={"Authorization": f"Bearer {access}"},
        timeout=15,
    ).status_code == 401


# ---- 设备流（CLI） ----

def test_oauth_device_flow(base_url):
    _, s = _session_user(base_url)

    dc = requests.post(
        f"{base_url}/login/oauth/device/code",
        data={"client_id": FIRST_PARTY_CLIENT_ID, "scope": "repo"},
        timeout=15,
    )
    assert dc.status_code == 200, dc.text
    dc = dc.json()
    assert dc["device_code"] and dc["user_code"]
    assert dc["interval"] >= 1 and dc["expires_in"] > 0

    # 未确认 → authorization_pending
    pending = _poll_device(base_url, dc["device_code"])
    assert pending.status_code == 400
    assert "authorization_pending" in pending.text

    # 浏览器确认（带 session cookie）
    approve = s.post(
        f"{base_url}/login/oauth/device",
        data={"user_code": dc["user_code"], "action": "approve"},
        timeout=15,
    )
    assert approve.status_code == 200

    # 轮询取 token
    tok = _poll_device(base_url, dc["device_code"])
    assert tok.status_code == 200, tok.text
    body = tok.json()
    assert body["token_type"] == "bearer" and body["scope"] == "repo"
    assert requests.get(
        f"{base_url}/api/repos",
        headers={"Authorization": f"Bearer {body['access_token']}"},
        timeout=15,
    ).status_code == 200

    # 设备码一次性
    assert _poll_device(base_url, dc["device_code"]).status_code == 400


def test_oauth_device_deny(base_url):
    _, s = _session_user(base_url)
    dc = requests.post(
        f"{base_url}/login/oauth/device/code",
        data={"client_id": FIRST_PARTY_CLIENT_ID},
        timeout=15,
    ).json()

    deny = s.post(
        f"{base_url}/login/oauth/device",
        data={"user_code": dc["user_code"], "action": "deny"},
        timeout=15,
    )
    assert deny.status_code == 200

    denied = _poll_device(base_url, dc["device_code"])
    assert denied.status_code == 400
    assert "access_denied" in denied.text


def test_oauth_device_code_unknown_client(base_url):
    r = requests.post(
        f"{base_url}/login/oauth/device/code",
        data={"client_id": "not-the-cli"},
        timeout=15,
    )
    assert r.status_code == 400
