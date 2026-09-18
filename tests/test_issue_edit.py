"""Issue 编辑（标题 / 正文 / 状态）与删除 —— 黑盒测试。"""


def _mk_issue(client, repo, title="first issue", body="original body"):
    return client.post(
        f"/repos/{repo}/issues", json={"title": title, "body": body}, expect=201
    ).json()


def test_issue_edit_title_body_and_state(repo_factory):
    repo, client = repo_factory()
    it = _mk_issue(client, repo)
    n = it["number"]

    # 只改标题 → 正文保持不变
    r = client.patch(f"/repos/{repo}/issues/{n}", json={"title": "renamed"}, expect=200).json()
    assert r["title"] == "renamed"
    assert r["body"] == "original body"

    # 只改正文 → 标题保持不变
    r = client.patch(f"/repos/{repo}/issues/{n}", json={"body": "new body"}, expect=200).json()
    assert r["title"] == "renamed"
    assert r["body"] == "new body"

    # 标题 + 正文 + 状态一起改
    r = client.patch(
        f"/repos/{repo}/issues/{n}",
        json={"title": "final", "body": "done", "state": "closed"},
        expect=200,
    ).json()
    assert r["title"] == "final"
    assert r["body"] == "done"
    assert r["state"] == "closed"
    assert r["closed_at"] is not None

    # 列表反映最新值
    lst = client.get(f"/repos/{repo}/issues", expect=200).json()
    assert lst[0]["title"] == "final"
    assert lst[0]["state"] == "closed"


def test_issue_edit_validation(repo_factory):
    repo, client = repo_factory()
    n = _mk_issue(client, repo)["number"]

    assert client.patch(f"/repos/{repo}/issues/{n}", json={}).status_code == 400
    assert client.patch(f"/repos/{repo}/issues/{n}", json={"title": "   "}).status_code == 400
    assert client.patch(f"/repos/{repo}/issues/{n}", json={"state": "bogus"}).status_code == 400
    assert client.patch(f"/repos/{repo}/issues/{n}", json={"title": "x" * 201}).status_code == 400
    assert client.patch(f"/repos/{repo}/issues/{n}", json={"body": "x" * 10001}).status_code == 400
    # 不存在的 issue
    assert client.patch(f"/repos/{repo}/issues/9999", json={"title": "x"}).status_code == 404


def test_issue_delete(repo_factory):
    repo, client = repo_factory()
    n = _mk_issue(client, repo)["number"]

    client.post(f"/repos/{repo}/issues/{n}/comments", json={"body": "a comment"}, expect=201)

    assert client.delete(f"/repos/{repo}/issues/{n}").status_code == 204
    # 再删 → 404
    assert client.delete(f"/repos/{repo}/issues/{n}").status_code == 404
    # 列表不再包含，且评论被级联清理
    assert client.get(f"/repos/{repo}/issues", expect=200).json() == []
    assert client.get(f"/repos/{repo}/issues/{n}/comments", expect=200).json() == []


def test_issue_delete_owner_qualified_route(repo_factory):
    """owner 限定删除路由（与简写路由同一 handler，但路径解析不同）。"""
    repo, client = repo_factory()
    me = client.get("/me", expect=200).json()["username"]
    n = _mk_issue(client, repo)["number"]
    assert client.delete(f"/users/{me}/repos/{repo}/issues/{n}").status_code == 204
    assert client.delete(f"/repos/{repo}/issues/{n}").status_code == 404


def test_issue_edit_delete_permissions(repo_factory, user_factory):
    repo, client = repo_factory()
    n = _mk_issue(client, repo)["number"]

    # 无权限用户：编辑/删除都返回 404（隐藏仓库与 issue 的存在性）
    _, _, outsider = user_factory("outsider")
    assert outsider.patch(f"/repos/{repo}/issues/{n}", json={"title": "hack"}).status_code == 404
    assert outsider.delete(f"/repos/{repo}/issues/{n}").status_code == 404
    # 原 issue 未受影响
    assert client.get(f"/repos/{repo}/issues", expect=200).json()[0]["number"] == n
