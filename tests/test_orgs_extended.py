"""组织扩展黑盒测试：profile / follow / followers（此前未覆盖）。"""

import uuid


def _uuid() -> str:
    return uuid.uuid4().hex[:10]


def test_org_follow_profile_followers(user_factory):
    _, _, c = user_factory("of")
    fname, _, cf = user_factory("off")
    org = f"of-{_uuid()}"
    c.post("/orgs", json={"name": org, "display": "Org"}, expect=201)

    # 非成员 profile：role 为空、未关注、粉丝 0
    p = cf.get(f"/orgs/{org}/profile", expect=200).json()
    assert p["name"] == org
    assert p["is_following"] is False
    assert p["followers"] == 0

    # 关注幂等
    r = cf.post(f"/orgs/{org}/follow", expect=200).json()
    assert r["is_following"] is True and r["followers"] == 1
    r = cf.post(f"/orgs/{org}/follow", expect=200).json()
    assert r["followers"] == 1

    followers = cf.get(f"/orgs/{org}/followers", expect=200).json()
    assert fname in [u["username"] for u in followers]
    assert cf.get(f"/orgs/{org}/profile", expect=200).json()["is_following"] is True

    # 取消关注幂等
    r = cf.delete(f"/orgs/{org}/follow", expect=200).json()
    assert r["is_following"] is False and r["followers"] == 0
    r = cf.delete(f"/orgs/{org}/follow", expect=200).json()
    assert r["followers"] == 0

    # 未知组织一律 404
    ghost = f"ghost-{_uuid()}"
    cf.get(f"/orgs/{ghost}/profile", expect=404)
    cf.post(f"/orgs/{ghost}/follow", expect=404)
    cf.delete(f"/orgs/{ghost}/follow", expect=404)
    cf.get(f"/orgs/{ghost}/followers", expect=404)
