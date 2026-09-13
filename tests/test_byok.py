"""BYOK + copilot 会话黑盒 API 测试。

覆盖：
- BYOK 密钥 CRUD（明文 key 永不回传、留空更新保留原 key、用户隔离）
- copilot 会话：缺失/无效 byok 拒绝；有效 byok 创建会话（无 docker 时终态 failed）；
  列表 / 详情 / 删除

与后端完全隔离，仅通过 HTTP API 验证。
"""
from __future__ import annotations

import uuid
from pathlib import Path

import pytest


def _uuid() -> str:
    return uuid.uuid4().hex[:8]


def _read_test_key() -> tuple[str, str, str] | None:
    """从仓库根目录 assets.md 读取测试密钥；不存在返回 None。"""
    p = Path(__file__).resolve().parent.parent / "assets.md"
    if not p.is_file():
        return None
    key = base = model = ""
    for line in p.read_text().splitlines():
        line = line.strip()
        if line.startswith("LLM_APIKEY="):
            key = line.split("=", 1)[1].strip().strip('"\'')
        elif line.startswith("LLM_BASE_URL="):
            base = line.split("=", 1)[1].strip().strip('"\'')
        elif line.startswith("LLM_MODEL="):
            model = line.split("=", 1)[1].strip().strip('"\'')
    if not key:
        return None
    return key, base, model


@pytest.fixture
def env(user_factory):
    owner, token, client = user_factory("by")
    repo = f"byrepo-{_uuid()}"
    client.post("/repos", json={"name": repo}, expect=201)
    yield owner, client, repo
    try:
        client.delete(f"/repos/{repo}", expect=204)
    except Exception:
        pass


def test_byok_crud_and_key_never_leaks(user_factory):
    _, _, c = user_factory("bk")
    key = c.post(
        "/me/byok",
        json={
            "name": "work",
            "provider": "anthropic",
            "api_key": "sk-ant-test-123",
            "base_url": "https://api.anthropic.com",
            "model": "claude-sonnet-4-5",
        },
        expect=201,
    ).json()
    assert key["key_set"] is True
    assert "api_key" not in key
    assert key["name"] == "work"

    # 列表同样不含明文
    keys = c.get("/me/byok", expect=200).json()
    assert len(keys) == 1
    assert "api_key" not in keys[0]

    # 更新：api_key 留空保留原 key
    updated = c.put(
        f"/me/byok/{key['id']}",
        json={"name": "work-2", "provider": "anthropic", "model": "claude-3-5"},
        expect=200,
    ).json()
    assert updated["name"] == "work-2"
    assert updated["model"] == "claude-3-5"
    assert updated["key_set"] is True

    # 仅支持 anthropic 类型
    c.post("/me/byok", json={"name": "x", "provider": "openai", "api_key": "sk-x"}, expect=400)

    c.delete(f"/me/byok/{key['id']}", expect=200)
    assert c.get("/me/byok", expect=200).json() == []


def test_byok_user_isolation(user_factory):
    _, _, alice = user_factory("bka")
    _, _, bob = user_factory("bkb")
    key = alice.post("/me/byok", json={"name": "a", "provider": "anthropic", "api_key": "sk-a"}, expect=201).json()
    # 别人拿不到
    assert bob.get("/me/byok", expect=200).json() == []
    bob.put(f"/me/byok/{key['id']}", json={"name": "hijack", "provider": "anthropic"}, expect=404)
    bob.delete(f"/me/byok/{key['id']}", expect=404)


def test_copilot_session_lifecycle(env):
    owner, client, repo = env

    # 无效 byok 拒绝
    client.post(
        f"/users/{owner}/repos/{repo}/copilots",
        json={"byok_id": 999999},
        expect=400,
    )

    byok = client.post(
        "/me/byok",
        json={"name": "k", "provider": "anthropic", "api_key": "sk-ant-test"},
        expect=201,
    ).json()

    created = client.post(
        f"/users/{owner}/repos/{repo}/copilots",
        json={"byok_id": byok["id"], "image": "alpine:3.19", "prompt": "hello"},
        expect=201,
    ).json()
    assert created["id"] > 0
    assert created["created_by"] == owner
    assert created["status"] in ("created", "running", "failed")

    listed = client.get(f"/users/{owner}/repos/{repo}/copilots", expect=200).json()
    assert any(s["id"] == created["id"] for s in listed)

    detail = client.get(f"/users/{owner}/repos/{repo}/copilots/{created['id']}", expect=200).json()
    assert detail["id"] == created["id"]

    # 停止 / 删除（无论容器是否存在，均应成功）
    client.post(f"/users/{owner}/repos/{repo}/copilots/{created['id']}/stop", expect=200)
    client.delete(f"/users/{owner}/repos/{repo}/copilots/{created['id']}", expect=200)
    assert client.get(f"/users/{owner}/repos/{repo}/copilots", expect=200).json() == []


def _commit(c, owner, repo, path, content):
    c.post(
        f"/users/{owner}/repos/{repo}/commits",
        json={"message": f"add {path}", "changes": [{"path": path, "action": "create", "content": content}]},
        expect=201,
    )


def test_copilot_session_with_real_key(user_factory):
    """有测试密钥 + docker 时：真实启动容器并校验注入的 LLM 环境变量。"""
    test_key = _read_test_key()
    if not test_key:
        pytest.skip("no BYOK test key in assets.md; skipping")
    api_key, base_url, model = test_key

    owner, _token, client = user_factory("byk")
    repo = f"byrepo-{_uuid()}"
    client.post("/repos", json={"name": repo, "private": False}, expect=201)
    _commit(client, owner, repo, "README.md", f"# {repo}\n")

    byok = client.post(
        "/me/byok",
        json={"name": "test", "provider": "anthropic", "api_key": api_key, "base_url": base_url, "model": model},
        expect=201,
    ).json()

    session = client.post(
        f"/users/{owner}/repos/{repo}/copilots",
        json={
            "byok_id": byok["id"],
            "image": "alpine:3.19",
            "prompt": "echo injected env",
            "command": "echo base=$LLM_BASE_URL; echo model=$LLM_MODEL; echo keylen=${#LLM_APIKEY}; ls /workspace",
        },
        expect=201,
    ).json()

    # 容器 echo 完即退出；无 docker 时终态为 failed，视为环境不具备真实执行条件
    import time
    deadline = time.time() + 60
    detail = None
    while time.time() < deadline:
        detail = client.get(f"/users/{owner}/repos/{repo}/copilots/{session['id']}", expect=200).json()
        if detail["status"] in ("failed", "stopped"):
            break
        time.sleep(1)

    if detail["status"] == "failed" and "docker" in (detail.get("error") or "").lower():
        pytest.skip("docker not available in this environment")

    logs = detail.get("log") or ""
    assert base_url in logs, logs
    assert model in logs, logs
    assert f"keylen={len(api_key)}" in logs, logs
    assert "README.md" in logs, logs

    client.delete(f"/users/{owner}/repos/{repo}/copilots/{session['id']}", expect=200)
    client.delete(f"/repos/{repo}", expect=204)
