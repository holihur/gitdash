"""空仓库（尚无提交）浏览端点：必须优雅返回空结果，不能泄漏原始 git 报错。

回归 issue #9：创建空仓库后浏览会抛出
`git -C owner/repo.git ls-tree -l -z main: fatal: Not a valid object name main`。
（代码修复见 4868477；本文件在 API 层钉住该行为，防止回归。）
"""


def _owner(client) -> str:
    return client.get("/me", expect=200).json()["username"]


def test_empty_repo_tree_returns_empty(repo_factory):
    repo, client = repo_factory()
    owner = _owner(client)

    # 简写与 owner 限定路由；默认 ref（browseRef 回退默认分支）与显式 ref
    for url in (
        f"/repos/{repo}/tree",
        f"/repos/{repo}/tree?ref=main",
        f"/users/{owner}/repos/{repo}/tree",
    ):
        r = client.get(url)
        assert r.status_code == 200, r.text
        body = r.json()
        assert body["entries"] == [], body
        assert body.get("empty") is True


def test_empty_repo_blob_and_blame_not_found(repo_factory):
    repo, client = repo_factory()
    owner = _owner(client)
    for url in (
        f"/repos/{repo}/blob?path=a.txt",
        f"/repos/{repo}/blame?path=a.txt",
        f"/users/{owner}/repos/{repo}/blob?path=a.txt",
        f"/users/{owner}/repos/{repo}/blame?path=a.txt",
    ):
        assert client.get(url).status_code == 404, url


def test_empty_repo_lists_and_reads(repo_factory):
    repo, client = repo_factory()
    owner = _owner(client)
    # 列表类返回空数组
    assert client.get(f"/repos/{repo}/commits").json() == []
    assert client.get(f"/repos/{repo}/branches").json() == []
    assert client.get(f"/repos/{repo}/search?q=x").json() == []
    assert client.get(f"/repos/{repo}/releases").json() == []
    assert client.get(f"/users/{owner}/repos/{repo}/pulls").json() == []
    assert client.get(f"/repos/{repo}/issues").json() == []
    # 详情不 5xx
    assert client.get(f"/repos/{repo}").status_code == 200
    assert client.get(f"/users/{owner}/repos/{repo}").status_code == 200


def test_empty_repo_no_raw_git_errors(repo_factory):
    """所有空仓库浏览响应都不得包含原始 git 错误文本。"""
    repo, client = repo_factory()
    owner = _owner(client)
    paths = [
        f"/repos/{repo}",
        f"/repos/{repo}/tree",
        f"/repos/{repo}/tree?ref=main",
        f"/repos/{repo}/blob?path=a.txt",
        f"/repos/{repo}/commits",
        f"/repos/{repo}/branches",
        f"/repos/{repo}/blame?path=a.txt",
        f"/repos/{repo}/search?q=x",
        f"/repos/{repo}/releases",
        f"/users/{owner}/repos/{repo}/pulls",
        f"/repos/{repo}/issues",
        f"/users/{owner}/repos/{repo}/tree",
        f"/users/{owner}/repos/{repo}/commits",
    ]
    for path in paths:
        r = client.get(path)
        for needle in ("fatal:", "Not a valid object", "exit status"):
            assert needle not in r.text, f"{path} -> {r.status_code}: {r.text}"
