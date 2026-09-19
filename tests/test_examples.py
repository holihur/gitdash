"""端到端跑一遍 examples/packages 的示例脚本（发布 + 消费）。

示例脚本本身是给用户的「可运行文档」；这里把它们作为黑盒用例执行，确保示例
与包注册表 API 不会随迭代漂移。需要实例（GITDASH_BIN / GITDASH_API_URL），
与其它 API 用例一致。
"""
from __future__ import annotations

import os
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent / "examples" / "packages"
PUBLISH = ROOT / "publish.py"
CONSUME = ROOT / "consume" / "consume.py"


def _run(script: Path, env: dict[str, str], timeout: int = 180) -> subprocess.CompletedProcess:
    proc = subprocess.run(
        [sys.executable, str(script)],
        env=env,
        capture_output=True,
        text=True,
        timeout=timeout,
    )
    assert proc.returncode == 0, f"{script.name}:\nstdout:\n{proc.stdout}\nstderr:\n{proc.stderr}"
    return proc


def test_package_examples_publish_and_consume(base_url, user_factory):
    username, _, client = user_factory("ex")
    pat = client.post(
        "/tokens", json={"name": "examples", "scopes": ["repo"]}, expect=201
    ).json()["token"]

    env = {
        **os.environ,
        "GITDASH_URL": base_url,
        "GITDASH_USER": username,
        "GITDASH_PAT": pat,
        "GITDASH_OWNER": username,
    }

    published = _run(PUBLISH, env)
    assert "7/7 registries passed" in published.stdout

    consumed = _run(CONSUME, env)
    assert "7/7 registries consumed" in consumed.stdout
