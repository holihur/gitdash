"""通用写限流：redis 共享计数（多实例共用同一份额度）—— 黑盒测试。

两个独立 gitdash 实例指向同一个 redis，写限额 GITDASH_WRITE_RPM=3：
交替向两个实例发写请求，累计第 4 个必须 429（证明额度跨实例共享）。
未认证请求按客户端 IP 计限，因此两实例无需共享 DB 即可复现同一 key。
"""

import os
import shutil
import socket
import subprocess
import time

import pytest
import requests

from conftest import GITDASH_BIN


def _free_port() -> int:
    with socket.socket() as s:
        s.bind(("127.0.0.1", 0))
        return s.getsockname()[1]


def _wait_tcp(port: int, timeout: float = 5.0) -> bool:
    deadline = time.time() + timeout
    while time.time() < deadline:
        try:
            c = socket.create_connection(("127.0.0.1", port), timeout=1)
            c.close()
            return True
        except OSError:
            time.sleep(0.1)
    return False


def _spawn(redis_port: int, tmp, write_rpm: int):
    http_port, ssh_port = _free_port(), _free_port()
    env = dict(os.environ)
    env.update(
        GITDASH_DATA=str(tmp / f"data-{http_port}"),
        GITDASH_HTTP_ADDR=f"127.0.0.1:{http_port}",
        GITDASH_SSH_ADDR=f"127.0.0.1:{ssh_port}",
        GITDASH_QUEUE="redis",
        GITDASH_REDIS_ADDR=f"127.0.0.1:{redis_port}",
        GITDASH_WRITE_RPM=str(write_rpm),
        GITDASH_PROFILE_REPO="0",
        GITDASH_DISABLE_RATE_LIMIT="",
    )
    log = open(tmp / f"server-{http_port}.log", "wb")
    proc = subprocess.Popen([GITDASH_BIN, "serve"], env=env, stdout=log, stderr=subprocess.STDOUT)
    base = f"http://127.0.0.1:{http_port}"
    deadline = time.time() + 30
    while time.time() < deadline:
        if proc.poll() is not None:
            pytest.fail(f"server exited rc={proc.returncode}")
        try:
            if requests.get(f"{base}/api/health", timeout=1).status_code == 200:
                return base, proc
        except requests.RequestException:
            pass
        time.sleep(0.2)
    pytest.fail("server not ready")


def test_write_ratelimit_shared_across_instances(tmp_path):
    if not GITDASH_BIN:
        pytest.skip("needs GITDASH_BIN")
    redis_bin = shutil.which("redis-server")
    if not redis_bin:
        pytest.skip("redis-server not found")

    rport = _free_port()
    rproc = subprocess.Popen(
        [redis_bin, "--port", str(rport), "--save", "", "--appendonly", "no"],
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
    )
    procs = []
    try:
        if not _wait_tcp(rport):
            pytest.skip("redis not reachable")
        a, pa = _spawn(rport, tmp_path, 3)
        b, pb = _spawn(rport, tmp_path, 3)
        procs = [pa, pb]

        codes = []
        for i in range(5):
            base = a if i % 2 == 0 else b
            # 未认证 POST：限流在鉴权之前生效，按 IP 计限；预期 401,401,401,429...
            resp = requests.post(f"{base}/api/repos", json={"name": f"r{i}"}, timeout=10)
            codes.append(resp.status_code)

        assert codes.count(429) >= 1, f"expected shared limit to trigger, got {codes}"
        # 前 3 个额度被两实例共用；429 之前不应出现第 4 个“非 429”的放行
        first429 = codes.index(429)
        assert first429 == 3, f"limit not shared across instances: {codes}"
    finally:
        for p in procs:
            p.terminate()
            try:
                p.wait(timeout=5)
            except subprocess.TimeoutExpired:
                p.kill()
        rproc.terminate()
