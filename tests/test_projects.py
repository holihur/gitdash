"""Projects 看板（项目 / 列 / 泳道 / 卡片）—— happy path + bad path + 权限 + 级联删除。"""

from __future__ import annotations

import pytest


def _p(owner: str, repo: str) -> str:
    return f"/users/{owner}/repos/{repo}"


@pytest.fixture
def proj_env(repo_factory):
    repo, c = repo_factory("pj")
    return repo, c


def test_projects_crud_and_board(proj_env):
    repo, c = proj_env
    base = _p(c.get("/me", expect=200).json()["username"], repo)

    # 创建项目：默认 3 列 + 1 泳道
    p = c.post(base + "/projects", json={"name": "kanban", "description": "d"}, expect=201).json()
    pid = p["id"]
    c.post(base + "/projects", json={"name": "  "}, expect=400)

    c.patch(base + f"/projects/{pid}", json={"name": "kanban2"}, expect=200)
    assert c.get(base + "/projects", expect=200).json()[0]["name"] == "kanban2"

    board = c.get(base + f"/projects/{pid}/board", expect=200).json()
    assert len(board["columns"]) == 3
    assert len(board["swimlanes"]) == 1
    cols = sorted(board["columns"], key=lambda x: x["position"])
    lane = board["swimlanes"][0]

    # issue 卡片：附带标题与状态；坏路径
    c.post(base + "/issues", json={"title": "i1"}, expect=201)
    card = c.post(base + f"/projects/{pid}/cards",
                  json={"column_id": cols[0]["id"], "swimlane_id": lane["id"], "issue_number": 1},
                  expect=201).json()
    assert card["issue_title"] == "i1" and card["issue_state"] == "open"
    c.post(base + f"/projects/{pid}/cards", json={"column_id": cols[0]["id"], "issue_number": 99}, expect=400)
    c.post(base + f"/projects/{pid}/cards", json={"column_id": 99999, "issue_number": 1}, expect=400)
    c.post(base + f"/projects/{pid}/cards",
           json={"column_id": cols[0]["id"], "swimlane_id": 99999, "issue_number": 1}, expect=400)

    # 文本卡片 + 移动（换列、泳道退回未分组）
    note = c.post(base + f"/projects/{pid}/cards",
                  json={"column_id": cols[0]["id"], "note": "hello"}, expect=201).json()
    moved = c.patch(base + f"/projects/{pid}/cards/{note['id']}",
                    json={"column_id": cols[1]["id"], "swimlane_id": 0, "position": 0}, expect=200).json()
    assert moved["swimlane_id"] == 0
    c.patch(base + f"/projects/{pid}/cards/{note['id']}", json={"column_id": 99999}, expect=400)

    # 列 / 泳道 CRUD
    nc = c.post(base + f"/projects/{pid}/columns", json={"name": "Review"}, expect=201).json()
    c.patch(base + f"/projects/{pid}/columns/{nc['id']}", json={"name": "QA", "position": 0}, expect=204)
    c.delete(base + f"/projects/{pid}/columns/{nc['id']}", expect=204)
    c.delete(base + f"/projects/{pid}/columns/{nc['id']}", expect=404)

    nl = c.post(base + f"/projects/{pid}/swimlanes", json={"name": "Hotfix"}, expect=201).json()
    c.patch(base + f"/projects/{pid}/swimlanes/{nl['id']}", json={"position": 0}, expect=204)
    # 卡片挪进 Hotfix 泳道后删除泳道 -> 卡片退回未分组
    c.patch(base + f"/projects/{pid}/cards/{card['id']}",
            json={"column_id": cols[0]["id"], "swimlane_id": nl["id"], "position": 0}, expect=200)
    c.delete(base + f"/projects/{pid}/swimlanes/{nl['id']}", expect=204)
    cards = c.get(base + f"/projects/{pid}/cards", expect=200).json()
    assert any(x["id"] == card["id"] and x["swimlane_id"] == 0 for x in cards)

    # 卡片编辑 / 删除
    c.patch(base + f"/projects/{pid}/cards/{note['id']}", json={"note": "edited"}, expect=200)
    c.delete(base + f"/projects/{pid}/cards/{card['id']}", expect=204)
    c.delete(base + f"/projects/{pid}/cards/{card['id']}", expect=404)

    # 级联删除
    c.delete(base + f"/projects/{pid}", expect=204)
    c.get(base + f"/projects/{pid}/board", expect=404)
    c.get(base + f"/projects/{pid}/columns", expect=404)
    c.get(base + f"/projects/{pid}/swimlanes", expect=404)
    c.get(base + f"/projects/{pid}/cards", expect=404)


def test_projects_permissions(proj_env, user_factory, anon):
    repo, c = proj_env
    owner = c.get("/me", expect=200).json()["username"]
    base = _p(owner, repo)
    p = c.post(base + "/projects", json={"name": "p1"}, expect=201).json()

    # 其他用户不可读写私有仓库的项目
    _, _, other = user_factory("pjx")
    other.get(base + "/projects", expect=404)
    other.post(base + "/projects", json={"name": "x"}, expect=404)
    other.delete(base + f"/projects/{p['id']}", expect=404)

    # 未登录 401
    anon.get(base + "/projects", expect=401)
    anon.post(base + "/projects", json={"name": "x"}, expect=401)
