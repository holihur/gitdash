"""仓库部署密钥（deploy key）黑盒测试：API 管理 + SSH 访问范围/读写权限。

SSH 用例需自启实例（ssh_port）与本机 git/ssh/ssh-keygen；API 用例仅需 HTTP。
"""

import os
import shutil
import subprocess
import uuid as _uuid

import pytest

pytestmark = pytest.mark.skipif(
    shutil.which("ssh-keygen") is None, reason="ssh-keygen not found"
)

SSH_READY = all(shutil.which(b) for b in ("git", "ssh", "ssh-keygen")) and not os.environ.get(
    "GITDASH_API_URL"
)


def _keygen(d) -> tuple[str, str]:
    key = str(d / f"id-{_uuid.uuid4().hex[:6]}")
    subprocess.run(["ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-f", key], check=True)
    return key, open(key + ".pub").read().strip()


def _git(workdir, key_path, args, *, check=True):
    env = dict(os.environ)
    env["GIT_SSH_COMMAND"] = (
        f"ssh -i {key_path} -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null"
        " -o IdentitiesOnly=yes"
    )
    r = subprocess.run(["git", *args], cwd=workdir, env=env, capture_output=True, text=True)
    if check and r.returncode != 0:
        raise AssertionError(f"git {' '.join(args)} failed rc={r.returncode}: {r.stderr}")
    return r


def _commit(client, owner, repo, path="README.md", content="# hi\n"):
    client.post(
        f"/users/{owner}/repos/{repo}/commits",
        json={
            "branch": "main",
            "message": f"add {path}",
            "changes": [{"path": path, "action": "create", "content": content}],
        },
        expect=201,
    )


# ---- API 管理 ----


def test_deploy_key_crud(user_factory, tmp_path):
    owner, _, c = user_factory("dk")
    repo = "dk-site"
    c.post("/repos", json={"name": repo}, expect=201)

    assert c.get(f"/users/{owner}/repos/{repo}/deploy-keys", expect=200).json() == []

    _, pub = _keygen(tmp_path)
    added = c.post(
        f"/users/{owner}/repos/{repo}/deploy-keys",
        json={"title": "ci", "key": pub, "read_only": False},
        expect=201,
    ).json()
    assert added["name"] == "ci" and added["permission"] == "write"
    assert added["fingerprint"].startswith("SHA256:")
    kid = added["id"]

    keys = c.get(f"/users/{owner}/repos/{repo}/deploy-keys", expect=200).json()
    assert [k["id"] for k in keys] == [kid]

    # 同一把钥匙不能重复注册
    c.post(
        f"/users/{owner}/repos/{repo}/deploy-keys",
        json={"title": "dup", "key": pub, "read_only": True},
        expect=409,
    )
    # 非法公钥 / 缺字段
    c.post(
        f"/users/{owner}/repos/{repo}/deploy-keys",
        json={"title": "bad", "key": "not-a-key", "read_only": True},
        expect=400,
    )
    c.post(
        f"/users/{owner}/repos/{repo}/deploy-keys",
        json={"title": "", "key": pub, "read_only": True},
        expect=400,
    )

    c.delete(f"/users/{owner}/repos/{repo}/deploy-keys/{kid}", expect=204)
    assert c.get(f"/users/{owner}/repos/{repo}/deploy-keys", expect=200).json() == []
    c.delete(f"/users/{owner}/repos/{repo}/deploy-keys/{kid}", expect=404)


def test_deploy_key_owner_only(user_factory, tmp_path):
    owner, _, c = user_factory("dk")
    _, _, other = user_factory("dkx")
    repo = "dk-priv"
    c.post("/repos", json={"name": repo}, expect=201)

    _, pub = _keygen(tmp_path)
    # 非 owner 无权管理（requireOwner 对他人返回 404）
    other.get(f"/users/{owner}/repos/{repo}/deploy-keys", expect=404)
    other.post(
        f"/users/{owner}/repos/{repo}/deploy-keys",
        json={"title": "x", "key": pub, "read_only": True},
        expect=404,
    )


# ---- SSH 端到端（自启实例） ----


@pytest.mark.skipif(not SSH_READY, reason="git/ssh/ssh-keygen + self-spawned instance required")
def test_deploy_key_ssh_write(user_factory, ssh_port, tmp_path):
    owner, _, c = user_factory("dk")
    repo = f"dk-{_uuid.uuid4().hex[:8]}"
    other = f"dk2-{_uuid.uuid4().hex[:8]}"
    c.post("/repos", json={"name": repo}, expect=201)
    _commit(c, owner, repo)

    key, pub = _keygen(tmp_path)
    c.post(
        f"/users/{owner}/repos/{repo}/deploy-keys",
        json={"title": "ci", "key": pub, "read_only": False},
        expect=201,
    )

    work = str(tmp_path / "w")
    url = f"ssh://git@127.0.0.1:{ssh_port}/{owner}/{repo}.git"
    _git(str(tmp_path), key, ["clone", "-q", url, "w"])

    open(os.path.join(work, "ci.txt"), "w").write("from ci\n")
    _git(work, key, ["add", "-A"])
    _git(work, key, ["-c", "user.name=ci", "-c", "user.email=ci@e.c", "commit", "-q", "-m", "ci"])
    _git(work, key, ["push", "-q", "origin", "HEAD"])

    # 单段路径（repo.git）解析为绑定的 owner/repo
    _git(str(tmp_path), key, ["ls-remote", f"ssh://git@127.0.0.1:{ssh_port}/{repo}.git"])

    # 不能访问其他仓库
    c.post("/repos", json={"name": other}, expect=201)
    _commit(c, owner, other)
    r = _git(
        str(tmp_path),
        key,
        ["ls-remote", f"ssh://git@127.0.0.1:{ssh_port}/{owner}/{other}.git"],
        check=False,
    )
    assert r.returncode != 0


@pytest.mark.skipif(not SSH_READY, reason="git/ssh/ssh-keygen + self-spawned instance required")
def test_deploy_key_ssh_read_only(user_factory, ssh_port, tmp_path):
    owner, _, c = user_factory("dk")
    repo = f"dko-{_uuid.uuid4().hex[:8]}"
    c.post("/repos", json={"name": repo}, expect=201)
    _commit(c, owner, repo)

    key, pub = _keygen(tmp_path)
    c.post(
        f"/users/{owner}/repos/{repo}/deploy-keys",
        json={"title": "ro", "key": pub, "read_only": True},
        expect=201,
    )

    work = str(tmp_path / "w")
    url = f"ssh://git@127.0.0.1:{ssh_port}/{owner}/{repo}.git"
    # 只读 key 可以 clone
    _git(str(tmp_path), key, ["clone", "-q", url, "w"])

    # 但不能 push
    open(os.path.join(work, "x.txt"), "w").write("x\n")
    _git(work, key, ["add", "-A"])
    _git(work, key, ["-c", "user.name=ro", "-c", "user.email=ro@e.c", "commit", "-q", "-m", "x"])
    r = _git(work, key, ["push", "-q", "origin", "HEAD"], check=False)
    assert r.returncode != 0
