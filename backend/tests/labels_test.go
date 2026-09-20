package tests

import (
	"encoding/json"
	"fmt"
	"testing"
)

func lmPath(owner, repo, kind, suffix string) string {
	return fmt.Sprintf("/users/%s/repos/%s/%s%s", owner, repo, kind, suffix)
}

func findLabelByName(labels []map[string]any, name string) (map[string]any, bool) {
	for _, l := range labels {
		if l["name"] == name {
			return l, true
		}
	}
	return nil, false
}

func decodeIssues(t *testing.T, raw string) []map[string]any {
	t.Helper()
	var out []map[string]any
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("issues decode: %s err=%v", raw, err)
	}
	return out
}

func TestIssueLabelsManagement(t *testing.T) {
	env := start(t)
	alice := register(t, env, "alice", "alice-pass-123")
	alice.mustStatus("POST", "/repos", map[string]string{"name": "lm"}, 201)
	alice.mustStatus("POST", "/repos/lm/issues", map[string]string{"title": "bug"}, 201)

	// 新建仓库会预置一批默认标签：应能找到 bug 且颜色为默认值。
	raw := rawGet(t, alice, lmPath("alice", "lm", "labels", ""))
	var labels []map[string]any
	if err := json.Unmarshal([]byte(raw), &labels); err != nil {
		t.Fatalf("labels decode: %v", err)
	}
	seeded := len(labels)
	if seeded == 0 {
		t.Fatalf("expected seeded default labels, got %s", raw)
	}
	bug, ok := findLabelByName(labels, "bug")
	if !ok || bug["color"] != "d73a4a" {
		t.Fatalf("seeded bug label = %v", bug)
	}
	lid := int64(bug["id"].(float64))

	// label CRUD
	alice.mustFail("POST", lmPath("alice", "lm", "labels", ""),
		map[string]string{"name": "bug"}, 409) // 与默认标签同名
	alice.mustFail("POST", lmPath("alice", "lm", "labels", ""),
		map[string]string{"name": "x", "color": "red"}, 400) // 非法颜色
	l2 := alice.mustStatus("POST", lmPath("alice", "lm", "labels", ""),
		map[string]string{"name": "regression"}, 201)
	l2id := int64(l2["id"].(float64))
	raw = rawGet(t, alice, lmPath("alice", "lm", "labels", ""))
	labels = nil
	if err := json.Unmarshal([]byte(raw), &labels); err != nil || len(labels) != seeded+1 {
		t.Fatalf("labels = %s err=%v", raw, err)
	}

	// 打标签（全量替换）
	alice.mustStatus("POST", "/users/alice/repos/lm/issues/1/labels",
		map[string]any{"label_ids": []int64{lid, l2id}}, 200)
	rawList := rawGet(t, alice, "/repos/lm/issues")
	var issues []map[string]any
	if err := json.Unmarshal([]byte(rawList), &issues); err != nil {
		t.Fatalf("issues decode: %v", err)
	}
	if ls, ok := issues[0]["labels"].([]any); !ok || len(ls) != 2 {
		t.Fatalf("issue labels = %v", issues[0]["labels"])
	}
	// 替换为空
	alice.mustStatus("POST", "/users/alice/repos/lm/issues/1/labels",
		map[string]any{"label_ids": []int64{}}, 200)
	// 跨仓库 label id 拒绝
	alice.mustFail("POST", "/users/alice/repos/lm/issues/1/labels",
		map[string]any{"label_ids": []int64{9999}}, 400)
	// 不存在的 issue
	alice.mustFail("POST", "/users/alice/repos/lm/issues/99/labels",
		map[string]any{"label_ids": []int64{lid}}, 404)

	// 改名 + 删除
	alice.mustStatus("PATCH", lmPath("alice", "lm", "labels", fmt.Sprintf("/%d", lid)),
		map[string]string{"name": "bugfix", "color": "0e8a16"}, 200)
	alice.mustStatus("DELETE", lmPath("alice", "lm", "labels", fmt.Sprintf("/%d", lid)), nil, 204)
	alice.mustFail("DELETE", lmPath("alice", "lm", "labels", fmt.Sprintf("/%d", lid)), nil, 404)
}

