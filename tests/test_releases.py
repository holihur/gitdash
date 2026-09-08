"""Release 黑盒测试：CRUD、附件上传/下载、边界（大小/数量/文件名）与权限。

目标是从行为差异里挖 bug，而不是复述 swagger。
"""

import uuid

import pytest


def _uuid() -> str:
    return uuid.uuid4().hex[:10]


def _r(owner, repo):
    return f"/users/{owner}/repos/{repo}"


@pytest.fixture
def rel_env(user_factory):
    """owner + 公开仓库 + 已存在的 tag v1。"""
    an, _, c = user_factory("r")
    repo = f"rel-{_uuid()}"
    c.post("/repos", json={"name": repo}, expect=201)
    c.post(_r(an, repo) + "/visibility", json={"private": False}, expect=200)
    c.post(_r(an, repo) + "/commits", json={
        "message": "m1",
        "changes": [{"path": "a.txt", "action": "create", "content": "1"}]}, expect=201)
    c.post(_r(an, repo) + "/refs",
           json={"type": "tag", "name": "v1", "from": "main"}, expect=201)
    yield an, c, repo
    try:
        c.delete(f"/repos/{repo}", expect=204)
    except Exception:
        pass


def test_release_create_validation(rel_env):
    an, c, repo = rel_env

    # 空 tag / 不存在的 tag 都必须 400
    c.post(_r(an, repo) + "/releases", json={}, expect=400)
    c.post(_r(an, repo) + "/releases", json={"tag_name": "  "}, expect=400)
    c.post(_r(an, repo) + "/releases", json={"tag_name": "ghost-tag"}, expect=400)

    # 正常创建：tag 已存在
    r = c.post(_r(an, repo) + "/releases",
               json={"tag_name": "v1", "name": "First", "body": "hello"}, expect=201).json()
    assert r["tag_name"] == "v1" and r["name"] == "First" and r["author"] == an

    # 同 tag 重复 → 409
    c.post(_r(an, repo) + "/releases", json={"tag_name": "v1"}, expect=409)


def test_release_read_list_delete(rel_env):
    an, c, repo = rel_env
    c.post(_r(an, repo) + "/releases", json={"tag_name": "v1"}, expect=201)

    got = c.get(_r(an, repo) + "/releases/v1", expect=200).json()
    assert got["tag_name"] == "v1"
    c.get(_r(an, repo) + "/releases/nope", expect=404)

    lst = c.get(_r(an, repo) + "/releases", expect=200).json()
    assert [x["tag_name"] for x in lst] == ["v1"]
    assert c.get(_r(an, repo) + "/releases", expect=200).headers["X-Total-Count"] == "1"

    # 删除 → 404 → 同 tag 可重建（tag 本身还在）
    c.delete(_r(an, repo) + "/releases/v1", expect=204)
    c.get(_r(an, repo) + "/releases/v1", expect=404)
    c.post(_r(an, repo) + "/releases", json={"tag_name": "v1"}, expect=201)


def test_release_asset_lifecycle(rel_env):
    an, c, repo = rel_env
    c.post(_r(an, repo) + "/releases", json={"tag_name": "v1"}, expect=201)
    base = _r(an, repo) + "/releases/v1/assets"

    a = c.post(base, files={"file": ("app.bin", b"\x00\x01bin", "application/octet-stream")},
               expect=201).json()
    assert a["filename"] == "app.bin" and a["size"] == 5

    # 下载内容一致
    dl = c.get(base + "/app.bin", expect=200)
    assert dl.content == b"\x00\x01bin"

    # 同名 409、缺失 404
    c.post(base, files={"file": ("app.bin", b"x")}, expect=409)
    c.get(base + "/ghost.bin", expect=404)

    # 路径穿越文件名必须被清洗成基名
    ev = c.post(base, files={"file": ("../../evil.txt", b"x")}, expect=201).json()
    assert ev["filename"] == "evil.txt"
    assert c.get(base + "/evil.txt", expect=200).content == b"x"
    # ..%2F 下载时也会被 basename 规范化（DB 存储，无文件系统逃逸面）
    assert c.get(base + "/..%2Fevil.txt", expect=200).content == b"x"

    # 列表
    names = {x["filename"] for x in c.get(base, expect=200).json()}
    assert names == {"app.bin", "evil.txt"}

    # 逐个删除到 404
    c.delete(base + "/app.bin", expect=204)
    c.delete(base + "/app.bin", expect=404)


def test_release_asset_limits(rel_env):
    an, c, repo = rel_env
    c.post(_r(an, repo) + "/releases", json={"tag_name": "v1"}, expect=201)
    base = _r(an, repo) + "/releases/v1/assets"

    # 恰好 10MB 允许，10MB+1 拒绝
    big = b"a" * (10 << 20)
    assert c.post(base, files={"file": ("big.bin", big)}, expect=201).json()["size"] == len(big)
    c.post(base, files={"file": ("big2.bin", big + b"x")}, expect=400)

    # 最多 10 个附件
    used = len(c.get(base, expect=200).json())
    for i in range(10 - used):
        c.post(base, files={"file": (f"f{i}", b"x")}, expect=201)
    c.post(base, files={"file": ("overflow", b"x")}, expect=400)


def test_release_permissions(rel_env, user_factory, client_factory):
    an, c, repo = rel_env
    c.post(_r(an, repo) + "/releases", json={"tag_name": "v1"}, expect=201)

    # 陌生人：公开仓库可读，不可写/删
    _, _, out = user_factory()
    out.get(_r(an, repo) + "/releases", expect=200)
    out.get(_r(an, repo) + "/releases/v1/assets", expect=200)
    out.post(_r(an, repo) + "/releases", json={"tag_name": "v1"}, expect=404)
    out.delete(_r(an, repo) + "/releases/v1", expect=404)
    out.delete(f"/repos/{repo}", expect=404)

    # 未认证 401
    anon = client_factory()
    anon.get(_r(an, repo) + "/releases", expect=401)

    # 私有仓库对陌生人整体 404
    bn, _, bob = user_factory()
    priv = f"priv-{_uuid()}"
    bob.post("/repos", json={"name": priv, "private": True}, expect=201)
    bob.post(_r(bn, priv) + "/commits", json={
        "message": "m", "changes": [{"path": "a", "action": "create", "content": "1"}]}, expect=201)
    bob.post(_r(bn, priv) + "/refs", json={"type": "tag", "name": "v1", "from": "main"}, expect=201)
    bob.post(_r(bn, priv) + "/releases", json={"tag_name": "v1"}, expect=201)
    c.get(_r(bn, priv) + "/releases", expect=404)
    c.get(_r(bn, priv) + "/releases/v1/assets/x", expect=404)
