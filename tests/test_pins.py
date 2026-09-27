"""个人仓库置顶（pinned repositories）黑盒测试。"""


def test_repo_pins_happy_path(user_factory):
    username, _, client = user_factory("pin")
    client.post("/repos", json={"name": "pinrepo", "private": True}, expect=201)

    # 初始为空
    body = client.get("/me/pins", expect=200).json()
    assert body["pins"] == []
    assert body["limit"] == 6

    # 置顶
    body = client.post("/me/pins", json={"repo": "pinrepo"}, expect=200).json()
    assert len(body["pins"]) == 1
    assert body["pins"][0]["name"] == "pinrepo"
    assert body["pins"][0]["pinned"] is True

    # 重复置顶幂等
    body = client.post("/me/pins", json={"repo": "pinrepo"}, expect=200).json()
    assert len(body["pins"]) == 1

    # 主页置顶仓库排在最前
    profile = client.get(f"/users/{username}", expect=200).json()
    assert profile["repos"][0]["name"] == "pinrepo"
    assert profile["repos"][0]["pinned"] is True

    # 取消置顶（幂等）
    body = client.delete(f"/me/pins/{username}/pinrepo", expect=200).json()
    assert body["pins"] == []
    client.delete(f"/me/pins/{username}/pinrepo", expect=200)


def test_repo_pins_bad_paths(user_factory, anon):
    username, _, client = user_factory("pinbad")

    client.post("/me/pins", json={}, expect=400)
    client.post("/me/pins", json={"repo": "does-not-exist"}, expect=404)
    # 只能置顶自己的仓库
    client.post("/me/pins", json={"owner": "someone-else", "repo": "x"}, expect=403)

    anon.get("/me/pins", expect=401)
    anon.post("/me/pins", json={"repo": "x"}, expect=401)
    anon.delete(f"/me/pins/{username}/x", expect=401)


def test_repo_pins_limit(user_factory):
    _, _, client = user_factory("pinlimit")
    names = [f"r{i}" for i in range(7)]
    for name in names:
        client.post("/repos", json={"name": name, "private": True}, expect=201)
    for name in names[:6]:
        client.post("/me/pins", json={"repo": name}, expect=200)
    client.post("/me/pins", json={"repo": names[6]}, expect=409)
