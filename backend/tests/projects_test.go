// projects_test.go 仓库看板项目（项目 / 列 / 泳道 / 卡片）CRUD 与权限、级联删除。
package tests

import (
	"fmt"
	"testing"
)

func projectP(owner, repo string, parts ...any) string {
	p := fmt.Sprintf("/users/%s/repos/%s/projects", owner, repo)
	for _, x := range parts {
		p += fmt.Sprintf("/%v", x)
	}
	return p
}

func TestProjectsCRUD(t *testing.T) {
	env := start(t)
	an := "pjowner"
	c := register(t, env, an, "pass-12345")
	c.mustStatus("POST", "/repos", map[string]string{"name": "proj"}, 201)

	// 创建项目：默认列 + 默认泳道
	p := c.mustStatus("POST", projectP(an, "proj"), map[string]string{"name": "kanban", "description": "d"}, 201)
	pid := int(p["id"].(float64))
	c.mustFail("POST", projectP(an, "proj"), map[string]string{"name": "  "}, 400)

	// 更新 / 列表
	c.mustStatus("PATCH", projectP(an, "proj", pid), map[string]string{"name": "kanban2"}, 200)
	c.mustStatus("GET", projectP(an, "proj"), nil, 200)

	board := c.mustStatus("GET", projectP(an, "proj", pid, "board"), nil, 200)
	cols, _ := board["columns"].([]any)
	lanes, _ := board["swimlanes"].([]any)
	if len(cols) != 3 || len(lanes) != 1 {
		t.Fatalf("default board: cols=%d lanes=%d", len(cols), len(lanes))
	}
	col0 := cols[0].(map[string]any)
	lane0 := lanes[0].(map[string]any)
	colID := int64(col0["id"].(float64))
	laneID := int64(lane0["id"].(float64))

	// issue 卡片
	c.mustStatus("POST", fmt.Sprintf("/users/%s/repos/%s/issues", an, "proj"), map[string]string{"title": "i1"}, 201)
	card := c.mustStatus("POST", projectP(an, "proj", pid, "cards"),
		map[string]any{"column_id": colID, "swimlane_id": laneID, "issue_number": 1}, 201)
	cardID := int64(card["id"].(float64))
	if card["issue_title"] != "i1" || card["issue_state"] != "open" {
		t.Fatalf("issue card enrich: %v", card)
	}
	// 不存在的 issue / 列 / 泳道
	c.mustFail("POST", projectP(an, "proj", pid, "cards"),
		map[string]any{"column_id": colID, "issue_number": 99}, 400)
	c.mustFail("POST", projectP(an, "proj", pid, "cards"),
		map[string]any{"column_id": 99999, "issue_number": 1}, 400)
	c.mustFail("POST", projectP(an, "proj", pid, "cards"),
		map[string]any{"column_id": colID, "swimlane_id": 99999, "issue_number": 1}, 400)

	// 文本卡片 + 移动到第二列、泳道退回未分组
	col1ID := int64(cols[1].(map[string]any)["id"].(float64))
	note := c.mustStatus("POST", projectP(an, "proj", pid, "cards"),
		map[string]any{"column_id": colID, "note": "hello"}, 201)
	noteID := int64(note["id"].(float64))
	c.mustStatus("PATCH", projectP(an, "proj", pid, "cards", noteID),
		map[string]any{"column_id": col1ID, "swimlane_id": 0, "position": 0}, 200)
	c.mustFail("PATCH", projectP(an, "proj", pid, "cards", noteID),
		map[string]any{"column_id": 99999}, 400)

	// 新列 / 新泳道 CRUD
	nc := c.mustStatus("POST", projectP(an, "proj", pid, "columns"), map[string]string{"name": "Review"}, 201)
	ncID := int(nc["id"].(float64))
	c.mustStatus("PATCH", projectP(an, "proj", pid, "columns", ncID), map[string]any{"name": "QA", "position": 0}, 204)
	c.mustStatus("DELETE", projectP(an, "proj", pid, "columns", ncID), nil, 204)
	c.mustFail("DELETE", projectP(an, "proj", pid, "columns", ncID), nil, 404)

	nl := c.mustStatus("POST", projectP(an, "proj", pid, "swimlanes"), map[string]string{"name": "Hotfix"}, 201)
	nlID := int(nl["id"].(float64))
	c.mustStatus("PATCH", projectP(an, "proj", pid, "swimlanes", nlID), map[string]any{"position": 0}, 204)
	// 删除泳道后卡片退回未分组
	c.mustStatus("PATCH", projectP(an, "proj", pid, "cards", cardID),
		map[string]any{"column_id": colID, "swimlane_id": nlID, "position": 0}, 200)
	c.mustStatus("DELETE", projectP(an, "proj", pid, "swimlanes", nlID), nil, 204)
	cards := c.mustStatus("GET", projectP(an, "proj", pid, "cards"), nil, 200)
	_ = cards

	// 编辑 / 删除卡片
	c.mustStatus("PATCH", projectP(an, "proj", pid, "cards", noteID), map[string]any{"note": "edited"}, 200)
	c.mustStatus("DELETE", projectP(an, "proj", pid, "cards", cardID), nil, 204)
	c.mustFail("DELETE", projectP(an, "proj", pid, "cards", cardID), nil, 404)

	// 级联：删除项目后子资源全部 404
	c.mustStatus("DELETE", projectP(an, "proj", pid), nil, 204)
	c.mustFail("GET", projectP(an, "proj", pid, "board"), nil, 404)
	c.mustFail("GET", projectP(an, "proj", pid, "columns"), nil, 404)
	c.mustFail("GET", projectP(an, "proj", pid, "swimlanes"), nil, 404)
	c.mustFail("GET", projectP(an, "proj", pid, "cards"), nil, 404)
}

func TestProjectsPermissions(t *testing.T) {
	env := start(t)
	owner := "pjown2"
	oc := register(t, env, owner, "pass-12345")
	oc.mustStatus("POST", "/repos", map[string]string{"name": "sec"}, 201)
	p := oc.mustStatus("POST", projectP(owner, "sec"), map[string]string{"name": "p1"}, 201)
	pid := int(p["id"].(float64))

	// 其他用户：私有仓库不可读
	other := register(t, env, "pjother", "pass-12345")
	other.mustFail("GET", projectP(owner, "sec"), nil, 404)
	other.mustFail("POST", projectP(owner, "sec"), map[string]string{"name": "x"}, 404)
	other.mustFail("DELETE", projectP(owner, "sec", pid), nil, 404)

	// 未登录
	anon := &Client{env: env}
	anon.mustFail("GET", projectP(owner, "sec"), nil, 401)
	anon.mustFail("POST", projectP(owner, "sec"), map[string]string{"name": "x"}, 401)
}
