"""Issue 搜索（关键词 / 状态过滤）与置顶 —— 黑盒测试。"""
from __future__ import annotations

import uuid


def _setup(user_factory):
    username, _token, client = user_factory("iss")
    repo = f"r-{uuid.uuid4().hex[:8]}"
    client.post("/repos", json={"name": repo}, expect=201)
    client.post(
        f"/repos/{repo}/issues",
        json={"title": "login crash", "body": "steps to reproduce"},
        expect=201,
    )
    client.post(
        f"/repos/{repo}/issues",
        json={"title": "dark mode", "body": "feature request"},
        expect=201,
    )
    client.post(
        f"/repos/{repo}/issues",
        json={"title": "docs", "body": "update readme"},
        expect=201,
    )
    return username, client, repo


def _numbers(client, repo, qs=""):
    url = f"/repos/{repo}/issues" + (f"?{qs}" if qs else "")
    return [i["number"] for i in client.get(url, expect=200).json()]


def test_issue_search_by_title_body_author(user_factory):
    username, client, repo = _setup(user_factory)

    assert _numbers(client, repo, "q=crash") == [1]  # 标题
    assert _numbers(client, repo, "q=feature") == [2]  # 正文
    assert _numbers(client, repo, f"q={username}") == [3, 2, 1]  # 作者
    assert _numbers(client, repo, "q=no-such-text") == []
    # 列表首字母大小写不敏感
    assert _numbers(client, repo, "q=CRASH") == [1]


def test_issue_search_state_filter(user_factory):
    _username, client, repo = _setup(user_factory)
    client.patch(f"/repos/{repo}/issues/2", json={"state": "closed"}, expect=200)

    assert _numbers(client, repo, "state=closed") == [2]
    assert _numbers(client, repo, "state=open") == [3, 1]
    assert _numbers(client, repo) == [3, 1, 2]  # 无过滤 = 全部
    assert client.get(f"/repos/{repo}/issues?state=bogus").status_code == 400

    # 关键词 + 状态组合
    assert _numbers(client, repo, "q=docs&state=open") == [3]
    assert _numbers(client, repo, "q=docs&state=closed") == []


def test_issue_pin_unpin(user_factory):
    _username, client, repo = _setup(user_factory)

    r = client.patch(f"/repos/{repo}/issues/1", json={"pinned": True}, expect=200).json()
    assert r["pinned"] is True

    # 置顶排最前，其余保持 open/编号倒序
    assert _numbers(client, repo) == [1, 3, 2]

    r = client.patch(f"/repos/{repo}/issues/1", json={"pinned": False}, expect=200).json()
    assert r["pinned"] is False
    assert _numbers(client, repo) == [3, 2, 1]


def test_issue_pin_permissions(user_factory):
    _username, client, repo = _setup(user_factory)
    _, _, outsider = user_factory("outsider")
    assert outsider.patch(f"/repos/{repo}/issues/1", json={"pinned": True}).status_code == 404
    # 未受影响
    assert client.get(f"/repos/{repo}/issues", expect=200).json()[0]["pinned"] is False
