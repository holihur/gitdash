"""SSH 公钥 API —— happy path + bad path（含全局指纹唯一约束）。"""

import shutil
import subprocess

import pytest

pytestmark = pytest.mark.skipif(
    shutil.which("ssh-keygen") is None, reason="ssh-keygen not found"
)


@pytest.fixture(scope="module")
def pub_keys(tmp_path_factory) -> list[str]:
    """每次会话用 ssh-keygen 生成 4 把全新公钥（fingerprint 全局唯一，避免跨用例残留）。"""
    d = tmp_path_factory.mktemp("keys")
    pubs = []
    for i in range(4):
        p = d / f"k{i}"
        subprocess.run(
            ["ssh-keygen", "-t", "ed25519", "-N", "", "-C", f"pytest-{i}", "-f", str(p)],
            check=True,
            capture_output=True,
        )
        pubs.append(p.with_suffix(".pub").read_text().strip())
    return pubs


def _list_keys(client) -> list[dict]:
    return client.get("/keys", expect=200).json()


def test_key_add_list_delete(user_factory, pub_keys):
    k1, k2, *_ = pub_keys
    _, _, client = user_factory()

    assert _list_keys(client) == []

    added = client.post("/keys", json={"name": "my laptop", "public_key": k1}, expect=201).json()
    assert added["name"] == "my laptop"
    assert added["fingerprint"].startswith("SHA256:")
    key_id = added["id"]

    keys = _list_keys(client)
    assert len(keys) == 1
    assert keys[0]["id"] == key_id

    # 添加第二把 key
    client.post("/keys", json={"name": "server", "public_key": k2}, expect=201)
    assert len(_list_keys(client)) == 2

    # 删除后消失
    client.delete(f"/keys/{key_id}", expect=204)
    assert all(k["id"] != key_id for k in _list_keys(client))
    client.delete(f"/keys/{key_id}", expect=404)


def test_key_invalid_public_key(user_factory, pub_keys):
    _, _, client = user_factory()
    client.post("/keys", json={"name": "nope", "public_key": "not-a-key"}, expect=400)
    client.post("/keys", json={"name": "nope", "public_key": ""}, expect=400)


def test_key_missing_name(user_factory, pub_keys):
    _, _, client = user_factory()
    client.post("/keys", json={"name": "", "public_key": pub_keys[0]}, expect=400)
    client.post("/keys", json={"public_key": pub_keys[0]}, expect=400)


def test_key_duplicate_fingerprint(user_factory, pub_keys):
    k3, k4 = pub_keys[2], pub_keys[3]
    _, _, alice = user_factory("alice")
    alice.post("/keys", json={"name": "a", "public_key": k3}, expect=201)
    # 同一用户重复添加
    alice.post("/keys", json={"name": "a2", "public_key": k3}, expect=409)

    # 不同用户也无法注册同一把公钥（fingerprint 全局唯一）
    _, _, bob = user_factory("bob")
    bob.post("/keys", json={"name": "b", "public_key": k3}, expect=409)
    # bob 用一把从未注册过的 key 正常
    bob.post("/keys", json={"name": "b", "public_key": k4}, expect=201)


def test_key_delete_unknown_id(user_factory):
    _, _, client = user_factory()
    client.delete("/keys/999999", expect=404)
    client.delete("/keys/abc", expect=400)


def test_keys_requires_auth(anon, pub_keys):
    anon.get("/keys", expect=401)
    anon.post("/keys", json={"name": "x", "public_key": pub_keys[0]}, expect=401)
    anon.delete("/keys/1", expect=401)
