"""Copilot 闭环黑盒测试。

用一个本地 mock Anthropic Messages 服务当“模型”，真实启动 gitdash + agent：

  浏览器 WS → gitdash → agent(/api/chat) → mock LLM 指示调用 write 工具
  → agent 改工作区 → gitdash 在 done 后 commit+push 到 copilot/session-<id>
  → 分支/文件出现在 gitdash。

因此它验证完整闭环，且不依赖外网/真实 LLM。
"""
from __future__ import annotations

import base64
import json
import os
import shutil
import socket
import ssl
import struct
import threading
import time
import uuid
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from urllib.parse import urlparse

import pytest

E2E_FILE = "COPILOT_E2E.txt"
E2E_CONTENT = "closed loop ok\n"


def _uuid() -> str:
    return uuid.uuid4().hex[:8]


def _agent_bin() -> str | None:
    p = os.environ.get("GITDASH_AGENT_BIN", "").strip()
    if p and Path(p).is_file():
        return p
    g = os.environ.get("GITDASH_BIN", "").strip()
    if g:
        cand = Path(g).resolve().parent / "agent"
        if cand.is_file():
            return str(cand)
    return shutil.which("agent")


# ---- mock Anthropic Messages API（流式 SSE）----

def _sse(handler: BaseHTTPRequestHandler, events: list[dict]) -> None:
    for ev in events:
        handler.wfile.write(f"event: {ev['type']}\ndata: {json.dumps(ev)}\n\n".encode())
        handler.wfile.flush()


class _MockLLM(BaseHTTPRequestHandler):
    def do_POST(self):  # noqa: N802
        length = int(self.headers.get("Content-Length", "0"))
        raw = self.rfile.read(length) if length else b"{}"
        try:
            body = json.loads(raw or b"{}")
        except json.JSONDecodeError:
            body = {}
        has_tool_result = any(
            isinstance(b, dict) and b.get("type") == "tool_result"
            for m in body.get("messages", [])
            for b in m.get("content", [])
        )
        self.send_response(200)
        self.send_header("Content-Type", "text/event-stream")
        self.end_headers()

        if not has_tool_result:
            args = json.dumps({"files": [{"path": E2E_FILE, "content": E2E_CONTENT}]})
            events = [
                {"type": "message_start", "message": {"id": "msg_1", "role": "assistant", "content": []}},
                {"type": "content_block_start", "index": 0,
                 "content_block": {"type": "tool_use", "id": "toolu_1", "name": "write", "input": {}}},
                {"type": "content_block_delta", "index": 0,
                 "delta": {"type": "input_json_delta", "partial_json": args}},
                {"type": "content_block_stop", "index": 0},
                {"type": "message_delta", "delta": {"stop_reason": "tool_use"}},
                {"type": "message_stop"},
            ]
        else:
            events = [
                {"type": "message_start", "message": {"id": "msg_2", "role": "assistant", "content": []}},
                {"type": "content_block_start", "index": 0, "content_block": {"type": "text", "text": ""}},
                {"type": "content_block_delta", "index": 0,
                 "delta": {"type": "text_delta", "text": "Wrote " + E2E_FILE + "."}},
                {"type": "content_block_stop", "index": 0},
                {"type": "message_delta", "delta": {"stop_reason": "end_turn"}},
                {"type": "message_stop"},
            ]
        _sse(self, events)

    def log_message(self, *args):  # silence
        pass


class MockLLMServer:
    def __init__(self):
        self.httpd = ThreadingHTTPServer(("127.0.0.1", 0), _MockLLM)
        self.thread = threading.Thread(target=self.httpd.serve_forever, daemon=True)

    def start(self) -> str:
        self.thread.start()
        return f"http://127.0.0.1:{self.httpd.server_address[1]}"

    def stop(self):
        self.httpd.shutdown()
        self.httpd.server_close()


# ---- 极简 WebSocket 客户端（stdlib，避免新增依赖）----

