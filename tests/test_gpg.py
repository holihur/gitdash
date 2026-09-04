"""GPG 公钥 API —— happy path + bad path（需要 gpg 生成真实密钥，缺失则跳过）。"""

import shutil
import subprocess

import pytest

pytestmark = pytest.mark.skipif(shutil.which("gpg") is None, reason="gpg not found")


@pytest.fixture(scope="module")
def gpg_armor(tmp_path_factory) -> str:
    d = tmp_path_factory.mktemp("gpg")
    homedir = d / "h"
    homedir.mkdir()
    params = d / "params"
    params.write_text(
        """%no-protection
Key-Type: RSA
Key-Length: 2048
Key-Usage: sign
Name-Real: pytest user
Name-Email: pytest@example.com
Expire-Date: 0
%commit
"""
    )
    env = {"GNUPGHOME": str(homedir)}
    subprocess.run(
        ["gpg", "--batch", "--generate-key", str(params)], env=env, check=True, capture_output=True
    )
    out = subprocess.run(
        ["gpg", "--armor", "--export"], env=env, check=True, capture_output=True, text=True
    )
    return out.stdout


def test_gpg_add_list_delete(user_factory, gpg_armor):
    _, _, c = user_factory("g")

    added = c.post("/gpg", json={"armor": gpg_armor}, expect=201).json()
    fp = added["fingerprint"]
    assert len(fp) == 40 and fp.isalnum()

    keys = c.get("/gpg", expect=200).json()
    assert len(keys) == 1 and keys[0]["id"] == added["id"]

    c.delete(f"/gpg/{added['id']}", expect=204)
    assert c.get("/gpg", expect=200).json() == []
    c.delete(f"/gpg/{added['id']}", expect=404)


def test_gpg_bad_inputs(user_factory, gpg_armor):
    _, _, c = user_factory("g")
    c.post("/gpg", json={"armor": "not a key"}, expect=400)
    c.post("/gpg", json={"armor": ""}, expect=400)
    c.post("/gpg", json={}, expect=400)


def test_gpg_duplicate_and_cross_user(user_factory, gpg_armor):
    _, _, a = user_factory("ga")
    _, _, b = user_factory("gb")

    a.post("/gpg", json={"armor": gpg_armor}, expect=201)
    a.post("/gpg", json={"armor": gpg_armor}, expect=409)  # 同用户
    b.post("/gpg", json={"armor": gpg_armor}, expect=409)  # 指纹全局唯一


def test_gpg_requires_auth(anon, gpg_armor):
    anon.get("/gpg", expect=401)
    anon.post("/gpg", json={"armor": gpg_armor}, expect=401)
    anon.delete("/gpg/1", expect=401)
