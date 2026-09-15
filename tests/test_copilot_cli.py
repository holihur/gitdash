"""gitdash-cli copilot 黑盒测试（可选）。

用本地 mock Anthropic 服务 + 真实 agent 运行时，跑：

    gitdash-cli copilot fix <owner/repo> <issue>

并断言 CLI 创建会话、驱动 agent、最终打印自动开出的 PR。

需要 GITDASH_CLI_BIN 指向 gitdash-cli 可执行文件；未设置则跳过。
"""
from __future__ import annotations

import os
import subprocess
import uuid
from pathlib import Path

import pytest

from test_copilot import E2E_FILE, MockLLMServer, _agent_bin


def _cli_bin() -> str | None:
    p = os.environ.get("GITDASH_CLI_BIN", "").strip()
    if p and Path(p).is_file():
        return p
    return None


@pytest.fixture
def cli_env(tmp_path):
    env = dict(os.environ)
    env["GITDASH_CONFIG_DIR"] = str(tmp_path / "config")
    return env


def test_copilot_cli_fix_issue(base_url, user_factory, cli_env):
    cli = _cli_bin()
    if not cli:
        pytest.skip("gitdash-cli binary not found (set GITDASH_CLI_BIN)")
    if not _agent_bin():
        pytest.skip("copilot agent binary not found (set GITDASH_AGENT_BIN)")

    mock = MockLLMServer()
    mock_base = mock.start()
    owner = repo = client = None
    try:
        owner, token, client = user_factory("cli")
        repo = f"clirepo-{uuid.uuid4().hex[:8]}"
        client.post("/repos", json={"name": repo}, expect=201)
        client.post(
            f"/users/{owner}/repos/{repo}/commits",
            json={"message": "init", "changes": [{"path": "README.md", "action": "create", "content": "# repo\n"}]},
            expect=201,
        )
        issue = client.post(
            f"/users/{owner}/repos/{repo}/issues",
            json={"title": "write the e2e file", "body": "please create it"},
            expect=201,
        ).json()
        client.post(
            "/me/byok",
            json={"name": "mock", "provider": "anthropic", "api_key": "sk-mock", "base_url": mock_base, "model": "mock-1"},
            expect=201,
        )

        proc = subprocess.run(
            [
                cli, "--host", base_url, "--token", token,
                "copilot", "fix", f"{owner}/{repo}", str(issue["number"]),
            ],
            env=cli_env,
            capture_output=True,
            text=True,
            timeout=180,
        )
        assert proc.returncode == 0, f"stdout={proc.stdout}\nstderr={proc.stderr}"
        assert "Opened pull request #" in proc.stdout, proc.stdout

        pulls = client.get(f"/users/{owner}/repos/{repo}/pulls", expect=200).json()
        assert len(pulls) == 1, pulls
        assert f"Closes #{issue['number']}" in pulls[0]["body"], pulls[0]
        assert pulls[0]["source_branch"].startswith("copilot/session-")

        # 闭环文件确实在会话分支上
        blob = client.get(
            f"/users/{owner}/repos/{repo}/blob",
            params={"ref": pulls[0]["source_branch"], "path": E2E_FILE},
            expect=200,
        ).json()
        assert "closed loop ok" in blob["content"], blob
    finally:
        mock.stop()
        if client and owner and repo:
            try:
                client.delete(f"/repos/{repo}", expect=204)
            except Exception:
                pass
