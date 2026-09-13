"""用户主页与关注（follow）—— happy path + 权限 + 边界。"""

import pytest


@pytest.fixture
def users(user_factory):
    a, _, ca = user_factory("fa")
    b, _, cb = user_factory("fb")
    return a, ca, b, cb


def test_follow_unfollow(users):
    a, ca, b, cb = users

    # b 关注 a
    r = cb.post(f"/users/{a}/follow", expect=200).json()
    assert r["followers"] == 1 and r["is_following"] is True
    # 幂等
    r = cb.post(f"/users/{a}/follow", expect=200).json()
    assert r["followers"] == 1

    # 主页（b 视角）
    prof = cb.get(f"/users/{a}", expect=200).json()
    assert prof["username"] == a
    assert prof["followers"] == 1 and prof["following"] == 0
    assert prof["is_following"] is True

    # 主页（a 视角，不能关注自己）
    own = ca.get(f"/users/{a}", expect=200).json()
    assert own["is_following"] is False

    # 不能关注自己 / 不能关注不存在的用户
    ca.post(f"/users/{a}/follow", expect=400)
    cb.post("/users/ghost-user-xyz/follow", expect=404)

    # 粉丝 / 关注列表
    cb.get(f"/users/{a}/followers", expect=200)
    cb.get(f"/users/{a}/following", expect=200)

    # 取消关注（幂等）
    r = cb.delete(f"/users/{a}/follow", expect=200).json()
    assert r["followers"] == 0 and r["is_following"] is False
    cb.delete(f"/users/{a}/follow", expect=200)


def test_profile_repo_visibility(users):
    a, ca, b, cb = users
    ca.post("/repos", json={"name": "pub"}, expect=201)
    ca.post("/repos", json={"name": "priv"}, expect=201)
    ca.post(f"/users/{a}/repos/pub/visibility", json={"private": False}, expect=200)

    # 他人只看得到公开仓库
    prof = cb.get(f"/users/{a}", expect=200).json()
    assert [r["name"] for r in prof["repos"]] == ["pub"]
    # 本人看得到全部
    own = ca.get(f"/users/{a}", expect=200).json()
    assert sorted(r["name"] for r in own["repos"]) == ["priv", "pub"]


def test_profile_not_found(users):
    _, ca, _, _ = users
    ca.get("/users/ghost-user-xyz", expect=404)
    ca.get("/users/ghost-user-xyz/followers", expect=404)


def test_follow_requires_auth(client_factory):
    client_factory().post("/users/someone/follow", expect=401)
    client_factory().get("/users/someone", expect=401)
