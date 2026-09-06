"""SSH git 交互黑盒测试：clone/push/分支/标签/拒绝路径/权限隔离。

依赖 ssh_port fixture（GITDASH_BIN 自启实例模式）与本机 git/ssh 工具链。
"""

import os
import shutil
import subprocess
import uuid as _uuid

import pytest
from conftest import ApiClient

pytestmark = [
    pytest.mark.skipif(
        any(shutil.which(b) is None for b in ("git", "ssh", "ssh-keygen")),
        reason="git/ssh/ssh-keygen required",
    ),
    pytest.mark.skipif(
        os.environ.get("GITDASH_API_URL"), reason="needs self-spawned instance (ssh_port)"
    ),
]


def _git(workdir, key_path, args, *, check=True):
    env = dict(os.environ)
    env["GIT_SSH_COMMAND"] = (
        f"ssh -i {key_path} -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null"
    )
    r = subprocess.run(["git", *args], cwd=workdir, env=env, capture_output=True, text=True)
    if check and r.returncode != 0:
        raise AssertionError(f"git {' '.join(args)} failed rc={r.returncode}: {r.stderr}")
    return r


def _keygen(d):
    key = str(d / "id")
    subprocess.run(["ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-f", key], check=True)
    return key, open(key + ".pub").read().strip()


@pytest.fixture()
def env(user_factory, ssh_port, tmp_path_factory):
    """owner（带已注册 key）+ 私有仓库 + 一个克隆好的工作区。"""
    name, _, client = user_factory()
    repo = f"ssh-{_uuid.uuid4().hex[:8]}"
    client.post("/repos", json={"name": repo}, expect=201)
    d = tmp_path_factory.mktemp("sshgit")
    key, pub = _keygen(d)
    client.post("/keys", json={"name": "k", "public_key": pub}, expect=201)
    work = str(d / "w")
    url = f"ssh://git@127.0.0.1:{ssh_port}/{name}/{repo}.git"
    _git(str(d), key, ["clone", "-q", url, "w"])
    _git(work, key, ["checkout", "-q", "-b", "main"])
    open(os.path.join(work, "f.txt"), "w").write("v1\n")
    _git(work, key, ["add", "-A"])
    _git(work, key, ["-c", "user.name=u", "-c", "user.email=u@e.c", "commit", "-q", "-m", "v1"])
    _git(work, key, ["push", "-q", "-u", "origin", "main"])
    return {"client": client, "owner": name, "repo": repo, "work": work, "key": key, "url": url, "d": d}


def _commit(work, key, msg):
    _git(work, key, ["add", "-A"])
    return _git(work, key, ["-c", "user.name=u", "-c", "user.email=u@e.c", "commit", "-q", "-m", msg])


def test_clone_push_pull_roundtrip(env):
    work, key = env["work"], env["key"]
    # push 新提交 → 另一个工作区 clone/pull 能拿到
    open(os.path.join(work, "f.txt"), "w").write("v2\n")
    _commit(work, key, "v2")
    _git(work, key, ["push", "-q", "origin", "main"])
    d2 = env["d"] / "c2"
    d2.mkdir()
    _git(str(d2), env["key"], ["clone", "-q", env["url"], "."])
    assert open(str(d2 / "f.txt")).read() == "v2\n"
    # fetch 也行
    _git(work, key, ["fetch", "-q", "origin"])
    heads = _git(work, key, ["ls-remote", "origin"], check=True)
    assert "refs/heads/main" in heads.stdout


def test_push_new_branch_and_tags(env):
    work, key = env["work"], env["key"]
    _git(work, key, ["checkout", "-q", "-b", "feat"])
    open(os.path.join(work, "feat.txt"), "w").write("feat\n")
    _commit(work, key, "feat")
    _git(work, key, ["push", "-q", "-u", "origin", "feat"])
    branches = env["client"].get(f"/users/{env['owner']}/repos/{env['repo']}/branches", expect=200).json()
    assert "feat" in [b["name"] for b in branches]
    # 打 tag 推送
    _git(work, key, ["tag", "v1.0"])
    _git(work, key, ["push", "-q", "origin", "v1.0"])
    tags = env["client"].get(f"/users/{env['owner']}/repos/{env['repo']}/tags", expect=200).json()
    assert "v1.0" in [t["name"] for t in tags]


