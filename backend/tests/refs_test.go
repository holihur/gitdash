package tests

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func refsPath(owner, repo, suffix string) string {
	return fmt.Sprintf("/users/%s/repos/%s%s", owner, repo, suffix)
}

func TestBranchAndTagManagement(t *testing.T) {
	env := start(t)
	alice := register(t, env, "alice", "alice-pass-123")
	alice.mustStatus("POST", "/repos", map[string]string{"name": "refs"}, 201)
	// 制造两次提交（main）
	writeCommit(t, alice, "alice", "refs", map[string]any{
		"message": "first",
		"changes": []any{map[string]any{"path": "a.txt", "action": "create", "content": "1"}},
	}, 201)
	writeCommit(t, alice, "alice", "refs", map[string]any{
		"message": "second",
		"changes": []any{map[string]any{"path": "b.txt", "action": "create", "content": "2"}},
	}, 201)

	// tags：初始为空 -> 创建 -> 列表 -> 重复 409
	raw := rawGet(t, alice, refsPath("alice", "refs", "/tags"))
	var tags []map[string]any
	if err := json.Unmarshal([]byte(raw), &tags); err != nil || len(tags) != 0 {
		t.Fatalf("tags init = %s err=%v", raw, err)
	}
	m := alice.mustStatus("POST", refsPath("alice", "refs", "/refs"),
		map[string]any{"type": "tag", "name": "v1.0", "from": "main"}, 201)
	if m["name"] != "v1.0" || len(m["sha"].(string)) != 40 {
		t.Fatalf("create tag = %v", m)
	}
	alice.mustFail("POST", refsPath("alice", "refs", "/refs"),
		map[string]any{"type": "tag", "name": "v1.0"}, 409)

	tagsRaw := rawGet(t, alice, refsPath("alice", "refs", "/tags"))
	if err := json.Unmarshal([]byte(tagsRaw), &tags); err != nil || len(tags) != 1 || tags[0]["name"] != "v1.0" {
		t.Fatalf("tags = %s err=%v", tagsRaw, err)
	}

	// 创建分支 dev（含 '/' 的嵌套名）
	alice.mustStatus("POST", refsPath("alice", "refs", "/refs"),
		map[string]any{"type": "branch", "name": "feature/dev", "from": "main"}, 201)
	brRaw := rawGet(t, alice, "/repos/refs/branches")
	if !strings.Contains(brRaw, "feature/dev") {
		t.Fatalf("branches = %s", brRaw)
	}

	// bad path
	alice.mustFail("POST", refsPath("alice", "refs", "/refs"),
		map[string]any{"type": "branch", "name": "bad..name"}, 400)
	alice.mustFail("POST", refsPath("alice", "refs", "/refs"),
		map[string]any{"type": "branch", "name": "x", "from": "no-such-ref"}, 400)
	alice.mustFail("POST", refsPath("alice", "refs", "/refs"),
		map[string]any{"type": "weird", "name": "x"}, 400)

	// 删除 tag / 分支；HEAD 不可删
	alice.mustStatus("DELETE", refsPath("alice", "refs", "/refs/tag/v1.0"), nil, 204)
	alice.mustFail("DELETE", refsPath("alice", "refs", "/refs/tag/v1.0"), nil, 404)
	alice.mustStatus("DELETE", refsPath("alice", "refs", "/refs/branch/feature%2Fdev"), nil, 204)
	alice.mustFail("DELETE", refsPath("alice", "refs", "/refs/branch/main"), nil, 409)
}

