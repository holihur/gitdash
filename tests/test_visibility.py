"""仓库可见性（私有/公开）与 Explore 发现页 —— happy path + 权限。"""

import pytest


def _uuid() -> str:
    import uuid

    return uuid.uuid4().hex[:10]


def _p(owner, repo):
    return f"/users/{owner}/repos/{repo}"


@pytest.fixture
def repo_env(user_factory):
    an, _, c = user_factory("vis")
    repo = f"vis-{_uuid()}"
    c.post("/repos", json={"name": repo}, expect=201)
    yield an, c, repo
    try:
        c.delete(f"/repos/{repo}", expect=204)
    except Exception:
        pass


def _explore_names(client):
    return [
        f'{r["owner"]}/{r["name"]}' for r in client.get("/explore/repos", expect=200).json()
    ]


def test_new_repo_private_by_default(repo_env):
    an, c, repo = repo_env
    assert c.get(_p(an, repo), expect=200).json()["private"] is True
    assert f"{an}/{repo}" not in _explore_names(c)


def test_make_public_visible_in_explore(repo_env):
    an, c, repo = repo_env
    r = c.post(_p(an, repo) + "/visibility", json={"private": False}, expect=200).json()
    assert r["private"] is False
    # 公开仓库出现在 Explore（包括自己的）
    assert f"{an}/{repo}" in _explore_names(c)


def test_back_to_private_hides_from_explore(repo_env):
    an, c, repo = repo_env
    c.post(_p(an, repo) + "/visibility", json={"private": False}, expect=200)
    c.post(_p(an, repo) + "/visibility", json={"private": True}, expect=200)
    assert c.get(_p(an, repo), expect=200).json()["private"] is True
    assert f"{an}/{repo}" not in _explore_names(c)


def test_public_repo_readable_by_stranger(user_factory, repo_env):
    an, c, repo = repo_env
    c.post(_p(an, repo) + "/visibility", json={"private": False}, expect=200)
    _, _, stranger = user_factory("s")
    # 陌生人：可见 + 可读仓库信息；也出现在陌生人的 Explore 列表
    stranger.get(_p(an, repo), expect=200)
    assert f"{an}/{repo}" in _explore_names(stranger)


def test_private_repo_hidden_from_stranger(user_factory, repo_env):
    an, c, repo = repo_env
    _, _, stranger = user_factory("s")
    stranger.get(_p(an, repo), expect=404)


def test_visibility_requires_owner(user_factory, repo_env):
    an, c, repo = repo_env
    c.post(_p(an, repo) + "/visibility", json={"private": False}, expect=200)
    _, _, stranger = user_factory("s")
    stranger.post(_p(an, repo) + "/visibility", json={"private": True}, expect=404)


def test_visibility_missing_field_400(repo_env):
    an, c, repo = repo_env
    c.post(_p(an, repo) + "/visibility", json={}, expect=400)


def test_visibility_requires_auth(client_factory):
    client_factory().post("/users/x/repos/y/visibility", json={"private": False}, expect=401)
