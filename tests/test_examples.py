"""端到端跑一遍 examples/packages 的示例发布脚本。

示例脚本本身是给用户的「可运行文档」；这里把它作为黑盒用例执行，确保示例
与包注册表 API 不会随迭代漂移。需要实例（GITDASH_BIN / GITDASH_API_URL），
与其它 API 用例一致。
"""
from __future__ import annotations

import os
import subprocess
import sys
from pathlib import Path

SCRIPT = Path(__file__).resolve().parent.parent / "examples" / "packages" / "publish.py"


def test_package_examples_publish_and_verify(base_url, user_factory):
    username, _, client = user_factory("ex")
    pat = client.post(
        "/tokens", json={"name": "examples", "scopes": ["repo"]}, expect=201
    ).json()["token"]

    env = {
        **os.environ,
        "GITDASH_URL": base_url,
        "GITDASH_USER": username,
        "GITDASH_PAT": pat,
    }
    proc = subprocess.run(
        [sys.executable, str(SCRIPT)],
        env=env,
        capture_output=True,
        text=True,
        timeout=180,
    )
    assert proc.returncode == 0, f"stdout:\n{proc.stdout}\nstderr:\n{proc.stderr}"
    assert "7/7 registries passed" in proc.stdout
