"""仓库标签（topics / labels）与 Explore 按标签搜索 —— 黑盒测试。"""
from __future__ import annotations

import uuid


def _user_repo(user_factory, private: bool = False):
    username, _token, client = user_factory("top")
    name = f"r-{uuid.uuid4().hex[:8]}"
    client.post("/repos", json={"name": name, "private": private}, expect=201)
    return username, client, name


def test_repo_topics_crud_and_explore_filter(user_factory):
    username, client, name = _user_repo(user_factory, private=False)
    base = f"/users/{username}/repos/{name}"

    # 设置（大小写归一 + 去重）
    r = client.put(f"{base}/topics", json={"topics": ["Go", "web", "go"]}, expect=200)
    assert r.json()["topics"] == ["go", "web"]

    # 仓库详情带 topics
    detail = client.get(base, expect=200).json()
    assert sorted(detail["topics"]) == ["go", "web"]

    # 仓库列表带 topics
    lst = client.get("/repos", expect=200).json()
    mine = [x for x in lst if x["owner"] == username and x["name"] == name]
    assert mine and sorted(mine[0]["topics"]) == ["go", "web"]

    # 全局标签列表（含使用数）
    topics = client.get("/topics", expect=200).json()
    by = {t["topic"]: t["count"] for t in topics}
    assert by.get("go", 0) >= 1

    # Explore 按标签过滤
    ex = client.get("/explore/repos?topic=go", expect=200).json()
    assert any(x["owner"] == username and x["name"] == name for x in ex)

    # 关键词过滤（命中描述/名称）
    exq = client.get("/explore/repos?q=r-", expect=200).json()
    assert isinstance(exq, list)

    # 替换标签后旧标签不再命中
    client.put(f"{base}/topics", json={"topics": ["cli"]}, expect=200)
    assert client.get(base, expect=200).json()["topics"] == ["cli"]
    ex2 = client.get("/explore/repos?topic=go", expect=200).json()
    assert not any(x["owner"] == username and x["name"] == name for x in ex2)

    # 清空标签
    client.put(f"{base}/topics", json={"topics": []}, expect=200)
    assert client.get(base, expect=200).json().get("topics", []) == []


def test_repo_topics_validation_and_permissions(user_factory):
    username, client, name = _user_repo(user_factory)
    base = f"/users/{username}/repos/{name}"

    assert client.put(f"{base}/topics", json={"topics": ["Has Space"]}).status_code == 400
    assert client.put(f"{base}/topics", json={"topics": ["UPPER!"]}).status_code == 400
    assert client.put(f"{base}/topics", json={"topics": ["_leading"]}).status_code == 400
    too_many = [f"t{i}" for i in range(25)]
    assert client.put(f"{base}/topics", json={"topics": too_many}).status_code == 400

    # 非 owner 不能改（返回 404 隐藏仓库存在性）
    _, _, other = user_factory("other")
    assert other.put(f"{base}/topics", json={"topics": ["x"]}).status_code == 404


def test_private_repo_topics_not_explorable(user_factory):
    username, client, name = _user_repo(user_factory, private=True)
    client.put(f"/users/{username}/repos/{name}/topics", json={"topics": ["secret-topic"]}, expect=200)

    ex = client.get("/explore/repos?topic=secret-topic", expect=200).json()
    assert not any(x["owner"] == username and x["name"] == name for x in ex)

    topics = client.get("/topics", expect=200).json()
    assert all(t["topic"] != "secret-topic" for t in topics)