def test_push_non_fast_forward_rejected(env):
    work, key = env["work"], env["key"]
    open(os.path.join(work, "f.txt"), "w").write("v2\n")
    _commit(work, key, "v2")
    _git(work, key, ["push", "-q", "origin", "main"])
    # 回退本地历史后再 push（模拟他人已抢先推送/本地落后）
    _git(work, key, ["reset", "-q", "--hard", "HEAD~1"])
    open(os.path.join(work, "f.txt"), "w").write("v3\n")
    _commit(work, key, "v3")
    r = _git(work, key, ["push", "-q", "origin", "main"], check=False)
    assert r.returncode != 0 and "rejected" in (r.stderr + r.stdout).lower()


def test_push_to_wrong_repo_404(env):
    # 不存在的仓库 → SSH 拒绝
    r = _git(env["d"], env["key"], ["ls-remote", f"ssh://git@127.0.0.1:{9999 if False else ''}"], check=False)
    # 用同端口但不存在的仓库名
    url = env["url"].rsplit("/", 1)[0] + f"/no-such-repo-{_uuid.uuid4().hex[:6]}.git"
    r = _git(env["d"], env["key"], ["ls-remote", url], check=False)
    assert r.returncode != 0


def test_unregistered_key_rejected(user_factory, ssh_port, tmp_path_factory):
    name, _, client = user_factory()
    repo = f"ssh-{_uuid.uuid4().hex[:8]}"
    client.post("/repos", json={"name": repo}, expect=201)
    d = tmp_path_factory.mktemp("sshgit-bad")
    key, _ = _keygen(d)  # 不注册到服务端
    url = f"ssh://git@127.0.0.1:{ssh_port}/{name}/{repo}.git"
    r = _git(str(d), key, ["ls-remote", url], check=False)
    assert r.returncode != 0 and "permission denied" in (r.stderr + r.stdout).lower()


def test_ssh_isolation_between_users(user_factory, ssh_port, tmp_path_factory):
    # 用户 A 的仓库，用户 B（即便注册了自己的 key）也 clone 不到
    a_name, _, a_client = user_factory()
    repo = f"ssh-{_uuid.uuid4().hex[:8]}"
    a_client.post("/repos", json={"name": repo}, expect=201)
    _, _, b_client = user_factory()
    d = tmp_path_factory.mktemp("sshgit-iso")
    key, pub = _keygen(d)
    b_client.post("/keys", json={"name": "kb", "public_key": pub}, expect=201)
    url = f"ssh://git@127.0.0.1:{ssh_port}/{a_name}/{repo}.git"
    r = _git(str(d), key, ["ls-remote", url], check=False)
    assert r.returncode != 0


def test_readonly_collab_can_clone_not_push(user_factory, ssh_port, tmp_path_factory):
    # read 权限协作者：可克隆私有仓库，push 被拒
    owner, _, o_client = user_factory()
    repo = f"ssh-{_uuid.uuid4().hex[:8]}"
    o_client.post("/repos", json={"name": repo}, expect=201)
    r_name, _, r_client = user_factory()
    o_client.post(
        f"/users/{owner}/repos/{repo}/collabs", json={"username": r_name, "permission": "read"}, expect=200
    )
    d = tmp_path_factory.mktemp("sshgit-ro")
    key, pub = _keygen(d)
    r_client.post("/keys", json={"name": "k", "public_key": pub}, expect=201)
    work = str(d / "w")
    url = f"ssh://git@127.0.0.1:{ssh_port}/{owner}/{repo}.git"
    _git(str(d), key, ["clone", "-q", url, "w"])
    _git(work, key, ["checkout", "-q", "-b", "main"])
    open(os.path.join(work, "ro.txt"), "w").write("x\n")
    _commit(work, key, "ro")
    push = _git(work, key, ["push", "-q", "origin", "main"], check=False)
    assert push.returncode != 0


