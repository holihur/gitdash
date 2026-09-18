"""全局代码搜索黑盒测试：跨仓库、内联限定符、权限隔离。

用 HTTP-only 的 commits 接口写入可搜索内容，无需 git/ssh。
"""

from __future__ import annotations

import uuid

import pytest


def _uuid() -> str:
    return uuid.uuid4().hex[:10]


def _p(owner, repo, suffix=""):
    return f"/users/{owner}/repos/{repo}{suffix}"


def _write(client, owner, repo, path, content, message="add"):
    client.post(
        _p(owner, repo, "/commits"),
        json={"message": message, "changes": [{"path": path, "action": "create", "content": content}]},
        expect=201,
    )


def _search(client, expect=200, **params):
    from urllib.parse import urlencode

    return client.get(f"/search/code?{urlencode(params)}", expect=expect).json()


@pytest.fixture(scope="module")
def code_env(base_url):
    from conftest import ApiClient

    def new_user(prefix):
        c = ApiClient(base_url)
        name = f"{prefix}-{_uuid()}"
        c.token = c.post(
            "/auth/register", json={"username": name, "password": "test-pass-123456"}, expect=201
        ).json()["token"]
        return name, c

    alice, alice_c = new_user("alice")
    bob, bob_c = new_user("bob")
    carol, carol_c = new_user("carol")

    alpha = f"alpha-{_uuid()[:6]}"
    gamma = f"gamma-{_uuid()[:6]}"
    beta = f"beta-{_uuid()[:6]}"
    for owner, c, repo in ((alice, alice_c, alpha), (carol, carol_c, gamma), (bob, bob_c, beta)):
        c.post("/repos", json={"name": repo, "template": "readme"}, expect=201)
    # alpha / gamma 公开，beta 保持私有
    alice_c.post(_p(alice, alpha, "/visibility"), json={"private": False}, expect=200)
    carol_c.post(_p(carol, gamma, "/visibility"), json={"private": False}, expect=200)

    _write(alice_c, alice, alpha, "src/main.go", "package main\n// uniqueAlphaToken here\n")
    _write(alice_c, alice, alpha, "docs/readme.md", "uniqueAlphaToken in markdown\n")
    _write(carol_c, carol, gamma, "lib/util.py", "uniqueAlphaToken = 1\n")
    _write(bob_c, bob, beta, "secret.go", "package main\n// uniqueBetaToken secret\n")

    yield {
        "alice": (alice, alice_c),
        "bob": (bob, bob_c),
        "alpha": alpha,
        "gamma": gamma,
        "beta": beta,
    }
    for owner, c, repo in ((alice, alice_c, alpha), (carol, carol_c, gamma), (bob, bob_c, beta)):
        try:
            c.delete(f"/repos/{repo}", expect=204)
        except Exception:
            pass


def test_code_search_across_public_repos(code_env):
    alice, c = code_env["alice"]
    r = _search(c, q="uniqueAlphaToken")
    repos = {hit["repo"] for hit in r["results"]}
    assert code_env["alpha"] in repos and code_env["gamma"] in repos
    assert r["repos_searched"] >= 2
    assert all(hit["text"] for hit in r["results"])


def test_private_repo_not_visible_to_others(code_env):
    alice, c = code_env["alice"]
    r = _search(c, q="uniqueBetaToken")
    assert r["results"] == []


def test_owner_can_search_own_private_repo(code_env):
    bob, c = code_env["bob"]
    r = _search(c, q="uniqueBetaToken")
    assert [hit["repo"] for hit in r["results"]] == [code_env["beta"]]


def test_lang_qualifier(code_env):
    _, c = code_env["alice"]
    r = _search(c, q="lang:go uniqueAlphaToken")
    assert r["results"]
    assert all(hit["path"].endswith(".go") for hit in r["results"])


def test_path_qualifier(code_env):
    _, c = code_env["alice"]
    r = _search(c, q="path:src uniqueAlphaToken")
    assert r["results"]
    assert all(hit["path"].startswith("src/") for hit in r["results"])


def test_repo_qualifier(code_env):
    alice, c = code_env["alice"]
    r = _search(c, q=f"repo:{alice}/{code_env['alpha']} uniqueAlphaToken")
    assert r["results"]
    assert {hit["repo"] for hit in r["results"]} == {code_env["alpha"]}


def test_repo_qualifier_inaccessible_is_empty(code_env):
    _, c = code_env["alice"]
    bob = code_env["bob"][0]
    r = _search(c, q=f"repo:{bob}/{code_env['beta']} uniqueBetaToken")
    assert r["results"] == []


def test_symbol_qualifier_word_boundary(code_env):
    _, c = code_env["alice"]
    # uniqueAlphaTokenFoo 不应命中 symbol:uniqueAlphaToken
    _write(c, code_env["alice"][0], code_env["alpha"], "extra.go", "package main\nvar uniqueAlphaTokenFoo = 1\n")
    r = _search(c, q="symbol:uniqueAlphaToken", path="extra.go")
    assert r["results"] == []
    r = _search(c, q="symbol:uniqueAlphaToken", path="src/main.go")
    assert r["results"]


def test_explicit_params(code_env):
    _, c = code_env["alice"]
    r = _search(c, q="uniqueAlphaToken", lang="go", path="src")
    assert r["results"]
    assert all(hit["path"].endswith(".go") and hit["path"].startswith("src/") for hit in r["results"])


def test_query_required(code_env):
    _, c = code_env["alice"]
    r = c.get("/search/code", expect=400).json()
    assert r["code"] == "query_required"


def test_code_search_requires_auth(code_env, anon):
    anon.get("/search/code?q=x", expect=401)
