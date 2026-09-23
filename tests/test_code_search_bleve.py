"""代码搜索（嵌入式 Bleve 索引）黑盒测试：端到端 + 私有仓库不泄漏。

单独拉起一个 `GITDASH_CODE_SEARCH=bleve` 的实例，验证：
- push 后索引被异步构建，探索页代码搜索实际走索引（indexed_repos > 0）；
- 私有仓库内容不会因索引跨用户泄漏；
- 私有仓库被删除后用同名重建（可见性改变）时，陈旧索引内容不会被返回。
"""
from __future__ import annotations

import subprocess
import time
import uuid
from pathlib import Path

import pytest

from conftest import GITDASH_BIN, ApiClient, _spawn_server


def _uuid() -> str:
    return uuid.uuid4().hex[:10]


def _p(owner, repo, suffix=""):
    return f"/users/{owner}/repos/{repo}{suffix}"


def _write(client, owner, repo, path, content):
    client.post(
        _p(owner, repo, "/commits"),
        json={"message": "add", "changes": [{"path": path, "action": "create", "content": content}]},
        expect=201,
    )


def _search(client, q, expect=200):
    from urllib.parse import urlencode

    return client.get(f"/search/code?{urlencode({'q': q})}", expect=expect).json()


@pytest.fixture(scope="module")
def bleve_base(tmp_path_factory):
    if not GITDASH_BIN:
        pytest.skip("bleve code-search tests need a self-spawned instance (GITDASH_BIN)")
    binary = Path(GITDASH_BIN)
    if not binary.is_file():
        pytest.fail(f"GITDASH_BIN is not a file: {binary}")
    base, proc = _spawn_server(
        binary,
        tmp_path_factory.mktemp("gitdash-bleve"),
        extra_env={"GITDASH_CODE_SEARCH": "bleve"},
        register_ssh=False,
    )
    try:
        yield base
    finally:
        proc.terminate()
        try:
            proc.wait(timeout=10)
        except subprocess.TimeoutExpired:
            proc.kill()


def _new_user(base: str, prefix: str) -> tuple[str, ApiClient]:
    c = ApiClient(base)
    name = f"{prefix}-{_uuid()}"
    c.token = c.post(
        "/auth/register", json={"username": name, "password": "test-pass-123456"}, expect=201
    ).json()["token"]
    return name, c


def _wait_indexed(client, q: str, timeout: float = 60.0) -> dict:
    """轮询直到搜索实际命中索引（indexed_repos > 0）且返回结果。"""
    deadline = time.time() + timeout
    last = {}
    while time.time() < deadline:
        last = _search(client, q)
        if last.get("indexed_repos", 0) > 0 and last.get("results"):
            return last
        time.sleep(0.5)
    pytest.fail(f"index did not become ready for query {q!r}: {last}")


def test_bleve_index_serves_public_code(bleve_base):
    alice, alice_c = _new_user(bleve_base, "alice")
    bob, bob_c = _new_user(bleve_base, "bob")
    repo = f"pub-{_uuid()[:8]}"
    token = f"PUBLICTOKEN{uuid.uuid4().hex[:8]}"
    alice_c.post("/repos", json={"name": repo}, expect=201)
    alice_c.post(_p(alice, repo, "/visibility"), json={"private": False}, expect=200)
    _write(alice_c, alice, repo, "src/main.go", f"package main\n// {token} here\n")

    r = _wait_indexed(bob_c, token)
    assert r["indexed_repos"] >= 1, r
    assert {h["repo"] for h in r["results"]} == {repo}
    assert all(h["text"] for h in r["results"])

    alice_c.delete(f"/repos/{repo}", expect=204)


def test_bleve_does_not_leak_private_repo(bleve_base):
    alice, alice_c = _new_user(bleve_base, "alice")
    bob, bob_c = _new_user(bleve_base, "bob")
    repo = f"priv-{_uuid()[:8]}"
    token = f"SECRETTOKEN{uuid.uuid4().hex[:8]}"
    alice_c.post("/repos", json={"name": repo}, expect=201)  # 默认私有
    _write(alice_c, alice, repo, "secret.go", f"package secret\n// {token}\n")

    # alice 自己能通过索引搜到（证明该私有仓库确实已建索引）。
    r = _wait_indexed(alice_c, token)
    assert {h["repo"] for h in r["results"]} == {repo}

    # bob 搜不到任何内容，且候选仓库里不含该私有仓库。
    rb = _search(bob_c, token)
    assert rb["results"] == [], rb

    alice_c.delete(f"/repos/{repo}", expect=204)