class WsClient:
    def __init__(self, url: str, headers: dict[str, str] | None = None, timeout: float = 15):
        u = urlparse(url)
        self.host = u.hostname
        self.port = u.port or (443 if u.scheme == "wss" else 80)
        self.path = u.path + (("?" + u.query) if u.query else "")
        key = base64.b64encode(os.urandom(16)).decode()
        req = (
            f"GET {self.path} HTTP/1.1\r\nHost: {u.netloc}\r\n"
            "Upgrade: websocket\r\nConnection: Upgrade\r\n"
            f"Sec-WebSocket-Key: {key}\r\nSec-WebSocket-Version: 13\r\n"
        )
        for k, v in (headers or {}).items():
            req += f"{k}: {v}\r\n"
        req += "\r\n"
        self.sock = socket.create_connection((self.host, self.port), timeout=timeout)
        if u.scheme == "wss":
            self.sock = ssl.create_default_context().wrap_socket(self.sock, server_hostname=self.host)
        self.sock.sendall(req.encode())
        buf = b""
        while b"\r\n\r\n" not in buf:
            chunk = self.sock.recv(4096)
            if not chunk:
                raise RuntimeError("ws handshake: connection closed")
            buf += chunk
        head, self.buf = buf.split(b"\r\n\r\n", 1)
        status = head.split(b"\r\n", 1)[0]
        if b" 101 " not in status + b" ":
            raise RuntimeError(f"ws handshake failed: {head.decode(errors='replace')}")

    def _read(self, n: int) -> bytes:
        while len(self.buf) < n:
            chunk = self.sock.recv(65536)
            if not chunk:
                raise ConnectionError("ws closed")
            self.buf += chunk
        out, self.buf = self.buf[:n], self.buf[n:]
        return out

    def send(self, obj: dict) -> None:
        data = json.dumps(obj).encode()
        mask = os.urandom(4)
        n = len(data)
        if n < 126:
            header = bytes([0x81, 0x80 | n])
        elif n < 65536:
            header = bytes([0x81, 0x80 | 126]) + struct.pack(">H", n)
        else:
            header = bytes([0x81, 0x80 | 127]) + struct.pack(">Q", n)
        masked = bytes(b ^ mask[i % 4] for i, b in enumerate(data))
        self.sock.sendall(header + mask + masked)

    def recv(self) -> str:
        while True:
            b1, b2 = self._read(2)
            opcode = b1 & 0x0F
            length = b2 & 0x7F
            if length == 126:
                length = struct.unpack(">H", self._read(2))[0]
            elif length == 127:
                length = struct.unpack(">Q", self._read(8))[0]
            masked = b2 & 0x80
            mask = self._read(4) if masked else None
            payload = self._read(length)
            if mask:
                payload = bytes(b ^ mask[i % 4] for i, b in enumerate(payload))
            if opcode == 0x1:
                return payload.decode()
            if opcode == 0x8:
                raise ConnectionError("ws closed by server")

    def close(self):
        try:
            self.sock.sendall(bytes([0x88, 0x80]) + os.urandom(4))
        except OSError:
            pass
        self.sock.close()


def _recv_until(ws: WsClient, events: list[dict], pred, timeout: float) -> None:
    deadline = time.time() + timeout
    while time.time() < deadline:
        try:
            frame = ws.recv()
        except (ConnectionError, OSError) as e:
            raise AssertionError(f"ws closed early: {e}; events={events}") from e
        try:
            ev = json.loads(frame)
        except json.JSONDecodeError:
            continue
        events.append(ev)
        if pred(events):
            return
    raise AssertionError(f"timeout waiting for events; got={events}")


