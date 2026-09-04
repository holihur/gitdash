"""Webhook CRUD —— happy path + bad path（owner-only / 校验 / 隔离）。"""

import pytest


def _p(owner, repo, suffix=""):
    return f"/users/{owner}/repos/{repo}/webhooks{suffix}"


def _uuid() -> str:
    import uuid

    return uuid.uuid4().hex[:10]


@pytest.fixture
def wh_env(user_factory):
    an, _, alice = user_factory("alice")
    bn, _, bob = user_factory("bob")
    repo = f"wh-{_uuid()}"
    alice.post("/repos", json={"name": repo}, expect=201)
    yield an, bn, alice, bob, repo
    try:
        alice.delete(f"/repos/{repo}", expect=204)
    except Exception:
        pass


def test_webhook_add_list_delete(wh_env):
    an, _, alice, _, repo = wh_env

    w = alice.post(_p(an, repo), json={"url": "https://example.com/hook"}, expect=201).json()
    assert w["url"] == "https://example.com/hook" and w["id"] >= 1

    hooks = alice.get(_p(an, repo), expect=200).json()
    assert [h["url"] for h in hooks] == ["https://example.com/hook"]

    # 重复 URL → 409
    alice.post(_p(an, repo), json={"url": "https://example.com/hook"}, expect=409)
    # 同仓库不同 URL 可再加
    alice.post(_p(an, repo), json={"url": "https://other.example/h"}, expect=201)

    alice.delete(_p(an, repo, f"/{w['id']}"), expect=204)
    assert len(alice.get(_p(an, repo), expect=200).json()) == 1
    alice.delete(_p(an, repo, f"/{w['id']}"), expect=404)


def test_webhook_invalid_url(wh_env):
    an, _, alice, _, repo = wh_env
    for bad in ["", "ftp://x", "not a url", "javascript:alert(1)", "http://"]:
        alice.post(_p(an, repo), json={"url": bad}, expect=400)


def test_webhook_owner_only_and_auth(wh_env):
    an, bn, alice, bob, repo = wh_env

    # 非 owner / 协作者不能管理
    bob.get(_p(an, repo), expect=404)
    bob.post(_p(an, repo), json={"url": "https://example.com/hook"}, expect=404)
    bob.delete(_p(an, repo, "/1"), expect=404)

    alice.post(
        f"/users/{an}/repos/{repo}/collabs", json={"username": bn, "permission": "write"}, expect=200
    )
    bob.get(_p(an, repo), expect=404)  # write 协作者也不行

    # 仓库不存在
    alice.get(_p(an, "nope"), expect=404)


def test_webhook_requires_auth(anon, wh_env):
    an, _, _, _, repo = wh_env
    anon.get(_p(an, repo), expect=401)
    anon.post(_p(an, repo), json={"url": "https://example.com"}, expect=401)


def test_webhook_repo_delete_cascades(wh_env):
    an, _, alice, _, repo = wh_env
    alice.post(_p(an, repo), json={"url": "https://example.com/hook"}, expect=201)
    alice.delete(f"/repos/{repo}", expect=204)
    # 重建同名仓库后 webhook 不残留
    alice.post("/repos", json={"name": repo}, expect=201)
    assert alice.get(_p(an, repo), expect=200).json() == []
    alice.delete(f"/repos/{repo}", expect=204)