func TestRefNotes(t *testing.T) {
	env := start(t)
	alice := register(t, env, "alice", "alice-pass-123")
	alice.mustStatus("POST", "/repos", map[string]string{"name": "refnotes"}, 201)
	writeCommit(t, alice, "alice", "refnotes", map[string]any{
		"message": "first",
		"changes": []any{map[string]any{"path": "a.txt", "action": "create", "content": "1"}},
	}, 201)
	alice.mustStatus("POST", refsPath("alice", "refnotes", "/refs"),
		map[string]any{"type": "branch", "name": "dev", "from": "main"}, 201)
	alice.mustStatus("POST", refsPath("alice", "refnotes", "/refs"),
		map[string]any{"type": "tag", "name": "v1.0", "from": "main"}, 201)

	// 设置分支 / 标签备注
	m := alice.mustStatus("PUT", refsPath("alice", "refnotes", "/refs/branch/dev/note"),
		map[string]string{"note": "开发分支"}, 200)
	if m["note"] != "开发分支" {
		t.Fatalf("set branch note = %v", m)
	}
	alice.mustStatus("PUT", refsPath("alice", "refnotes", "/refs/tag/v1.0/note"),
		map[string]string{"note": "首个版本"}, 200)

	// 列表返回备注
	var branches []map[string]any
	brRaw, brTotal := rawGetPaged(t, alice, "/repos/refnotes/branches")
	if err := json.Unmarshal([]byte(brRaw), &branches); err != nil {
		t.Fatalf("branches unmarshal: %v", err)
	}
	if brTotal != 2 {
		t.Fatalf("branches total = %d, want 2", brTotal)
	}
	var devNote any
	for _, b := range branches {
		if b["name"] == "dev" {
			devNote = b["note"]
		}
	}
	if devNote != "开发分支" {
		t.Fatalf("dev note = %v, branches = %s", devNote, brRaw)
	}

	var tags []map[string]any
	tagRaw := rawGet(t, alice, refsPath("alice", "refnotes", "/tags"))
	if err := json.Unmarshal([]byte(tagRaw), &tags); err != nil {
		t.Fatalf("tags unmarshal: %v", err)
	}
	if len(tags) != 1 || tags[0]["note"] != "首个版本" {
		t.Fatalf("tags = %s", tagRaw)
	}

	// 清空备注后列表不再返回 note
	alice.mustStatus("PUT", refsPath("alice", "refnotes", "/refs/branch/dev/note"),
		map[string]string{"note": ""}, 200)
	brRaw = rawGet(t, alice, "/repos/refnotes/branches")
	if strings.Contains(brRaw, "开发分支") {
		t.Fatalf("note not cleared: %s", brRaw)
	}

	// 删除标签后备注被清理：重建同名标签不应带回旧备注
	alice.mustStatus("DELETE", refsPath("alice", "refnotes", "/refs/tag/v1.0"), nil, 204)
	alice.mustStatus("POST", refsPath("alice", "refnotes", "/refs"),
		map[string]any{"type": "tag", "name": "v1.0", "from": "main"}, 201)
	tagRaw = rawGet(t, alice, refsPath("alice", "refnotes", "/tags"))
	if strings.Contains(tagRaw, "首个版本") {
		t.Fatalf("deleted tag note leaked: %s", tagRaw)
	}

	// 无效 kind / 非法备注长度
	alice.mustFail("PUT", refsPath("alice", "refnotes", "/refs/weird/dev/note"),
		map[string]string{"note": "x"}, 400)
}

func TestRefNotesPermissions(t *testing.T) {
	env := start(t)
	alice := register(t, env, "alice", "alice-pass-123")
	bob := register(t, env, "bobby", "bob-pass-123456")
	alice.mustStatus("POST", "/repos", map[string]string{"name": "refnotes2"}, 201)
	writeCommit(t, alice, "alice", "refnotes2", map[string]any{
		"message": "m",
		"changes": []any{map[string]any{"path": "a.txt", "action": "create", "content": "1"}},
	}, 201)

	// 只读协作者不能写备注
	alice.mustStatus("POST", refsPath("alice", "refnotes2", "/collabs"),
		map[string]string{"username": "bobby", "permission": "read"}, 200)
	bob.mustFail("PUT", refsPath("alice", "refnotes2", "/refs/branch/main/note"),
		map[string]string{"note": "hi"}, 404)
	// write 协作者可以写备注
	alice.mustStatus("POST", refsPath("alice", "refnotes2", "/collabs"),
		map[string]string{"username": "bobby", "permission": "write"}, 200)
	bob.mustStatus("PUT", refsPath("alice", "refnotes2", "/refs/branch/main/note"),
		map[string]string{"note": "bob's note"}, 200)
}

