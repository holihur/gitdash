"""仓库杂项端点黑盒测试：简写路由（/repos/{name}/...）、blame/search、gc、
revert、template。

这些端点此前未被黑盒套件覆盖（见 scripts/route-coverage.py 报告），是最容易
藏 bug 的盲区。
"""

import time
import uuid

import pytest


def _uuid() -> str:
    return uuid.uuid4().hex[:10]


def _search_wait(client, url, params, timeout=60):
    """单仓库搜索最终一致：索引构建中（X-Code-Search: indexing）时轮询等待。"""
    deadline = time.time() + timeout
    while True:
        resp = client.get(url, params=params, expect=200)
        if resp.headers.get("X-Code-Search") != "indexing" or time.time() >= deadline:
            return resp
        time.sleep(0.3)


@pytest.fixture
def repo_env(user_factory):
    """owner + 公开仓库 + 一个提交（hello.txt）。"""
    uname, _, c = user_factory("rm")
    repo = f"rm-{_uuid()}"
    c.post("/repos", json={"name": repo, "private": False}, expect=201)
    c.post(
        f"/users/{uname}/repos/{repo}/commits",
        json={
            "message": "add hello",
            "changes": [{"path": "hello.txt", "action": "create", "content": "hello world\n"}],
        },
        expect=201,
    )
    yield uname, c, repo
    try:
        c.delete(f"/repos/{repo}", expect=204)
    except Exception:
        pass


def test_shorthand_releases(repo_env):
    uname, c, repo = repo_env
    c.post(
        f"/users/{uname}/repos/{repo}/refs",
        json={"type": "tag", "name": "v1", "from": "main"},
        expect=201,
    )
    c.post(f"/repos/{repo}/releases", json={"tag_name": "v1", "name": "R"}, expect=201)
    assert [x["tag_name"] for x in c.get(f"/repos/{repo}/releases", expect=200).json()] == ["v1"]
    assert c.get(f"/repos/{repo}/releases/v1", expect=200).json()["tag_name"] == "v1"
    c.delete(f"/repos/{repo}/releases/v1", expect=204)
    c.get(f"/repos/{repo}/releases/v1", expect=404)


def test_shorthand_blame_and_search(repo_env):
    uname, c, repo = repo_env
    blame = c.get(f"/repos/{repo}/blame?ref=main&path=hello.txt", expect=200).json()
    assert blame["path"] == "hello.txt"
    assert any("hello world" in line["content"] for line in blame["lines"])

    hits = _search_wait(c, f"/repos/{repo}/search", {"q": "hello", "ref": "main"}).json()
    assert any(h["path"] == "hello.txt" for h in hits)

    # 参数校验
    c.get(f"/repos/{repo}/search", expect=400)


def test_commit_detail(repo_env):
    """单个提交元数据（blame 深链的跳转目标）。"""
    uname, c, repo = repo_env
    commits = c.get(f"/users/{uname}/repos/{repo}/commits?ref=main", expect=200).json()
    sha = commits[0]["sha"]

    detail = c.get(f"/users/{uname}/repos/{repo}/commits/{sha}", expect=200).json()
    assert detail["sha"] == sha
    assert detail["author"]
    assert detail["date"]
    assert detail["message"] == "add hello"

    # 简写路由
    assert c.get(f"/repos/{repo}/commits/{sha}", expect=200).json()["sha"] == sha

    # 非法 sha → 400 invalid_sha
    assert (
        c.get(f"/users/{uname}/repos/{repo}/commits/not-a-sha", expect=400).json()["code"]
        == "invalid_sha"
    )
    # 格式合法但不存在的 sha → 400
    c.get(f"/users/{uname}/repos/{repo}/commits/{'0' * 40}", expect=400)


def test_gc_both_routes(repo_env, user_factory):
    uname, c, repo = repo_env
    res = c.post(f"/repos/{repo}/gc", expect=200).json()
    assert {"before_bytes", "after_bytes", "freed_bytes"} <= set(res)
    res2 = c.post(f"/users/{uname}/repos/{repo}/gc", expect=200).json()
    assert res2["before_bytes"] >= 0

    # 非所有者不可 gc（requireOwner 对他人返回 404，避免泄露仓库存在性）
    _, _, other = user_factory("rmx")
    other.post(f"/repos/{repo}/gc", expect=404)


def test_revert_commit(repo_env):
    uname, c, repo = repo_env
    commits = c.get(f"/repos/{repo}/commits?ref=main", expect=200).json()
    sha = commits[0]["sha"]

    r = c.post(
        f"/users/{uname}/repos/{repo}/commits/{sha}/revert",
        json={"branch": "main", "message": "revert it"},
        expect=201,
    ).json()
    assert r["branch"] == "main" and r["sha"] and r["sha"] != sha

    # 非法 sha / 缺 branch
    c.post(f"/users/{uname}/repos/{repo}/commits/deadbeef/revert", json={"branch": "main"}, expect=400)
    c.post(f"/users/{uname}/repos/{repo}/commits/{sha}/revert", json={}, expect=400)


def test_repo_template_toggle(repo_env):
    uname, c, repo = repo_env
    r = c.post(f"/users/{uname}/repos/{repo}/template", json={"is_template": True}, expect=200).json()
    assert r["is_template"] is True
    tmpls = c.get("/templates", expect=200).json()
    assert any(t["name"] == repo for t in tmpls)

    c.post(f"/users/{uname}/repos/{repo}/template", json={}, expect=400)
    r = c.post(f"/users/{uname}/repos/{repo}/template", json={"is_template": False}, expect=200).json()
    assert r["is_template"] is False
