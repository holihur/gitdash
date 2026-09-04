"""Star / Fork —— happy path + bad path（权限、冲突、来源记录）。"""

import pytest


def _p(owner, repo, suffix=""):
    return f"/users/{owner}/repos/{repo}{suffix}"


def _uuid() -> str:
    import uuid

    return uuid.uuid4().hex[:10]


@pytest.fixture
def star_env(user_factory):
    """alice 建一个公开仓库（含 README 提交），供 bob star/fork。"""
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


def test_star_unstar_and_count(star_env):
    an, bn, alice, bob, repo = star_env

    # 初始未 star，计数 0
    r = bob.get(_p(an, repo), expect=200).json()
    assert r["stars"] == 0 and r["starred"] is False

    # star
    s = bob.put(_p(an, repo, "/star"), expect=200).json()
    assert s["starred"] is True and s["stars"] == 1
    r = bob.get(_p(an, repo), expect=200).json()
    assert r["stars"] == 1 and r["starred"] is True

    # 幂等 star（不重复计数）
    s = bob.put(_p(an, repo, "/star"), expect=200).json()
    assert s["stars"] == 1

    # alice 也可以 star（不同用户计数叠加）
    alice.put(_p(an, repo, "/star"), expect=200)
    assert bob.get(_p(an, repo), expect=200).json()["stars"] == 2

    # starred 列表（bob star 了 alice 的仓库）
    starred = [(r["owner"], r["name"]) for r in bob.get("/starred", expect=200).json()]
    assert (an, repo) in starred

    # unstar
    s = bob.delete(_p(an, repo, "/star"), expect=200).json()
    assert s["starred"] is False and s["stars"] == 1
    starred = [(r["owner"], r["name"]) for r in bob.get("/starred", expect=200).json()]
    assert (an, repo) not in starred


def test_star_private_repo_forbidden(user_factory):
    an, _, alice = user_factory("alice")
    bn, _, bob = user_factory("bob")
    repo = f"secret-{_uuid()}"
    alice.post("/repos", json={"name": repo}, expect=201)
    bob.put(_p(an, repo, "/star"), expect=404)
    bob.delete(_p(an, repo, "/star"), expect=404)
    alice.delete(f"/repos/{repo}", expect=204)


def test_fork_copies_content_and_records_source(star_env):
    an, bn, alice, bob, repo = star_env

    r = bob.post(_p(an, repo, "/fork"), json={}, expect=201).json()
    assert r["owner"] == bn and r["name"] == repo

    # fork 来源已记录
    fr = bob.get(_p(bn, repo), expect=200).json()
    assert fr["fork_owner"] == an and fr["fork_repo"] == repo

    # 分支与内容一致
    branches = bob.get(_p(bn, repo, "/branches"), expect=200).json()
    assert [b["name"] for b in branches] == ["main"]
    blob = bob.get(_p(bn, repo, "/blob?ref=main&path=README.md"), expect=200).json()
    assert blob["content"] == f"# {repo}\n"

    # 源仓库仍在
    alice.get(_p(an, repo), expect=200)
    bob.delete(f"/repos/{repo}", expect=204)


def test_fork_custom_name_and_conflict(star_env):
    an, bn, alice, bob, repo = star_env

    r = bob.post(_p(an, repo, "/fork"), json={"name": "my-copy"}, expect=201).json()
    assert r["name"] == "my-copy"

    # 再次 fork 同名冲突
    bob.post(_p(an, repo, "/fork"), json={"name": "my-copy"}, expect=409)
    bob.delete(f"/repos/my-copy", expect=204)


def test_fork_private_forbidden(user_factory):
    an, _, alice = user_factory("alice")
    bn, _, bob = user_factory("bob")
    repo = f"secret-{_uuid()}"
    alice.post("/repos", json={"name": repo}, expect=201)
    bob.post(_p(an, repo, "/fork"), json={}, expect=404)
    alice.delete(f"/repos/{repo}", expect=204)


def test_star_fork_requires_auth(anon, star_env):
    an, _, _, _, repo = star_env
    anon.post(_p(an, repo, "/fork"), json={}, expect=401)
    anon.put(_p(an, repo, "/star"), expect=401)
    anon.get("/starred", expect=401)