def test_copilot_closed_loop(base_url, user_factory):
    if not _agent_bin():
        pytest.skip("copilot agent binary not found (set GITDASH_AGENT_BIN)")

    mock = MockLLMServer()
    mock_base = mock.start()
    owner = None
    repo = None
    client = None
    try:
        owner, token, client = user_factory("cp")
        repo = f"cprepo-{_uuid()}"
        client.post("/repos", json={"name": repo}, expect=201)
        # 初始提交，确保有 main 分支可克隆
        client.post(
            f"/users/{owner}/repos/{repo}/commits",
            json={"message": "init", "changes": [{"path": "README.md", "action": "create", "content": "# repo\n"}]},
            expect=201,
        )

        byok = client.post(
            "/me/byok",
            json={
                "name": "mock",
                "provider": "anthropic",
                "api_key": "sk-mock",
                "base_url": mock_base,
                "model": "mock-1",
            },
            expect=201,
        ).json()
        session = client.post(
            f"/users/{owner}/repos/{repo}/copilots",
            json={"byok_id": byok["id"]},
            expect=201,
        ).json()
        sid = session["id"]
        branch = f"copilot/session-{sid}"

        ws_url = base_url.replace("http://", "ws://", 1) + (
            f"/api/users/{owner}/repos/{repo}/copilots/{sid}/chat"
        )
        ws = WsClient(ws_url, headers={"Authorization": f"Bearer {token}"})
        try:
            events: list[dict] = []
            # 连接后：history + status(idle)
            _recv_until(ws, events, lambda evs: any(e.get("type") == "status" for e in evs), timeout=20)
            ws.send({"type": "user", "text": "create the file"})
            _recv_until(ws, events, lambda evs: any(e.get("type") == "done" for e in evs), timeout=90)
        finally:
            ws.close()

        types = [e["type"] for e in events]
        assert "tool_start" in types, types
        assert "tool_end" in types, types
        assert "done" in types, types

        # 闭环：分支已推送、文件存在、内容正确
        branches = client.get(f"/users/{owner}/repos/{repo}/branches", expect=200).json()
        assert any(b["name"] == branch for b in branches), branches
        blob = client.get(
            f"/users/{owner}/repos/{repo}/blob",
            params={"ref": branch, "path": E2E_FILE},
            expect=200,
        ).json()
        assert blob["content"] == E2E_CONTENT, blob

        # gitdash 记录了分支与提交
        detail = client.get(f"/users/{owner}/repos/{repo}/copilots/{sid}", expect=200).json()
        assert detail["branch"] == branch
        assert detail["head_sha"]

        # 对话历史经 agent 持久化并可回放
        msgs = client.get(f"/users/{owner}/repos/{repo}/copilots/{sid}/messages", expect=200).json()
        assert any(m["role"] == "user" for m in msgs), msgs
        assert any(m["role"] == "assistant" and m.get("tools") for m in msgs), msgs

        client.delete(f"/users/{owner}/repos/{repo}/copilots/{sid}", expect=200)
    finally:
        mock.stop()
        if client and owner and repo:
            try:
                client.delete(f"/repos/{repo}", expect=204)
            except Exception:
                pass


def test_copilot_issue_auto_pr(base_url, user_factory):
    """关联 issue 的会话在闭环后自动开 PR（正文 Closes #N）。"""
    if not _agent_bin():
        pytest.skip("copilot agent binary not found (set GITDASH_AGENT_BIN)")

    mock = MockLLMServer()
    mock_base = mock.start()
    owner = repo = client = None
    try:
        owner, token, client = user_factory("cpi")
        repo = f"cprepo-{_uuid()}"
        client.post("/repos", json={"name": repo}, expect=201)
        client.post(
            f"/users/{owner}/repos/{repo}/commits",
            json={"message": "init", "changes": [{"path": "README.md", "action": "create", "content": "# repo\n"}]},
            expect=201,
        )
        issue = client.post(
            f"/users/{owner}/repos/{repo}/issues",
            json={"title": "crash on startup", "body": "please fix the startup crash"},
            expect=201,
        ).json()
        issue_number = issue["number"]

        byok = client.post(
            "/me/byok",
            json={"name": "mock", "provider": "anthropic", "api_key": "sk-mock", "base_url": mock_base, "model": "mock-1"},
            expect=201,
        ).json()
        session = client.post(
            f"/users/{owner}/repos/{repo}/copilots",
            json={"byok_id": byok["id"], "issue_number": issue_number},
            expect=201,
        ).json()
        sid = session["id"]
        branch = f"copilot/session-{sid}"
        assert session["issue_number"] == issue_number
        assert "crash on startup" in session["prompt"]

        ws_url = base_url.replace("http://", "ws://", 1) + (
            f"/api/users/{owner}/repos/{repo}/copilots/{sid}/chat"
        )
        ws = WsClient(ws_url, headers={"Authorization": f"Bearer {token}"})
        try:
            events: list[dict] = []
            _recv_until(ws, events, lambda evs: any(e.get("type") == "status" for e in evs), timeout=20)
            ws.send({"type": "user", "text": "fix the crash"})
            _recv_until(ws, events, lambda evs: any(e.get("type") == "done" for e in evs), timeout=90)
        finally:
            ws.close()

        detail = client.get(f"/users/{owner}/repos/{repo}/copilots/{sid}", expect=200).json()
        assert detail["pr_number"], detail
        pr = client.get(f"/users/{owner}/repos/{repo}/pulls/{detail['pr_number']}", expect=200).json()
        assert pr["source_branch"] == branch, pr
        assert f"Closes #{issue_number}" in pr["body"], pr

        client.delete(f"/users/{owner}/repos/{repo}/copilots/{sid}", expect=200)
    finally:
        mock.stop()
        if client and owner and repo:
            try:
                client.delete(f"/repos/{repo}", expect=204)
            except Exception:
                pass


