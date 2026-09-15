"""BYOK + copilot 会话黑盒 API 测试。

覆盖：
- BYOK 密钥 CRUD（明文 key 永不回传、留空更新保留原 key、用户隔离）
- copilot 会话：缺失/无效 byok 拒绝；有效 byok 创建会话（无 docker 时终态 failed）；
  列表 / 详情 / 删除

与后端完全隔离，仅通过 HTTP API 验证。
"""
from __future__ import annotations

import uuid
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from threading import Thread

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


class _MockMessages(BaseHTTPRequestHandler):
    """最小 Anthropic Messages 端点，用于测试 BYOK 连接检测。"""

    status = 200

    def do_POST(self):  # noqa: N802
        length = int(self.headers.get("Content-Length", "0"))
        self.rfile.read(length) if length else None
        self.send_response(self.status)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        self.wfile.write(b'{"content": []}')

    def log_message(self, *args):
        pass


@pytest.fixture
def mock_llm():
    httpd = ThreadingHTTPServer(("127.0.0.1", 0), _MockMessages)
    t = Thread(target=httpd.serve_forever, daemon=True)
    t.start()
    url = f"http://127.0.0.1:{httpd.server_address[1]}"
    yield url, httpd
    httpd.shutdown()
    httpd.server_close()


def test_byok_providers_and_key_requirement(user_factory):
    _, _, c = user_factory("bp")

    # ollama：无需密钥（服务端补占位密钥），key_set 仍为 True
    local = c.post("/me/byok", json={"name": "local", "provider": "ollama"}, expect=201).json()
    assert local["provider"] == "ollama"
    assert local["key_set"] is True

    # compatible：需要密钥
    c.post("/me/byok", json={"name": "gw", "provider": "compatible"}, expect=400)
    gw = c.post(
        "/me/byok",
        json={"name": "gw", "provider": "compatible", "api_key": "sk-x", "base_url": "https://gw.example.com"},
        expect=201,
    ).json()
    assert gw["provider"] == "compatible"

    # 未支持的 provider 仍拒绝
    c.post("/me/byok", json={"name": "x", "provider": "openai", "api_key": "sk"}, expect=400)


def test_byok_test_connection(user_factory, mock_llm):
    _, _, c = user_factory("bt")
    url, httpd = mock_llm

    # 连通：本地 mock 端点 + 显式 model
    ok = c.post(
        "/me/byok/test",
        json={"provider": "compatible", "base_url": url, "model": "m", "api_key": "sk-x"},
        expect=200,
    ).json()
    assert ok["ok"] is True

    # 已保存密钥 + api_key 留空 → 回退到 id
    saved = c.post(
        "/me/byok",
        json={"name": "k", "provider": "compatible", "api_key": "sk-x", "base_url": url, "model": "m"},
        expect=201,
    ).json()
    ok2 = c.post("/me/byok/test", json={"id": saved["id"], "provider": "compatible"}, expect=200).json()
    assert ok2["ok"] is True

    # 认证失败 → ok=false，而不是 5xx
    _MockMessages.status = 401
    bad = c.post(
        "/me/byok/test",
        json={"provider": "compatible", "base_url": url, "model": "m", "api_key": "sk-bad"},
        expect=200,
    ).json()
    _MockMessages.status = 200
    assert bad["ok"] is False
    assert "401" in bad["error"]


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
        json={"byok_id": byok["id"], "prompt": "hello"},
        expect=201,
    ).json()
    assert created["id"] > 0
    assert created["created_by"] == owner
    assert created["status"] == "idle"
    assert created["branch"] == f"copilot/session-{created['id']}"

    listed = client.get(f"/users/{owner}/repos/{repo}/copilots", expect=200).json()
    assert any(s["id"] == created["id"] for s in listed)

    detail = client.get(f"/users/{owner}/repos/{repo}/copilots/{created['id']}", expect=200).json()
    assert detail["id"] == created["id"]

    # 新会话历史为空
    assert client.get(f"/users/{owner}/repos/{repo}/copilots/{created['id']}/messages", expect=200).json() == []

    # 停止（取消当前轮次；无运行轮次也应成功）；删除会清掉工作区与记录
    client.post(f"/users/{owner}/repos/{repo}/copilots/{created['id']}/stop", expect=200)
    client.delete(f"/users/{owner}/repos/{repo}/copilots/{created['id']}", expect=200)
    assert client.get(f"/users/{owner}/repos/{repo}/copilots", expect=200).json() == []
