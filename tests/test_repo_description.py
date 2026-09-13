"""仓库描述修改 —— happy path + 权限 + 长度限制。"""

import pytest


def _uuid() -> str:
    import uuid

    return uuid.uuid4().hex[:10]


def _p(owner, repo):
    return f"/users/{owner}/repos/{repo}"


@pytest.fixture
def repo_env(user_factory):
    an, _, c = user_factory("desc")
    repo = f"desc-{_uuid()}"
    c.post("/repos", json={"name": repo, "description": "old"}, expect=201)
    yield an, c, repo
    try:
        c.delete(f"/repos/{repo}", expect=204)
    except Exception:
        pass


def test_update_description(repo_env):
    an, c, repo = repo_env
    r = c.post(_p(an, repo) + "/description", json={"description": "  new desc  "}, expect=200)
    assert r.json()["description"] == "new desc"
    assert c.get(_p(an, repo), expect=200).json()["description"] == "new desc"


def test_clear_description(repo_env):
    an, c, repo = repo_env
    r = c.post(_p(an, repo) + "/description", json={"description": ""}, expect=200)
    assert r.json()["description"] == ""


def test_description_too_long(repo_env):
    an, c, repo = repo_env
    c.post(_p(an, repo) + "/description", json={"description": "a" * 501}, expect=400)


def test_description_requires_owner(user_factory, repo_env):
    an, c, repo = repo_env
    _, _, stranger = user_factory("s")
    stranger.post(_p(an, repo) + "/description", json={"description": "hacked"}, expect=404)


def test_description_requires_auth(client_factory):
    client_factory().post("/users/x/repos/y/description", json={"description": "x"}, expect=401)
