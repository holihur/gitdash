"""管理端代码搜索指标端点。"""
from __future__ import annotations

import time
import uuid

from conftest import ApiClient


def _uuid() -> str:
    return uuid.uuid4().hex[:10]


def test_admin_codesearch_metrics(admin, base_url, user_factory):
    before = admin.get("/admin/codesearch", expect=200).json()
    assert before["backend"] in ("bleve", "grep", "remote")
    for key in ("search_requests", "index_runs", "index"):
        assert key in before, before
    for key in ("repos", "documents", "dirty"):
        assert key in before["index"], before
    for key in ("full", "incremental", "ok", "error"):
        assert key in before["index_runs"], before

    def _total(r):
        return sum(r["search_requests"].get(k, 0) for k in ("index", "grep", "indexing"))

    before_total = _total(before)

    # 触发一次代码搜索。
    username, token, client = user_factory("csmetrics")
    repo = f"m-{_uuid()[:8]}"
    client.post("/repos", json={"name": repo}, expect=201)
    client.post(f"/users/{username}/repos/{repo}/visibility", json={"private": False}, expect=200)
    client.post(
        f"/users/{username}/repos/{repo}/commits",
        json={"message": "add", "changes": [
            {"path": "a.go", "action": "create", "content": f"package a\n// ADMINMETRIC{uuid.uuid4().hex[:6]}\n"},
        ]},
        expect=201,
    )
    # 用另一个会话轮询搜索，直到索引就绪并返回结果。
    c2 = ApiClient(base_url, token)
    deadline = time.time() + 60
    while time.time() < deadline:
        r = c2.get("/search/code?q=ADMINMETRIC", expect=200).json()
        if r.get("indexed_repos", 0) > 0 and r.get("results"):
            break
        time.sleep(0.3)

    after = admin.get("/admin/codesearch", expect=200).json()
    assert _total(after) > before_total, (before, after)
    assert isinstance(after.get("index_avg_duration_ms"), (int, float))

    client.delete(f"/repos/{repo}", expect=204)
