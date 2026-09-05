"""Watch / Inbox —— 关注仓库 + 收件箱通知（happy path + bad path）。

PR 通知用“同 sha 分支 + fast-forward 合并”的 HTTP-only 方式构造（无需 git/ssh）。
"""

import uuid

import pytest


def _p(owner, repo, suffix=""):
    return f"/users/{owner}/repos/{repo}{suffix}"


def _uuid() -> str:
    return uuid.uuid4().hex[:10]


@pytest.fixture
def watch_env(user_factory):
    """alice 建公开仓库（带 README 提交，创建者自动关注），bob 可 watch。"""
    an, _, alice = user_factory("alice")
    bn, _, bob = user_factory("bob")
    repo = f"proj-{_uuid()}"
    alice.post("/repos", json={"name": repo, "template": "readme"}, expect=201)
    alice.post(_p(an, repo, "/visibility"), json={"private": False}, expect=200)
    yield an, bn, alice, bob, repo
    try:
        alice.delete(f"/repos/{repo}", expect=204)
    except Exception:
        pass


def test_watch_unwatch_and_count(watch_env):
    an, bn, alice, bob, repo = watch_env

    # 创建者自动 watch（watchers=1）；bob 初始未 watch
    r = alice.get(_p(an, repo), expect=200).json()
    assert r["watchers"] == 1 and r["watching"] is True
    r = bob.get(_p(an, repo), expect=200).json()
    assert r["watching"] is False and r["watchers"] == 1

    # bob watch
    s = bob.put(_p(an, repo, "/watch"), expect=200).json()
    assert s["watching"] is True and s["watchers"] == 2
    # 幂等
    s = bob.put(_p(an, repo, "/watch"), expect=200).json()
    assert s["watchers"] == 2
    # 详情携带状态
    r = bob.get(_p(an, repo), expect=200).json()
    assert r["watching"] is True and r["watchers"] == 2

    # watched 列表
    watched = [(r["owner"], r["name"]) for r in bob.get("/watched", expect=200).json()]
    assert (an, repo) in watched

    # unwatch
    s = bob.delete(_p(an, repo, "/watch"), expect=200).json()
    assert s["watching"] is False and s["watchers"] == 1
    watched = [(r["owner"], r["name"]) for r in bob.get("/watched", expect=200).json()]
    assert (an, repo) not in watched


def test_watch_private_repo_forbidden(user_factory):
    an, _, alice = user_factory("alice")
    _, _, bob = user_factory("bob")
    repo = f"secret-{_uuid()}"
    alice.post("/repos", json={"name": repo}, expect=201)
    bob.put(_p(an, repo, "/watch"), expect=404)
    bob.delete(_p(an, repo, "/watch"), expect=404)
    alice.delete(f"/repos/{repo}", expect=204)


def test_inbox_issue_notifications(watch_env):
    an, bn, alice, bob, repo = watch_env
    bob.put(_p(an, repo, "/watch"), expect=200)
    # bob 需要写权限才能开/关 issue
    alice.post(_p(an, repo, "/collabs"), json={"username": bn, "permission": "write"}, expect=200)

    # alice 开 issue → 通知 bob；actor 不通知自己
    alice.post(_p(an, repo, "/issues"), json={"title": "t1"}, expect=201)
    items = bob.get("/inbox", expect=200).json()
    assert len(items) == 1
    n = items[0]
    assert n["kind"] == "issue" and n["action"] == "opened" and n["number"] == 1
    assert n["actor"] == an and n["repo"] == repo and n["read"] is False
    assert bob.get("/inbox/unread", expect=200).json()["count"] == 1
    assert alice.get("/inbox", expect=200).json() == []

    # alice 取消关注后仍收 owner 级通知
    alice.delete(_p(an, repo, "/watch"), expect=200)

    # bob 开 issue → alice（owner）收到
    bob.post(_p(an, repo, "/issues"), json={"title": "t2"}, expect=201)
    a_items = alice.get("/inbox", expect=200).json()
    assert len(a_items) == 1 and a_items[0]["actor"] == bn and a_items[0]["action"] == "opened"

    # alice 关闭 #2 → bob 收到 closed；重复关闭不重复通知
    alice.patch(_p(an, repo, "/issues/2"), json={"state": "closed"}, expect=200)
    items = bob.get("/inbox", expect=200).json()
    assert len(items) == 2 and items[0]["action"] == "closed" and items[0]["number"] == 2
    alice.patch(_p(an, repo, "/issues/2"), json={"state": "closed"}, expect=200)
    assert len(bob.get("/inbox", expect=200).json()) == 2

    # bob 重新打开 #2 → alice 收到 reopened
    bob.patch(_p(an, repo, "/issues/2"), json={"state": "open"}, expect=200)
    a_items = alice.get("/inbox", expect=200).json()
    assert len(a_items) == 2 and a_items[0]["action"] == "reopened"

    # 已读管理
    nid = a_items[0]["id"]
    alice.post(f"/inbox/read/{nid}", expect=200)
    assert alice.get("/inbox/unread", expect=200).json()["count"] == 1
    alice.post("/inbox/read", expect=200)  # 全部已读
    assert alice.get("/inbox/unread", expect=200).json()["count"] == 0
    assert all(i["read"] for i in alice.get("/inbox", expect=200).json())
    # 越权 / 不存在
    bob.post(f"/inbox/read/{nid}", expect=404)
    alice.post("/inbox/read/999999", expect=404)
    # 删除
    alice.delete(f"/inbox/{nid}", expect=204)
    assert len(alice.get("/inbox", expect=200).json()) == 1
    alice.delete("/inbox/999999", expect=404)


def _open_pr(client, owner, repo, num, branch):
    """基于同 sha 分支快速构造 PR（HTTP-only，无需 git）。"""
    client.post(_p(owner, repo, "/refs"), json={"type": "branch", "name": branch, "from": "main"}, expect=201)
    client.post(_p(owner, repo, "/pulls"), json={
        "title": f"pr-{num}", "body": "b",
        "source_branch": branch, "target_branch": "main",
    }, expect=201)


def test_inbox_pull_notifications(watch_env):
    an, bn, alice, bob, repo = watch_env
    bob.put(_p(an, repo, "/watch"), expect=200)

    _open_pr(alice, an, repo, 1, "feat-1")
    items = bob.get("/inbox", expect=200).json()
    assert len(items) == 1 and items[0]["kind"] == "pull" and items[0]["action"] == "opened"
    assert items[0]["number"] == 1 and items[0]["actor"] == an

    # 关闭 → 重新打开 → 合并：每个状态各通知一次
    alice.post(_p(an, repo, "/pulls/1/state"), json={"state": "closed"}, expect=200)
    alice.post(_p(an, repo, "/pulls/1/state"), json={"state": "open"}, expect=200)
    m = alice.post(_p(an, repo, "/pulls/1/merge"), json={}, expect=200).json()
    assert m["state"] == "merged"

    actions = [i["action"] for i in bob.get("/inbox", expect=200).json()]
    assert actions == ["merged", "reopened", "closed", "opened"]  # 新 → 旧
    # actor（alice）不通知自己
    assert alice.get("/inbox", expect=200).json() == []


def test_inbox_requires_auth(anon, watch_env):
    an, _, _, _, repo = watch_env
    anon.get("/inbox", expect=401)
    anon.get("/inbox/unread", expect=401)
    anon.post("/inbox/read", expect=401)
    anon.post("/inbox/read/1", expect=401)
    anon.delete("/inbox/1", expect=401)
    anon.put(_p(an, repo, "/watch"), expect=401)
    anon.get("/watched", expect=401)