def test_bleve_incremental_update(bleve_base):
    """增量重建：更新/新增/删除文件后，旧内容消失、新内容可搜。"""
    alice, alice_c = _new_user(bleve_base, "alice")
    repo = f"inc-{_uuid()[:8]}"
    keep = f"KEEP{uuid.uuid4().hex[:6]}"
    old = f"OLD{uuid.uuid4().hex[:6]}"
    new = f"NEW{uuid.uuid4().hex[:6]}"
    added = f"ADD{uuid.uuid4().hex[:6]}"
    removed = f"REM{uuid.uuid4().hex[:6]}"
    alice_c.post("/repos", json={"name": repo}, expect=201)
    alice_c.post(_p(alice, repo, "/visibility"), json={"private": False}, expect=200)
    alice_c.post(
        _p(alice, repo, "/commits"),
        json={"message": "init", "changes": [
            {"path": "keep.go", "action": "create", "content": f"package keep\n// {keep}\n"},
            {"path": "a.go", "action": "create", "content": f"package a\n// {old}\n"},
            {"path": "gone.go", "action": "create", "content": f"package gone\n// {removed}\n"},
        ]},
        expect=201,
    )
    for tok in (keep, old, removed):
        _wait_indexed(alice_c, tok)

    alice_c.post(
        _p(alice, repo, "/commits"),
        json={"message": "update", "changes": [
            {"path": "a.go", "action": "update", "content": f"package a\n// {new}\n"},
            {"path": "gone.go", "action": "delete"},
            {"path": "added.go", "action": "create", "content": f"package added\n// {added}\n"},
        ]},
        expect=201,
    )
    _wait_indexed(alice_c, new)

    assert _search(alice_c, keep)["results"], "unchanged file lost"
    assert _search(alice_c, new)["results"], "updated content missing"
    assert _search(alice_c, added)["results"], "added file missing"
    assert _search(alice_c, old)["results"] == [], "stale content still indexed"
    assert _search(alice_c, removed)["results"] == [], "deleted file still indexed"

    alice_c.delete(f"/repos/{repo}", expect=204)


def test_bleve_cjk_search(bleve_base):
    """中文/日文/韓文代码注释可搜索（端到端）。"""
    alice, alice_c = _new_user(bleve_base, "alice")
    repo = f"cjk-{_uuid()[:8]}"
    zh = "中文代码搜索功能"
    ja = "日本語の検索"
    alice_c.post("/repos", json={"name": repo}, expect=201)
    alice_c.post(_p(alice, repo, "/visibility"), json={"private": False}, expect=200)
    alice_c.post(
        _p(alice, repo, "/commits"),
        json={"message": "add", "changes": [
            {"path": "a.go", "action": "create", "content": f"package a\n// {zh}\n// {ja}\n"},
        ]},
        expect=201,
    )

    r = _wait_indexed(alice_c, zh)
    assert r["results"], r
    assert any(zh in h["text"] for h in r["results"]), r

    r = _wait_indexed(alice_c, "検索")
    assert r["results"], r

    alice_c.delete(f"/repos/{repo}", expect=204)


def test_metrics_do_not_leak_repo(bleve_base):
    """公开的 /metrics 不得包含仓库名或代码内容（无 per-repo 标签）。"""
    import requests

    alice, alice_c = _new_user(bleve_base, "alice")
    repo = f"metrics-{_uuid()[:8]}"
    token = f"METRICTOKEN{uuid.uuid4().hex[:8]}"
    alice_c.post("/repos", json={"name": repo}, expect=201)
    alice_c.post(_p(alice, repo, "/visibility"), json={"private": False}, expect=200)
    _write(alice_c, alice, repo, "a.go", f"package a\n// {token}\n")
    _wait_indexed(alice_c, token)

    text = requests.get(f"{bleve_base}/metrics", timeout=10).text
    assert 'gitdash_code_search_requests_total{source="index"}' in text
    assert repo not in text, "repo name leaked into public metrics"
    assert token not in text, "code content leaked into public metrics"

    alice_c.delete(f"/repos/{repo}", expect=204)


def test_bleve_no_stale_leak_after_delete_and_recreate(bleve_base):
    """私有仓库删除后用同名（公开、无内容）重建：旧私有内容不得被返回。"""
    alice, alice_c = _new_user(bleve_base, "alice")
    bob, bob_c = _new_user(bleve_base, "bob")
    repo = f"reuse-{_uuid()[:8]}"
    token = f"STALETOKEN{uuid.uuid4().hex[:8]}"

    # 私有仓库 + 敏感内容并等待索引完成。
    alice_c.post("/repos", json={"name": repo}, expect=201)
    _write(alice_c, alice, repo, "secret.go", f"package secret\n// {token}\n")
    _wait_indexed(alice_c, token)

    # 删除后以同名重建为公开、空仓库（不写内容，避免立即重建索引）。
    alice_c.delete(f"/repos/{repo}", expect=204)
    alice_c.post("/repos", json={"name": repo}, expect=201)
    alice_c.post(_p(alice, repo, "/visibility"), json={"private": False}, expect=200)

    # bob 能在探索页看到这个公开仓库，但绝不能拿到已被删除的旧私有内容。
    rb = _search(bob_c, token)
    assert rb["results"] == [], f"stale private content leaked: {rb}"

    alice_c.delete(f"/repos/{repo}", expect=204)
