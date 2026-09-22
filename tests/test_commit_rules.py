"""仓库提交身份校验：API 配置 + pre-receive 拒绝不符合格式的提交。"""

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


def _git(workdir, key, args, *, check=True):
    env = dict(os.environ)
    env["GIT_SSH_COMMAND"] = (
        f"ssh -i {key} -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null"
        " -o IdentitiesOnly=yes"
    )
    r = subprocess.run(["git", *args], cwd=workdir, env=env, capture_output=True, text=True)
    if check and r.returncode != 0:
        raise AssertionError(f"git {' '.join(args)} failed rc={r.returncode}: {r.stderr}")
    return r


def _api_commit(client, owner, repo, path="README.md"):
    client.post(
        f"/users/{owner}/repos/{repo}/commits",
        json={
            "branch": "main",
            "message": f"add {path}",
            "changes": [{"path": path, "action": "create", "content": "# hi\n"}],
        },
        expect=201,
    )


def test_commit_rules_api(user_factory):
    owner, _, c = user_factory("cr")
    repo = "cr-api"
    c.post("/repos", json={"name": repo}, expect=201)

    cfg = c.get(f"/users/{owner}/repos/{repo}/commit-rules", expect=200).json()
    assert cfg["enabled"] is False

    saved = c.put(
        f"/users/{owner}/repos/{repo}/commit-rules",
        json={"name_pattern": "^[a-z]+$", "email_pattern": r"@example\.com$"},
        expect=200,
    ).json()
    assert saved["enabled"] is True and saved["name_pattern"] == "^[a-z]+$"

    assert c.get(f"/users/{owner}/repos/{repo}/commit-rules", expect=200).json()["enabled"] is True

    # 非法正则
    c.put(
        f"/users/{owner}/repos/{repo}/commit-rules",
        json={"name_pattern": "[", "email_pattern": ""},
        expect=400,
    )

    # 清空两项 = 关闭
    c.put(
        f"/users/{owner}/repos/{repo}/commit-rules",
        json={"name_pattern": "", "email_pattern": ""},
        expect=200,
    )
    assert c.get(f"/users/{owner}/repos/{repo}/commit-rules", expect=200).json()["enabled"] is False


@pytest.mark.skipif(not SSH_READY, reason="git/ssh/ssh-keygen + self-spawned instance required")
def test_commit_rules_reject_push(user_factory, ssh_port, tmp_path):
    owner, _, c = user_factory("cr")
    repo = f"cr-{_uuid.uuid4().hex[:8]}"
    c.post("/repos", json={"name": repo}, expect=201)
    _api_commit(c, owner, repo)

    key, pub = _keygen(tmp_path)
    c.post("/keys", json={"name": "k", "public_key": pub}, expect=201)

    c.put(
        f"/users/{owner}/repos/{repo}/commit-rules",
        json={"name_pattern": "^[a-z]+$", "email_pattern": r"^[^@]+@allowed\.example$"},
        expect=200,
    )

    url = f"ssh://git@127.0.0.1:{ssh_port}/{owner}/{repo}.git"
    work = str(tmp_path / "w")
    _git(str(tmp_path), key, ["clone", "-q", url, "w"])

    def commit(path, name, email, msg):
        open(os.path.join(work, path), "w").write(msg + "\n")
        _git(work, key, ["add", "-A"])
        _git(
            work,
            key,
            ["-c", f"user.name={name}", "-c", f"user.email={email}", "commit", "-q", "-m", msg],
        )

    # 合法提交 → 允许
    commit("ok.txt", "alice", "alice@allowed.example", "ok")
    _git(work, key, ["push", "-q", "origin", "HEAD"])

    # 姓名不符合（大写）→ 拒绝
    _git(work, key, ["reset", "-q", "--hard", "origin/main"])
    commit("badname.txt", "Alice", "alice@allowed.example", "badname")
    r = _git(work, key, ["push", "-q", "origin", "HEAD"], check=False)
    assert r.returncode != 0 and "required format" in (r.stderr + r.stdout)

    # 邮箱不符合 → 拒绝
    _git(work, key, ["reset", "-q", "--hard", "origin/main"])
    commit("bademail.txt", "alice", "alice@other.example", "bademail")
    r = _git(work, key, ["push", "-q", "origin", "HEAD"], check=False)
    assert r.returncode != 0 and "required format" in (r.stderr + r.stdout)

    # 关闭规则后，同样的非法提交可推送
    c.put(
        f"/users/{owner}/repos/{repo}/commit-rules",
        json={"name_pattern": "", "email_pattern": ""},
        expect=200,
    )
    _git(work, key, ["reset", "-q", "--hard", "origin/main"])
    commit("free.txt", "Alice", "alice@other.example", "free")
    _git(work, key, ["push", "-q", "origin", "HEAD"])
