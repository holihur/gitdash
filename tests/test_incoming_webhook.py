"""入站 webhook（外部系统凭 token 创建 issue，支持多个）+ 出站 webhook 触发。"""

import time

import pytest


def _uuid() -> str:
    import uuid

    return uuid.uuid4().hex[:10]


def _r(owner, repo):
    return f"/users/{owner}/repos/{repo}"


def _create_hook(client, owner, repo, name="default"):
    """创建入站 webhook，返回 (id, token)。"""
    body = client.post(_r(owner, repo) + "/incoming-webhooks", json={"name": name}, expect=201).json()
    return body["webhook"]["id"], body["token"]


def _post_incoming(client, owner, repo, *, token=None, query_token=None, body=None):
    """直接发原始请求（ApiClient 固定了 Authorization 头，无法携带自定义头）。"""
    url = f"{client.base}/api/hooks/incoming/{owner}/{repo}"
    headers = {}
    if token:
        headers["X-Gitdash-Token"] = token
    if query_token:
        url += f"?token={query_token}"
    return client.session.post(url, headers=headers, json=body or {}, timeout=15)


@pytest.fixture
def env(user_factory):
    an, _, c = user_factory("inc")
    repo = f"inc-{_uuid()}"
    c.post("/repos", json={"name": repo}, expect=201)
    yield an, c, repo
    try:
        c.delete(f"/repos/{repo}", expect=204)
    except Exception:
        pass


def test_incoming_webhook_lifecycle(env, anon):
    an, c, repo = env

    # 初始为空
    listed = c.get(_r(an, repo) + "/incoming-webhooks", expect=200).json()
    assert listed["webhooks"] == []
    assert listed["path"] == f"/api/hooks/incoming/{an}/{repo}"

    # 创建 → 返回一次性 token 与调用路径
    hook_id, token = _create_hook(c, an, repo, "ci")
    assert len(token) == 64

    listed = c.get(_r(an, repo) + "/incoming-webhooks", expect=200).json()
    assert len(listed["webhooks"]) == 1
    assert listed["webhooks"][0]["name"] == "ci"
    assert "token" not in listed["webhooks"][0]

    # 无鉴权客户端携带 token 创建 issue
    r = _post_incoming(anon, an, repo, token=token, body={"title": "from ci", "body": "build failed"})
    assert r.status_code == 201, r.text
    issue = r.json()
    assert issue["title"] == "from ci" and issue["author"] == an

    issues = c.get(_r(an, repo) + "/issues", expect=200).json()
    assert any(i["title"] == "from ci" for i in issues)

    # 删除 → 旧 token 失效
    c.delete(_r(an, repo) + f"/incoming-webhooks/{hook_id}", expect=204)
    assert _post_incoming(anon, an, repo, token=token, body={"title": "nope"}).status_code == 401
    c.delete(_r(an, repo) + f"/incoming-webhooks/{hook_id}", expect=404)


def test_incoming_webhook_multiple_tokens(env, anon):
    """一个仓库可配置多个入站 token，各自独立有效，删除其一不影响其它。"""
    an, c, repo = env
    id1, token1 = _create_hook(c, an, repo, "ci")
    id2, token2 = _create_hook(c, an, repo, "alerts")

    listed = c.get(_r(an, repo) + "/incoming-webhooks", expect=200).json()
    assert {h["name"] for h in listed["webhooks"]} == {"ci", "alerts"}

    assert _post_incoming(anon, an, repo, token=token1, body={"title": "t1"}).status_code == 201
    assert _post_incoming(anon, an, repo, token=token2, body={"title": "t2"}).status_code == 201

    c.delete(_r(an, repo) + f"/incoming-webhooks/{id1}", expect=204)
    assert _post_incoming(anon, an, repo, token=token1, body={"title": "x"}).status_code == 401
    assert _post_incoming(anon, an, repo, token=token2, body={"title": "still works"}).status_code == 201


def test_incoming_webhook_disable_enable(env, anon):
    """每个入站 webhook 可单独禁用/启用；禁用后 token 立即失效，启用后恢复。"""
    an, c, repo = env
    hook_id, token = _create_hook(c, an, repo, "ci")

    c.patch(_r(an, repo) + f"/incoming-webhooks/{hook_id}", json={"enabled": False}, expect=200)
    listed = c.get(_r(an, repo) + "/incoming-webhooks", expect=200).json()
    assert listed["webhooks"][0]["enabled"] is False
    assert _post_incoming(anon, an, repo, token=token, body={"title": "x"}).status_code == 401

    c.patch(_r(an, repo) + f"/incoming-webhooks/{hook_id}", json={"enabled": True}, expect=200)
    assert _post_incoming(anon, an, repo, token=token, body={"title": "ok"}).status_code == 201

    # 未知 id / 缺 enabled
    c.patch(_r(an, repo) + "/incoming-webhooks/999999", json={"enabled": True}, expect=404)
    c.patch(_r(an, repo) + f"/incoming-webhooks/{hook_id}", json={}, expect=400)


def test_incoming_webhook_auth_and_token_transport(env, anon, user_factory):
    an, c, repo = env
    _, token = _create_hook(c, an, repo)

    # 错误 / 缺失 token
    assert _post_incoming(anon, an, repo, token="bad", body={"title": "x"}).status_code == 401
    assert _post_incoming(anon, an, repo, body={"title": "x"}).status_code == 401

    # query token 亦可用
    assert _post_incoming(anon, an, repo, query_token=token, body={"title": "via query"}).status_code == 201

    # 非 owner 不能管理
    _, _, bob = user_factory("bob")
    bob.get(_r(an, repo) + "/incoming-webhooks", expect=404)
    bob.post(_r(an, repo) + "/incoming-webhooks", json={"name": "x"}, expect=404)
    bob.delete(_r(an, repo) + "/incoming-webhooks/1", expect=404)


def test_incoming_webhook_validation(env, anon):
    an, c, repo = env
    _, token = _create_hook(c, an, repo)

    # 缺 title
    assert _post_incoming(anon, an, repo, token=token, body={"body": "x"}).status_code == 400

    # issue 功能关闭 → 403
    c.post(_r(an, repo) + "/issues-enabled", json={"has_issues": False}, expect=200)
    assert _post_incoming(anon, an, repo, token=token, body={"title": "x"}).status_code == 403
    c.post(_r(an, repo) + "/issues-enabled", json={"has_issues": True}, expect=200)


def _wait_delivery(client, owner, repo, hook_id, timeout=15.0):
    deadline = time.time() + timeout
    while time.time() < deadline:
        dl = client.get(_r(owner, repo) + f"/webhooks/{hook_id}/deliveries", expect=200).json()
        if dl:
            return dl[0]
        time.sleep(0.5)
    raise AssertionError(f"no delivery record for hook {hook_id}")


def test_incoming_webhook_triggers_outbound(env, anon):
    """出站能力：入站 webhook 创建 issue 后，仓库配置的出站 webhook 收到事件。"""
    an, c, repo = env
    # 127.0.0.1 属私网，投递会被 SSRF 防护拦截，但一定留下投递记录（可观测）
    hook = c.post(
        _r(an, repo) + "/webhooks",
        json={"url": "http://127.0.0.1:12345/hook"},
        expect=201,
    ).json()
    _, token = _create_hook(c, an, repo)
    r = _post_incoming(anon, an, repo, token=token, body={"title": "notify me"})
    assert r.status_code == 201, r.text

    d = _wait_delivery(c, an, repo, hook["id"])
    assert d["event"] == "issues"
    assert d["status"] in ("retry", "failed")
