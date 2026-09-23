"""Issue 用户体验增强黑盒测试：负责人 / 标签筛选与计数 / 排序 / 订阅 /
时间线 / 评论编辑 / 关闭附评论与原因 / 通知参与者（作者与 @提及）。"""

from __future__ import annotations

import uuid

import pytest


def _uuid() -> str:
    return uuid.uuid4().hex[:10]


def _p(owner, repo, suffix=""):
    return f"/users/{owner}/repos/{repo}{suffix}"


def _create_issue(c, owner, repo, title="bug", body="body"):
    return c.post(_p(owner, repo, "/issues"), json={"title": title, "body": body}, expect=201).json()


def _label_id(c, owner, repo, name="bug"):
    for l in c.get(_p(owner, repo, "/labels"), expect=200).json():
        if l["name"] == name:
            return l["id"]
    raise AssertionError(f"label {name} not found")


@pytest.fixture
def issue_env(user_factory):
    an, _, alice = user_factory("iu")
    bn, _, bob = user_factory("iu")
    repo = f"iu-{_uuid()}"
    alice.post("/repos", json={"name": repo}, expect=201)
    alice.post(_p(an, repo, "/collabs"), json={"username": bn, "permission": "write"}, expect=200)
    yield an, alice, bn, bob, repo
    try:
        alice.delete(f"/repos/{repo}", expect=204)
    except Exception:
        pass


def test_assignees_set_and_filter(issue_env):
    an, alice, bn, bob, repo = issue_env
    _create_issue(alice, an, repo, "one")
    _create_issue(alice, an, repo, "two")

    got = alice.put(
        _p(an, repo, "/issues/1/assignees"), json={"assignees": [bn]}, expect=200
    ).json()
    assert got["assignees"] == [bn]

    # 按负责人过滤（owner 视角用用户名；bob 用 me）
    q = alice.get(_p(an, repo, "/issues") + "?assignee=" + bn, expect=200).json()
    assert [i["number"] for i in q] == [1]
    q = bob.get(_p(an, repo, "/issues") + "?assignee=me", expect=200).json()
    assert [i["number"] for i in q] == [1]
    q = alice.get(_p(an, repo, "/issues") + "?assignee=none", expect=200).json()
    assert [i["number"] for i in q] == [2]

    # 清空负责人
    got = alice.put(_p(an, repo, "/issues/1/assignees"), json={"assignees": []}, expect=200).json()
    assert got["assignees"] == []


def test_label_filter_counts_and_sort(issue_env):
    an, alice, _, _, repo = issue_env
    for title in ("a", "b", "c"):
        _create_issue(alice, an, repo, title)
    lid = _label_id(alice, an, repo)
    alice.post(_p(an, repo, "/issues/1/labels"), json={"label_ids": [lid]}, expect=200)

    by_label = alice.get(_p(an, repo, "/issues") + f"?label={lid}", expect=200).json()
    assert [i["number"] for i in by_label] == [1]

    counts = alice.get(_p(an, repo, "/issues/counts") + f"?label={lid}", expect=200).json()
    assert counts == {"open": 1, "closed": 0}
    counts_all = alice.get(_p(an, repo, "/issues/counts"), expect=200).json()
    assert counts_all == {"open": 3, "closed": 0}

    oldest = alice.get(_p(an, repo, "/issues") + "?sort=oldest", expect=200).json()
    assert oldest[0]["number"] == 1
    newest = alice.get(_p(an, repo, "/issues") + "?sort=newest", expect=200).json()
    assert newest[0]["number"] == 3
    # 非法排序 → 400
    alice.get(_p(an, repo, "/issues") + "?sort=bogus", expect=400)


def test_subscribe_events_close_with_comment(issue_env):
    an, alice, bn, bob, repo = issue_env
    _create_issue(alice, an, repo, "needs work")

    # bob 订阅
    assert bob.post(_p(an, repo, "/issues/1/subscribe"), expect=200).json()["subscribed"] is True
    detail = bob.get(_p(an, repo, "/issues/1"), expect=200).json()
    assert detail["subscribed"] is True
    bob.delete(_p(an, repo, "/issues/1/subscribe"), expect=200)
    assert bob.get(_p(an, repo, "/issues/1"), expect=200).json()["subscribed"] is False

    # 关闭并附评论 + 关闭原因
    closed = alice.patch(
        _p(an, repo, "/issues/1"),
        json={"state": "closed", "comment": "done, thanks", "state_reason": "completed"},
        expect=200,
    ).json()
    assert closed["state"] == "closed"
    assert closed["state_reason"] == "completed"
    assert closed["comment_count"] == 1

    # 时间线包含 opened / closed / commented
    events = alice.get(_p(an, repo, "/issues/1/events"), expect=200).json()
    actions = [e["action"] for e in events]
    assert "opened" in actions and "closed" in actions and "commented" in actions

    # 非法关闭原因
    alice.patch(
        _p(an, repo, "/issues/1"), json={"state": "closed", "state_reason": "bogus"}, expect=400
    )


def test_comment_edit_permission(issue_env):
    an, alice, bn, bob, repo = issue_env
    _create_issue(alice, an, repo, "discuss")
    c = alice.post(_p(an, repo, "/issues/1/comments"), json={"body": "first"}, expect=201).json()

    edited = alice.patch(_p(an, repo, f"/comments/{c['id']}"), json={"body": "second"}, expect=200).json()
    assert edited["body"] == "second"

    # 非作者不能编辑
    bob.patch(_p(an, repo, f"/comments/{c['id']}"), json={"body": "hijack"}, expect=403)


def test_author_and_mention_notifications(issue_env, user_factory):
    an, alice, bn, bob, repo = issue_env
    # bob（协作者）开 issue；alice 评论后 bob 应收到通知（参与者），无需 watch
    it = _create_issue(bob, an, repo, "bob's issue")
    num = it["number"]
    alice.post(_p(an, repo, f"/issues/{num}/comments"), json={"body": "ping"}, expect=201)

    bob_inbox = bob.get("/inbox", expect=200).json()
    assert any(n["number"] == num and n["kind"] == "issue" for n in bob_inbox)

    # @提及非参与者也应收到通知
    cn, _, carol = user_factory("carol")
    alice.post(
        _p(an, repo, f"/issues/{num}/comments"),
        json={"body": f"@{cn} please look"},
        expect=201,
    )
    carol_inbox = carol.get("/inbox", expect=200).json()
    assert any(n["number"] == num and n["kind"] == "issue" for n in carol_inbox)


def test_linked_pulls_field(issue_env):
    an, alice, _, _, repo = issue_env
    _create_issue(alice, an, repo, "refs #5", "see #5")
    detail = alice.get(_p(an, repo, "/issues/1"), expect=200).json()
    assert isinstance(detail["linked_pulls"], list)
