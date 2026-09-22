"""Pages 静态网站托管：仓库级开关（默认关闭）+ 站点服务与访问控制。"""

import requests


def _commit(client, owner: str, repo: str, branch: str, path: str, content: str) -> None:
    client.post(
        f"/users/{owner}/repos/{repo}/commits",
        json={
            "branch": branch,
            "message": f"add {path}",
            "changes": [{"path": path, "action": "create", "content": content}],
        },
        expect=201,
    )


def test_pages_disabled_by_default(user_factory, base_url):
    owner, _, c = user_factory("pg")
    repo = "site"
    c.post("/repos", json={"name": repo, "private": False}, expect=201)

    cfg = c.get(f"/users/{owner}/repos/{repo}/pages", expect=200).json()
    assert cfg["enabled"] is False
    assert cfg["url"].endswith(f"/pages/{owner}/{repo}/")

    # 未开启时站点 404
    assert requests.get(f"{base_url}/pages/{owner}/{repo}/", timeout=15).status_code == 404


def test_pages_enable_serve_disable(user_factory, base_url):
    owner, _, c = user_factory("pg")
    repo = "site"
    c.post("/repos", json={"name": repo, "private": False}, expect=201)

    _commit(c, owner, repo, "main", "index.html", "<h1>hello pages</h1>\n")
    _commit(c, owner, repo, "main", "assets/app.css", "body{color:red}\n")
    _commit(c, owner, repo, "main", "404.html", "<h1>custom nope</h1>\n")

    cfg = c.put(
        f"/users/{owner}/repos/{repo}/pages",
        json={"enabled": True, "branch": "main", "dir": ""},
        expect=200,
    ).json()
    assert cfg["enabled"] is True

    # 根路径 → index.html
    r = requests.get(f"{base_url}/pages/{owner}/{repo}/", timeout=15)
    assert r.status_code == 200 and "hello pages" in r.text
    assert r.headers["content-type"].startswith("text/html")
    # UGC 内容被 sandbox 隔离
    assert "sandbox" in r.headers.get("content-security-policy", "")

    # 静态资源类型
    r = requests.get(f"{base_url}/pages/{owner}/{repo}/assets/app.css", timeout=15)
    assert r.status_code == 200
    assert r.headers["content-type"].startswith("text/css")

    # 目录回退：/assets/ → 无 index.html → 自定义 404
    r = requests.get(f"{base_url}/pages/{owner}/{repo}/assets/", timeout=15)
    assert r.status_code == 404 and "custom nope" in r.text

    # 关闭后站点不可访问
    c.put(
        f"/users/{owner}/repos/{repo}/pages",
        json={"enabled": False, "branch": "main", "dir": ""},
        expect=200,
    )
    assert requests.get(f"{base_url}/pages/{owner}/{repo}/", timeout=15).status_code == 404


def test_pages_source_dir(user_factory, base_url):
    owner, _, c = user_factory("pg")
    repo = "site"
    c.post("/repos", json={"name": repo, "private": False}, expect=201)

    _commit(c, owner, repo, "main", "public/index.html", "<h1>in public</h1>\n")
    c.put(
        f"/users/{owner}/repos/{repo}/pages",
        json={"enabled": True, "branch": "main", "dir": "public"},
        expect=200,
    )
    r = requests.get(f"{base_url}/pages/{owner}/{repo}/", timeout=15)
    assert r.status_code == 200 and "in public" in r.text


def test_pages_invalid_branch_rejected(user_factory):
    owner, _, c = user_factory("pg")
    repo = "site"
    c.post("/repos", json={"name": repo, "private": False}, expect=201)
    c.put(
        f"/users/{owner}/repos/{repo}/pages",
        json={"enabled": True, "branch": "no-such-branch", "dir": ""},
        expect=400,
    )


def test_pages_private_requires_access(user_factory, base_url):
    owner, _, c = user_factory("pg")
    _, _, other = user_factory("pgx")
    repo = "priv"
    c.post("/repos", json={"name": repo, "private": True}, expect=201)
    _commit(c, owner, repo, "main", "index.html", "<h1>secret</h1>\n")
    c.put(
        f"/users/{owner}/repos/{repo}/pages",
        json={"enabled": True, "branch": "main", "dir": ""},
        expect=200,
    )

    # 匿名 → 404（不泄露私有仓库是否开启 Pages）
    assert requests.get(f"{base_url}/pages/{owner}/{repo}/", timeout=15).status_code == 404
    # 无权限用户 → 404
    r = requests.get(
        f"{base_url}/pages/{owner}/{repo}/",
        headers={"Authorization": f"Bearer {other.token}"},
        timeout=15,
    )
    assert r.status_code == 404
    # owner（携带 token）→ 200
    r = requests.get(
        f"{base_url}/pages/{owner}/{repo}/",
        headers={"Authorization": f"Bearer {c.token}"},
        timeout=15,
    )
    assert r.status_code == 200 and "secret" in r.text