def _read_test_key() -> tuple[str, str, str] | None:
    """从仓库根目录 assets.md 读取真实测试密钥；不存在返回 None。"""
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
    return (key, base, model) if key else None


def test_copilot_closed_loop_real_key(base_url, user_factory):
    """可选：用 assets.md 的真实密钥跑一次完整闭环（CI 无该文件会自动跳过）。"""
    if not _agent_bin():
        pytest.skip("copilot agent binary not found (set GITDASH_AGENT_BIN)")
    creds = _read_test_key()
    if not creds:
        pytest.skip("no BYOK test key in assets.md; skipping")
    api_key, llm_base, llm_model = creds

    owner, token, client = user_factory("cpr")
    repo = f"cprepo-{_uuid()}"
    client.post("/repos", json={"name": repo}, expect=201)
    client.post(
        f"/users/{owner}/repos/{repo}/commits",
        json={"message": "init", "changes": [{"path": "README.md", "action": "create", "content": "# repo\n"}]},
        expect=201,
    )
    byok = client.post(
        "/me/byok",
        json={"name": "real", "provider": "anthropic", "api_key": api_key, "base_url": llm_base, "model": llm_model},
        expect=201,
    ).json()
    session = client.post(
        f"/users/{owner}/repos/{repo}/copilots",
        json={"byok_id": byok["id"]},
        expect=201,
    ).json()
    sid = session["id"]
    branch = f"copilot/session-{sid}"
    try:
        ws_url = base_url.replace("http://", "ws://", 1) + f"/api/users/{owner}/repos/{repo}/copilots/{sid}/chat"
        ws = WsClient(ws_url, headers={"Authorization": f"Bearer {token}"})
        try:
            events: list[dict] = []
            _recv_until(ws, events, lambda evs: any(e.get("type") == "status" for e in evs), timeout=20)
            ws.send({
                "type": "user",
                "text": f"Create a file named {E2E_FILE} containing exactly the line: closed loop ok. "
                        "Use the write tool. Then reply done.",
            })
            _recv_until(ws, events, lambda evs: any(e.get("type") == "done" for e in evs), timeout=180)
        finally:
            ws.close()

        branches = client.get(f"/users/{owner}/repos/{repo}/branches", expect=200).json()
        assert any(b["name"] == branch for b in branches), branches
        detail = client.get(f"/users/{owner}/repos/{repo}/copilots/{sid}", expect=200).json()
        assert detail["head_sha"]
        blob = client.get(
            f"/users/{owner}/repos/{repo}/blob", params={"ref": branch, "path": E2E_FILE}, expect=200
        ).json()
        assert E2E_CONTENT.strip() in blob["content"], blob
    finally:
        try:
            client.delete(f"/users/{owner}/repos/{repo}/copilots/{sid}", expect=200)
        except Exception:
            pass
        try:
            client.delete(f"/repos/{repo}", expect=204)
        except Exception:
            pass
