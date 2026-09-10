"""仓库级流水线环境变量 API —— CRUD、校验与权限隔离。"""

import uuid


def _uuid() -> str:
    return uuid.uuid4().hex[:10]


def _env_path(owner: str, repo: str, key: str | None = None) -> str:
    p = f"/users/{owner}/repos/{repo}/env"
    return f"{p}/{key}" if key else p


def _make_repo(client, prefix: str = "env") -> str:
    name = f"{prefix}-{_uuid()}"
    client.post("/repos", json={"name": name}, expect=201)
    return name


def test_repo_env_crud(user_factory):
    username, _, c = user_factory("env")
    repo = _make_repo(c)
    try:
        assert c.get(_env_path(username, repo), expect=200).json() == []

        # 新增
        vars = c.put(
            _env_path(username, repo),
            json={"key": "GOFLAGS", "value": "-mod=mod"},
            expect=200,
        ).json()
        assert len(vars) == 1 and vars[0]["key"] == "GOFLAGS"
        assert vars[0]["value"] == "-mod=mod"

        # 覆盖（upsert）
        vars = c.put(
            _env_path(username, repo),
            json={"key": "GOFLAGS", "value": "-race"},
            expect=200,
        ).json()
        assert len(vars) == 1 and vars[0]["value"] == "-race"

        # 第二个变量
        c.put(_env_path(username, repo), json={"key": "CGO_ENABLED", "value": "0"}, expect=200)
        assert len(c.get(_env_path(username, repo), expect=200).json()) == 2

        # 删除
        c.delete(_env_path(username, repo, "GOFLAGS"), expect=200)
        left = c.get(_env_path(username, repo), expect=200).json()
        assert [v["key"] for v in left] == ["CGO_ENABLED"]

        # 删除不存在 → 404
        c.delete(_env_path(username, repo, "NOPE"), expect=404)
    finally:
        c.delete(f"/repos/{repo}", expect=204)


def test_repo_env_invalid_key(user_factory):
    username, _, c = user_factory("env")
    repo = _make_repo(c)
    try:
        for bad in ("1BAD", "has space", "a-b", ""):
            c.put(
                _env_path(username, repo),
                json={"key": bad, "value": "x"},
                expect=400,
            )
    finally:
        c.delete(f"/repos/{repo}", expect=204)


def test_repo_env_owner_only(user_factory):
    alice_name, _, alice = user_factory("alice")
    _, _, bob = user_factory("bob")
    repo = _make_repo(alice)
    try:
        alice.put(_env_path(alice_name, repo), json={"key": "TOKEN", "value": "s3cr3t"}, expect=200)

        # bob 读 / 写 / 删一律 404
        bob.get(_env_path(alice_name, repo), expect=404)
        bob.put(_env_path(alice_name, repo), json={"key": "X", "value": "1"}, expect=404)
        bob.delete(_env_path(alice_name, repo, "TOKEN"), expect=404)
    finally:
        alice.delete(f"/repos/{repo}", expect=204)
