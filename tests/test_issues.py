"""Issue API —— happy path + bad path（含多用户隔离 / 删除级联）。"""

import pytest


def _list_issues(client, repo) -> list[dict]:
    return client.get(f"/repos/{repo}/issues", expect=200).json()


# ---- happy path ----

def test_issue_create_list_close_reopen(repo_factory):
    repo, client = repo_factory()

    assert _list_issues(client, repo) == []

    r1 = client.post(
        "/repos/%s/issues" % repo,
        json={"title": "crash on login", "body": "steps to reproduce"},
        expect=201,
    ).json()
    assert r1["number"] == 1
    assert r1["state"] == "open"
    assert r1["closed_at"] is None
    assert "created_at" in r1 and "author" in r1

    r2 = client.post(
        "/repos/%s/issues" % repo, json={"title": "feature: dark mode"}, expect=201
    ).json()
    assert r2["number"] == 2  # 同仓库内自动递增

    # 列表：open 在前、number 倒序
    issues = _list_issues(client, repo)
    assert [i["number"] for i in issues] == [2, 1]
    assert all(i["state"] == "open" for i in issues)

    # close -> closed_at 置位；列表排序变化
    closed = client.patch(
        "/repos/%s/issues/1" % repo, json={"state": "closed"}, expect=200
    ).json()
    assert closed["state"] == "closed"
    assert closed["closed_at"] is not None

    issues = _list_issues(client, repo)
    assert [i["state"] for i in issues] == ["open", "closed"]

    # reopen -> closed_at 清空
    reopened = client.patch(
        "/repos/%s/issues/1" % repo, json={"state": "open"}, expect=200
    ).json()
    assert reopened["state"] == "open"
    assert reopened["closed_at"] is None


def test_issue_created_without_body(repo_factory):
    repo, client = repo_factory()
    issue = client.post(
        "/repos/%s/issues" % repo, json={"title": "no body here"}, expect=201
    ).json()
    assert issue["body"] == ""
    assert issue["number"] == 1


def test_issue_repo_recreated_after_delete_has_no_stale(repo_factory, user_factory):
    repo, client = repo_factory()
    client.post("/repos/%s/issues" % repo, json={"title": "old bug"}, expect=201)
    client.delete(f"/repos/{repo}", expect=204)
    # 同名重建：级联删除后无残留 issue
    client.post("/repos", json={"name": repo}, expect=201)
    assert _list_issues(client, repo) == []
    client.delete(f"/repos/{repo}", expect=204)


# ---- bad path ----

def test_issue_empty_or_whitespace_title(repo_factory):
    repo, client = repo_factory()
    for title in ["", "   "]:
        client.post("/repos/%s/issues" % repo, json={"title": title}, expect=400)


def test_issue_title_too_long(repo_factory):
    repo, client = repo_factory()
    client.post("/repos/%s/issues" % repo, json={"title": "x" * 201}, expect=400)


def test_issue_body_too_long(repo_factory):
    repo, client = repo_factory()
    client.post(
        "/repos/%s/issues" % repo, json={"title": "ok", "body": "y" * 10001}, expect=400
    )


def test_issue_invalid_state(repo_factory):
    repo, client = repo_factory()
    client.post("/repos/%s/issues" % repo, json={"title": "t"}, expect=201)
    for state in ["banana", "", "OPEN", "closed "]:
        client.patch(
            "/repos/%s/issues/1" % repo, json={"state": state}, expect=400
        )


def test_issue_patch_missing_number(repo_factory):
    repo, client = repo_factory()
    client.patch(
        "/repos/%s/issues/999" % repo, json={"state": "closed"}, expect=404
    )


def test_issue_patch_invalid_number(repo_factory):
    repo, client = repo_factory()
    client.patch(
        "/repos/%s/issues/abc" % repo, json={"state": "closed"}, expect=400
    )


def test_issue_on_missing_repo(user_factory):
    _, _, client = user_factory()
    missing = f"nope-{uuid4hex()}"
    client.get(f"/repos/{missing}/issues", expect=404)
    client.post(f"/repos/{missing}/issues", json={"title": "t"}, expect=404)
    client.patch(f"/repos/{missing}/issues/1", json={"state": "closed"}, expect=404)


def test_issue_user_isolation(user_factory):
    _, _, alice = user_factory("alice")
    _, _, bob = user_factory("bob")
    repo = f"iso-{uuid4hex()}"

    alice.post("/repos", json={"name": repo}, expect=201)
    alice.post("/repos/%s/issues" % repo, json={"title": "alice only"}, expect=201)

    # bob 对 alice 仓库的 issue 全 404（仓库隔离优先）
    bob.get(f"/repos/{repo}/issues", expect=404)
    bob.post(f"/repos/{repo}/issues", json={"title": "x"}, expect=404)
    bob.patch(f"/repos/{repo}/issues/1", json={"state": "closed"}, expect=404)


def test_issue_requires_auth(anon, repo_factory):
    repo, _ = repo_factory()
    anon.get(f"/repos/{repo}/issues", expect=401)
    anon.post(f"/repos/{repo}/issues", json={"title": "x"}, expect=401)
    anon.patch(f"/repos/{repo}/issues/1", json={"state": "closed"}, expect=401)


def uuid4hex() -> str:
    import uuid

    return uuid.uuid4().hex[:10]