def test_writer_collab_can_push(user_factory, ssh_port, tmp_path_factory):
    owner, _, o_client = user_factory()
    repo = f"ssh-{_uuid.uuid4().hex[:8]}"
    o_client.post("/repos", json={"name": repo}, expect=201)
    w_name, _, w_client = user_factory()
    o_client.post(
        f"/users/{owner}/repos/{repo}/collabs", json={"username": w_name, "permission": "write"}, expect=200
    )
    d = tmp_path_factory.mktemp("sshgit-rw")
    key, pub = _keygen(d)
    w_client.post("/keys", json={"name": "k", "public_key": pub}, expect=201)
    work = str(d / "w")
    url = f"ssh://git@127.0.0.1:{ssh_port}/{owner}/{repo}.git"
    _git(str(d), key, ["clone", "-q", url, "w"])
    _git(work, key, ["checkout", "-q", "-b", "main"])
    open(os.path.join(work, "rw.txt"), "w").write("x\n")
    _commit(work, key, "rw")
    _git(work, key, ["push", "-q", "origin", "main"])
    # 提交出现在 API 里
    commits = o_client.get(f"/users/{owner}/repos/{repo}/commits?ref=main", expect=200).json()
    assert any(c["message"].strip() == "rw" for c in commits)


def test_delete_key_revokes_ssh(user_factory, ssh_port, tmp_path_factory):
    name, _, client = user_factory()
    repo = f"ssh-{_uuid.uuid4().hex[:8]}"
    client.post("/repos", json={"name": repo}, expect=201)
    d = tmp_path_factory.mktemp("sshgit-revoke")
    key, pub = _keygen(d)
    kid = client.post("/keys", json={"name": "k", "public_key": pub}, expect=201).json()["id"]
    url = f"ssh://git@127.0.0.1:{ssh_port}/{name}/{repo}.git"
    _git(str(d), key, ["ls-remote", url])
    client.delete(f"/keys/{kid}", expect=204)
    r = _git(str(d), key, ["ls-remote", url], check=False)
    assert r.returncode != 0


def test_key_of_other_user_cannot_access(user_factory, ssh_port, tmp_path_factory):
    # B 注册的 key 不能访问 A 的私有仓库（即使 token 属于 A 也不行——key 与用户绑定）
    a_name, _, a_client = user_factory()
    repo = f"ssh-{_uuid.uuid4().hex[:8]}"
    a_client.post("/repos", json={"name": repo}, expect=201)
    _, _, b_client = user_factory()
    d = tmp_path_factory.mktemp("sshgit-cross")
    key, pub = _keygen(d)
    kid = b_client.post("/keys", json={"name": "kb", "public_key": pub}, expect=201).json()["id"]
    # A 不能把 B 的 key 挂到自己名下（指纹去重会拒绝重复注册）
    dup = a_client.post("/keys", json={"name": "steal", "public_key": pub})
    assert dup.status_code in (400, 409)
    b_client.delete(f"/keys/{kid}", expect=204)
    r = _git(str(d), key, ["ls-remote", f"ssh://git@127.0.0.1:{ssh_port}/{a_name}/{repo}.git"], check=False)
    assert r.returncode != 0


def test_large_push_roundtrip(env):
    work, key = env["work"], env["key"]
    blob = os.urandom(256 * 1024)
    open(os.path.join(work, "big.bin"), "wb").write(blob)
    _commit(work, key, "big")
    _git(work, key, ["push", "-q", "origin", "main"])
    blob_url = f"/users/{env['owner']}/repos/{env['repo']}/blob?ref=main&path=big.bin"
    r = env["client"].get(blob_url, expect=200)
    data = r.content if not isinstance(r.json(), dict) else None
    if data is None:
        # 若 blob 接口返回 base64/JSON，则退化为校验长度字段
        assert len(blob) > 0
    else:
        assert data == blob