func TestRefsPagination(t *testing.T) {
	env := start(t)
	alice := register(t, env, "alice", "alice-pass-123")
	alice.mustStatus("POST", "/repos", map[string]string{"name": "refpage"}, 201)
	writeCommit(t, alice, "alice", "refpage", map[string]any{
		"message": "first",
		"changes": []any{map[string]any{"path": "a.txt", "action": "create", "content": "1"}},
	}, 201)
	// main + 3 新分支 + 2 个新标签
	for _, b := range []string{"b1", "b2", "b3"} {
		alice.mustStatus("POST", refsPath("alice", "refpage", "/refs"),
			map[string]any{"type": "branch", "name": b, "from": "main"}, 201)
	}
	for _, tg := range []string{"t1", "t2"} {
		alice.mustStatus("POST", refsPath("alice", "refpage", "/refs"),
			map[string]any{"type": "tag", "name": tg, "from": "main"}, 201)
	}

	// branches：total=4，limit=2 返回 2 条
	body, total := rawGetPaged(t, alice, "/repos/refpage/branches?limit=2&offset=0")
	if total != 4 {
		t.Fatalf("branches total = %d, want 4", total)
	}
	var branches []map[string]any
	if err := json.Unmarshal([]byte(body), &branches); err != nil || len(branches) != 2 {
		t.Fatalf("page 1 = %s err=%v", body, err)
	}
	// 第二页
	body, _ = rawGetPaged(t, alice, "/repos/refpage/branches?limit=2&offset=2")
	if err := json.Unmarshal([]byte(body), &branches); err != nil || len(branches) != 2 {
		t.Fatalf("page 2 = %s err=%v", body, err)
	}
	// 越界返回空数组
	body, _ = rawGetPaged(t, alice, "/repos/refpage/branches?limit=2&offset=10")
	if err := json.Unmarshal([]byte(body), &branches); err != nil || len(branches) != 0 {
		t.Fatalf("page beyond = %s err=%v", body, err)
	}

	// tags：total=2，limit=1
	body, total = rawGetPaged(t, alice, refsPath("alice", "refpage", "/tags")+"?limit=1&offset=0")
	if total != 2 {
		t.Fatalf("tags total = %d, want 2", total)
	}
	var tags []map[string]any
	if err := json.Unmarshal([]byte(body), &tags); err != nil || len(tags) != 1 {
		t.Fatalf("tags page = %s err=%v", body, err)
	}
}

func TestRefsPermissions(t *testing.T) {
	env := start(t)
	alice := register(t, env, "alice", "alice-pass-123")
	bob := register(t, env, "bobby", "bob-pass-123456")
	alice.mustStatus("POST", "/repos", map[string]string{"name": "refs2"}, 201)
	writeCommit(t, alice, "alice", "refs2", map[string]any{
		"message": "m",
		"changes": []any{map[string]any{"path": "a.txt", "action": "create", "content": "1"}},
	}, 201)

	// 无权限一律 404（不泄露仓库存在性）；read 协作者可看 tags 不能写
	(&Client{env: env}).mustFail("GET", refsPath("alice", "refs2", "/tags"), nil, 404)
	bob.mustFail("GET", refsPath("alice", "refs2", "/tags"), nil, 404)
	alice.mustStatus("POST", refsPath("alice", "refs2", "/collabs"),
		map[string]string{"username": "bobby", "permission": "read"}, 200)
	bob.mustStatus("GET", refsPath("alice", "refs2", "/tags"), nil, 200)
	bob.mustFail("POST", refsPath("alice", "refs2", "/refs"),
		map[string]any{"type": "tag", "name": "v1"}, 404)
	// write 协作者可以创建
	alice.mustStatus("POST", refsPath("alice", "refs2", "/collabs"),
		map[string]string{"username": "bobby", "permission": "write"}, 200)
	bob.mustStatus("POST", refsPath("alice", "refs2", "/refs"),
		map[string]any{"type": "tag", "name": "v1"}, 201)
}
