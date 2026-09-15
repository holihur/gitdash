"""出站 webhook：事件订阅 + 多事件类型（分支/标签、Release），全部经队列异步投递。"""

import time

import pytest


def _uuid() -> str:
    import uuid

    return uuid.uuid4().hex[:10]


def _r(owner, repo):
    return f"/users/{owner}/repos/{repo}"


def _hooks(owner, repo):
    return f"/users/{owner}/repos/{repo}/webhooks"


@pytest.fixture
def env(user_factory):
    an, _, c = user_factory("evt")
    repo = f"evt-{_uuid()}"
    c.post("/repos", json={"name": repo}, expect=201)
    c.post(
        _r(an, repo) + "/commits",
        json={"message": "init", "changes": [{"path": "a.txt", "action": "create", "content": "1"}]},
        expect=201,
    )
    yield an, c, repo
    try:
        c.delete(f"/repos/{repo}", expect=204)
    except Exception:
        pass


def _wait_delivery(client, owner, repo, hook_id, timeout=15.0):
    deadline = time.time() + timeout
    while time.time() < deadline:
        dl = client.get(_r(owner, repo) + f"/webhooks/{hook_id}/deliveries", expect=200).json()
        if dl:
            return dl[0]
        time.sleep(0.4)
    raise AssertionError(f"no delivery record for hook {hook_id}")


# 私网地址会被 SSRF 拦截，但一定会落一条投递记录，便于确定性断言事件已入队。
_BLOCKED = "http://127.0.0.1:12345/hook"


def test_list_webhook_events(env):
    _, c, _ = env
    events = c.get("/webhook-events", expect=200).json()
    for e in ("push", "issues", "pulls", "comment", "create", "delete", "release", "pipeline"):
        assert e in events


def test_webhook_event_subscription(env):
    an, c, repo = env
    w = c.post(
        _hooks(an, repo),
        json={"url": _BLOCKED, "events": ["issues"]},
        expect=201,
    ).json()
    assert w["events"] == ["issues"]

    # 无订阅 = 全部
    w2 = c.post(
        _hooks(an, repo),
        json={"url": "https://example.com/hook2"},
        expect=201,
    ).json()
    assert w2["events"] == []

    # 未知事件 → 400
    c.post(
        _hooks(an, repo),
        json={"url": "https://example.com/bad", "events": ["nope"]},
        expect=400,
    )

    # 创建 issue → 订阅 issues 的 hook 收到投递
    c.post(_r(an, repo) + "/issues", json={"title": "hello"}, expect=201)
    d = _wait_delivery(c, an, repo, w["id"])
    assert d["event"] == "issues"


def test_branch_and_tag_events(env):
    an, c, repo = env
    hook = c.post(
        _hooks(an, repo),
        json={"url": _BLOCKED, "events": ["create", "delete"]},
        expect=201,
    ).json()

    # 创建分支 → create 事件
    c.post(_r(an, repo) + "/refs", json={"type": "branch", "name": "feature", "from": "main"}, expect=201)
    assert _wait_delivery(c, an, repo, hook["id"])["event"] == "create"

    # 删除分支 → delete 事件
    c.delete(_r(an, repo) + "/refs/branch/feature", expect=204)
    deadline = time.time() + 15
    dl = []
    while time.time() < deadline:
        dl = c.get(_r(an, repo) + f"/webhooks/{hook['id']}/deliveries", expect=200).json()
        if any(x["event"] == "delete" for x in dl):
            break
        time.sleep(0.4)
    assert any(x["event"] == "delete" for x in dl)


def test_release_event(env):
    an, c, repo = env
    hook = c.post(
        _hooks(an, repo),
        json={"url": _BLOCKED, "events": ["release"]},
        expect=201,
    ).json()
    c.post(_r(an, repo) + "/refs", json={"type": "tag", "name": "v1.0", "from": "main"}, expect=201)
    c.post(_r(an, repo) + "/releases", json={"tag_name": "v1.0", "name": "First"}, expect=201)
    assert _wait_delivery(c, an, repo, hook["id"])["event"] == "release"


def test_subscription_filters_other_events(env):
    """订阅 issues 的 hook 不应收到 create 事件（出站按订阅过滤）。"""
    an, c, repo = env
    issues_hook = c.post(
        _hooks(an, repo),
        json={"url": "http://127.0.0.1:12346/issues", "events": ["issues"]},
        expect=201,
    ).json()
    create_hook = c.post(
        _hooks(an, repo),
        json={"url": "http://127.0.0.1:12347/create", "events": ["create"]},
        expect=201,
    ).json()

    c.post(_r(an, repo) + "/refs", json={"type": "branch", "name": "topic", "from": "main"}, expect=201)
    # create hook 一定收到
    assert _wait_delivery(c, an, repo, create_hook["id"])["event"] == "create"
    # 再等一会儿，issues hook 不应有记录
    time.sleep(2)
    assert c.get(_r(an, repo) + f"/webhooks/{issues_hook['id']}/deliveries", expect=200).json() == []
