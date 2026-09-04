"""Issue 标签 / 里程碑 —— happy path + bad path + 权限；列表 modified_at 与 .gitkeep 自动清理。"""

import pytest


def _uuid() -> str:
    import uuid

    return uuid.uuid4().hex[:10]


def _p(owner, repo):
    return f"/users/{owner}/repos/{repo}"


@pytest.fixture
def lm_env(user_factory):
    an, _, c = user_factory("lm")
    repo = f"lm-{_uuid()}"
    c.post("/repos", json={"name": repo}, expect=201)
    c.post(_p(an, repo) + "/commits", json={
        "message": "seed",
        "changes": [{"path": "README.md", "action": "create", "content": "# x\n"}]}, expect=201)
    yield an, c, repo
    try:
        c.delete(f"/repos/{repo}", expect=204)
    except Exception:
        pass


def test_labels_crud_and_assign(lm_env):
    an, c, repo = lm_env
    c.post(_p(an, repo) + "/issues", json={"title": "issue1"}, expect=201)

    l1 = c.post(_p(an, repo) + "/labels", json={"name": "bug", "color": "d73a4a"}, expect=201).json()
    l2 = c.post(_p(an, repo) + "/labels", json={"name": "docs"}, expect=201).json()
    labels = c.get(_p(an, repo) + "/labels", expect=200).json()
    assert len(labels) == 2

    # 校验错误
    c.post(_p(an, repo) + "/labels", json={"name": "bug"}, expect=409)
    c.post(_p(an, repo) + "/labels", json={"name": "x", "color": "red"}, expect=400)

    # 打/换/清标签
    r = c.post(_p(an, repo) + "/issues/1/labels", json={"label_ids": [l1["id"], l2["id"]]}, expect=200).json()
    assert len(r["labels"]) == 2
    issues = c.get(_p(an, repo) + "/issues", expect=200).json()
    assert len(issues[0]["labels"]) == 2
    c.post(_p(an, repo) + "/issues/1/labels", json={"label_ids": []}, expect=200)
    c.post(_p(an, repo) + "/issues/1/labels", json={"label_ids": [99999]}, expect=400)
    c.post(_p(an, repo) + "/issues/99/labels", json={"label_ids": []}, expect=404)

    # 改名/删除
    c.patch(_p(an, repo) + f"/labels/{l1['id']}", json={"name": "bugfix"}, expect=200)
    c.delete(_p(an, repo) + f"/labels/{l1['id']}", expect=204)
    c.delete(_p(an, repo) + f"/labels/{l1['id']}", expect=404)


def test_milestones_crud_and_assign(lm_env):
    an, c, repo = lm_env
    c.post(_p(an, repo) + "/issues", json={"title": "a"}, expect=201)
    c.post(_p(an, repo) + "/issues", json={"title": "b"}, expect=201)

    m = c.post(_p(an, repo) + "/milestones",
               json={"title": "v1", "description": "first"}, expect=201).json()
    c.post(_p(an, repo) + "/milestones", json={"title": "  "}, expect=400)

    c.post(_p(an, repo) + "/issues/1/milestone", json={"milestone_id": m["id"]}, expect=200)
    c.post(_p(an, repo) + "/issues/2/milestone", json={"milestone_id": m["id"]}, expect=200)
    c.patch(f"/repos/{repo}/issues/2", json={"state": "closed"}, expect=200)

    ms = c.get(_p(an, repo) + "/milestones", expect=200).json()
    assert ms[0]["open_issues"] == 1 and ms[0]["closed_issues"] == 1

    issues = c.get(_p(an, repo) + "/issues", expect=200).json()
    assert issues[0]["milestone"]["title"] == "v1"

    # 更新/清除/删除
    c.patch(_p(an, repo) + f"/milestones/{m['id']}", json={"state": "closed"}, expect=200)
    c.post(_p(an, repo) + "/issues/1/milestone", json={"milestone_id": 0}, expect=200)
    c.post(_p(an, repo) + "/issues/1/milestone", json={"milestone_id": 7777}, expect=400)
    c.delete(_p(an, repo) + f"/milestones/{m['id']}", expect=204)
    c.delete(_p(an, repo) + f"/milestones/{m['id']}", expect=404)


def test_tree_last_modified_and_gitkeep_cleanup(lm_env):
    an, c, repo = lm_env
    # 建目录占位
    c.post(_p(an, repo) + "/commits", json={
        "message": "dir", "changes": [{"path": "docs/.gitkeep", "action": "create", "content": ""}]}, expect=201)
    # 放第一个真实文件 -> .gitkeep 自动移除
    c.post(_p(an, repo) + "/commits", json={
        "message": "real file", "changes": [{"path": "docs/api.md", "action": "create", "content": "hi"}]}, expect=201)
    entries = c.get(_p(an, repo) + "/tree?ref=main&path=docs", expect=200).json()["entries"]
    assert [e["name"] for e in entries] == ["api.md"]
    # 列表带最后修改信息
    assert entries[0]["modified_at"]
    root = c.get(_p(an, repo) + "/tree?ref=main&path=", expect=200).json()["entries"]
    for e in root:
        assert e["modified_at"]
