"""PAT（个人访问令牌）黑盒测试。"""
from __future__ import annotations

import uuid

import pytest


@pytest.fixture
def pat_user(user_factory, client_factory):
    """注册一个用户并创建一个全 scope PAT，返回 (username, pat_token, pat_client, session_client)。"""
    username, token, client = user_factory()
    resp = client.post(
        "/tokens",
        json={"name": "full", "scopes": ["repo", "inbox", "keys"]},
        expect=201,
    )
    body = resp.json()
    pat_token = body["token"]
    return username, pat_token, client_factory(pat_token), client


def test_pat_create_list_delete(user_factory, client_factory):
    username, token, client = user_factory()
    resp = client.post(
        "/tokens",
        json={"name": "ci", "scopes": ["repo", "keys"]},
        expect=201,
    )
    body = resp.json()
    assert isinstance(body.get("token"), str) and len(body["token"]) == 64
    assert body["name"] == "ci"
    assert body["scopes"] == ["repo", "keys"]
    pat_id = body["id"]
    assert body.get("last_used_at") == ""

    listed = client.get("/tokens", expect=200).json()
    assert len(listed) == 1
    row = listed[0]
    assert row["id"] == pat_id
    assert row["name"] == "ci"
    assert row["scopes"] == ["repo", "keys"]
    assert "token" not in row  # 明文 token 绝不回传

    client.delete(f"/tokens/{pat_id}", expect=204)
    assert client.get("/tokens", expect=200).json() == []
    client.delete(f"/tokens/{pat_id}", expect=404)


def test_pat_default_scopes_repo(user_factory):
    _, _, client = user_factory()
    body = client.post("/tokens", json={"name": "d"}, expect=201).json()
    assert body["scopes"] == ["repo"]


def test_pat_calls_api(pat_user, client_factory):
    username, pat_token, pat_client, session_client = pat_user
    me = pat_client.get("/me", expect=200).json()
    assert me["username"] == username

    repo = f"pr-{uuid.uuid4().hex[:8]}"
    pat_client.post("/repos", json={"name": repo}, expect=201)
    repos = pat_client.get("/repos", expect=200).json()
    assert any(r["name"] == repo for r in repos)


def test_pat_scope_enforcement(user_factory, client_factory):
    username, token, client = user_factory()
    body = client.post("/tokens", json={"name": "inbox-only", "scopes": ["inbox"]}, expect=201).json()
    pat = client_factory(body["token"])

    r = pat.get("/repos")
    assert r.status_code == 403
    assert r.json().get("code") == "insufficient_scope"

    pat.get("/inbox", expect=200)

    # admin：PAT 一律 403
    r = pat.post("/admin/login", json={"username": "a", "password": "b"})
    assert r.status_code == 403

    # 无 repo scope 不能建仓库
    r = pat.post("/repos", json={"name": f"nr-{uuid.uuid4().hex[:8]}"})
    assert r.status_code == 403


def test_pat_cannot_escalate_scopes(user_factory, client_factory):
    """安全审计 A1：窄作用域 PAT 不能铸造全权 PAT，也不能碰凭据管理端点。"""
    _, _, client = user_factory()
    body = client.post(
        "/tokens", json={"name": "narrow", "scopes": ["inbox"]}, expect=201
    ).json()
    pat = client_factory(body["token"])

    # 不能读/写 PAT
    r = pat.get("/tokens")
    assert r.status_code == 403, r.text
    r = pat.post("/tokens", json={"name": "wide", "scopes": ["repo", "inbox", "keys"]})
    assert r.status_code == 403, r.text
    assert r.json().get("code") == "insufficient_scope"
    # 不能删除自己的 PAT
    r = pat.delete(f"/tokens/{body['id']}")
    assert r.status_code == 403, r.text

    # 不能注册 passkey / OAuth 应用 / BYOK
    r = pat.post("/me/passkeys/register/begin")
    assert r.status_code == 403, r.text
    r = pat.post("/applications", json={"name": "app", "redirect_uris": ["https://x"]})
    assert r.status_code == 403, r.text
    r = pat.post("/me/byok", json={"name": "k", "provider": "anthropic", "api_key": "sk-x"})
    assert r.status_code == 403, r.text

    # 原 PAT 仍然有效
    pat.get("/inbox", expect=200)


def test_pat_invalid_after_delete(user_factory, client_factory):
    _, _, client = user_factory()
    body = client.post("/tokens", json={"name": "gone"}, expect=201).json()
    pat = client_factory(body["token"])
    pat.get("/me", expect=200)
    client.delete(f"/tokens/{body['id']}", expect=204)
    r = pat.get("/me")
    assert r.status_code == 401


def test_pat_bad_scopes(user_factory):
    _, _, client = user_factory()
    r = client.post("/tokens", json={"name": "x", "scopes": ["admin"]})
    assert r.status_code == 400


def test_pat_empty_name(user_factory):
    _, _, client = user_factory()
    r = client.post("/tokens", json={"name": "  "})
    assert r.status_code == 400