func TestIssueMilestonesManagement(t *testing.T) {
	env := start(t)
	alice := register(t, env, "alice", "alice-pass-123")
	alice.mustStatus("POST", "/repos", map[string]string{"name": "lm"}, 201)
	alice.mustStatus("POST", "/repos/lm/issues", map[string]string{"title": "i1"}, 201)
	alice.mustStatus("POST", "/repos/lm/issues", map[string]string{"title": "i2"}, 201)

	m := alice.mustStatus("POST", lmPath("alice", "lm", "milestones", ""),
		map[string]string{"title": "v0.7", "description": "release"}, 201)
	mid := int64(m["id"].(float64))
	alice.mustFail("POST", lmPath("alice", "lm", "milestones", ""),
		map[string]string{"title": "  "}, 400)

	// 指派两个 issue，关闭其一
	alice.mustStatus("POST", "/users/alice/repos/lm/issues/1/milestone",
		map[string]any{"milestone_id": mid}, 200)
	alice.mustStatus("POST", "/users/alice/repos/lm/issues/2/milestone",
		map[string]any{"milestone_id": mid}, 200)
	alice.mustStatus("PATCH", "/repos/lm/issues/2", map[string]string{"state": "closed"}, 200)
	raw := rawGet(t, alice, lmPath("alice", "lm", "milestones", ""))
	var ms []map[string]any
	if err := json.Unmarshal([]byte(raw), &ms); err != nil || len(ms) != 1 {
		t.Fatalf("milestones = %s err=%v", raw, err)
	}
	if ms[0]["open_issues"] != float64(1) || ms[0]["closed_issues"] != float64(1) {
		t.Fatalf("milestone counts = %v", ms[0])
	}

	// issue 响应带 milestone
	rawList := rawGet(t, alice, "/repos/lm/issues")
	var issues []map[string]any
	if err := json.Unmarshal([]byte(rawList), &issues); err != nil {
		t.Fatalf("issues decode: %v", err)
	}
	if ms, ok := issues[0]["milestone"].(map[string]any); !ok || ms["title"] != "v0.7" {
		t.Fatalf("issue milestone = %v", issues[0]["milestone"])
	}

	// 按里程碑过滤：指定 id / 未指派 / 与状态组合 / 非法值。
	byMs := decodeIssues(t, rawGet(t, alice, "/repos/lm/issues?milestone="+fmt.Sprint(mid)))
	if len(byMs) != 2 {
		t.Fatalf("milestone filter = %+v", byMs)
	}
	if none := decodeIssues(t, rawGet(t, alice, "/repos/lm/issues?milestone=none")); len(none) != 0 {
		t.Fatalf("no-milestone filter should be empty, got %+v", none)
	}
	closedByMs := decodeIssues(t, rawGet(t, alice, "/repos/lm/issues?state=closed&milestone="+fmt.Sprint(mid)))
	if len(closedByMs) != 1 || closedByMs[0]["number"] != float64(2) {
		t.Fatalf("combined filter = %+v", closedByMs)
	}
	alice.mustFail("GET", "/repos/lm/issues?milestone=abc", nil, 400)

	// 更新状态 / 标题；清除指派
	alice.mustStatus("PATCH", lmPath("alice", "lm", "milestones", fmt.Sprintf("/%d", mid)),
		map[string]string{"state": "closed", "title": "v0.7-final"}, 200)
	alice.mustStatus("POST", "/users/alice/repos/lm/issues/1/milestone",
		map[string]any{"milestone_id": 0}, 200)
	// 清除后，未指派过滤应命中 issue 1。
	if none := decodeIssues(t, rawGet(t, alice, "/repos/lm/issues?milestone=none")); len(none) != 1 || none[0]["number"] != float64(1) {
		t.Fatalf("no-milestone after clear = %+v", none)
	}
	// 非法 milestone id（跨仓库）
	alice.mustFail("POST", "/users/alice/repos/lm/issues/1/milestone",
		map[string]any{"milestone_id": 7777}, 400)

	// 删除里程碑后 issue 不残留
	alice.mustStatus("DELETE", lmPath("alice", "lm", "milestones", fmt.Sprintf("/%d", mid)), nil, 204)
	alice.mustFail("DELETE", lmPath("alice", "lm", "milestones", fmt.Sprintf("/%d", mid)), nil, 404)
}

func TestLabelsMilestonesPermissions(t *testing.T) {
	env := start(t)
	alice := register(t, env, "alice", "alice-pass-123")
	bob := register(t, env, "bobby", "bob-pass-123456")
	alice.mustStatus("POST", "/repos", map[string]string{"name": "lm"}, 201)

	// 未登录 / 无权限一律 404（不泄露仓库存在性）
	(&Client{env: env}).mustFail("GET", lmPath("alice", "lm", "labels", ""), nil, 404)
	bob.mustFail("GET", lmPath("alice", "lm", "labels", ""), nil, 404)
	alice.mustStatus("POST", "/users/alice/repos/lm/collabs",
		map[string]string{"username": "bobby", "permission": "read"}, 200)
	bob.mustStatus("GET", lmPath("alice", "lm", "labels", ""), nil, 200)
	bob.mustStatus("GET", lmPath("alice", "lm", "milestones", ""), nil, 200)
	bob.mustFail("POST", lmPath("alice", "lm", "labels", ""), map[string]string{"name": "x"}, 404)
}
