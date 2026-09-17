"""仓库 CI secrets API —— CRUD、校验与权限隔离（值永不回传）。"""

import uuid


def _uuid() -> str:
    return uuid.uuid4().hex[:10]


def _secrets_path(owner: str, repo: str, name: str | None = None) -> str:
    p = f"/users/{owner}/repos/{repo}/secrets"
    return f"{p}/{name}" if name else p


def _make_repo(client, prefix: str = "sec") -> str:
    name = f"{prefix}-{_uuid()}"
    client.post("/repos", json={"name": name}, expect=201)
    return name


def test_repo_secret_crud(user_factory):
    username, _, c = user_factory("sec")
    repo = _make_repo(c)
    try:
        assert c.get(_secrets_path(username, repo), expect=200).json() == []

        secrets = c.put(
            _secrets_path(username, repo),
            json={"name": "TOKEN", "value": "s3cr3t"},
            expect=200,
        ).json()
        assert [s["name"] for s in secrets] == ["TOKEN"]
        # 明文值永不回传
        assert "value" not in secrets[0]

        # 覆盖（upsert）
        c.put(_secrets_path(username, repo), json={"name": "TOKEN", "value": "new"}, expect=200)
        assert [s["name"] for s in c.get(_secrets_path(username, repo), expect=200).json()] == ["TOKEN"]

        # 第二个
        c.put(_secrets_path(username, repo), json={"name": "DB_PASS", "value": "x"}, expect=200)
        assert len(c.get(_secrets_path(username, repo), expect=200).json()) == 2

        # 删除
        c.delete(_secrets_path(username, repo, "TOKEN"), expect=200)
        left = c.get(_secrets_path(username, repo), expect=200).json()
        assert [s["name"] for s in left] == ["DB_PASS"]

        # 删除不存在 → 404
        c.delete(_secrets_path(username, repo, "NOPE"), expect=404)
    finally:
        c.delete(f"/repos/{repo}", expect=204)


def test_repo_secret_invalid_name(user_factory):
    username, _, c = user_factory("sec")
    repo = _make_repo(c)
    try:
        for bad in ("1BAD", "has space", "a-b", ""):
            c.put(_secrets_path(username, repo), json={"name": bad, "value": "x"}, expect=400)
    finally:
        c.delete(f"/repos/{repo}", expect=204)


def test_repo_secret_owner_only(user_factory):
    alice_name, _, alice = user_factory("alice")
    _, _, bob = user_factory("bob")
    repo = _make_repo(alice)
    try:
        alice.put(
            _secrets_path(alice_name, repo),
            json={"name": "TOKEN", "value": "s3cr3t"},
            expect=200,
        )
        # bob 读 / 写 / 删一律 404
        bob.get(_secrets_path(alice_name, repo), expect=404)
        bob.put(_secrets_path(alice_name, repo), json={"name": "X", "value": "1"}, expect=404)
        bob.delete(_secrets_path(alice_name, repo, "TOKEN"), expect=404)
    finally:
        alice.delete(f"/repos/{repo}", expect=204)
