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

func TestRefsPermissions(t *testing.T) {
	env := start(t)
	alice := register(t, env, "alice", "alice-pass-123")
	bob := register(t, env, "bob", "bob-pass-123456")
	alice.mustStatus("POST", "/repos", map[string]string{"name": "refs2"}, 201)
	writeCommit(t, alice, "alice", "refs2", map[string]any{
		"message": "m",
		"changes": []any{map[string]any{"path": "a.txt", "action": "create", "content": "1"}},
	}, 201)

	// 无权限：401/404；read 协作者可看 tags 不能写
	(&Client{env: env}).mustFail("GET", refsPath("alice", "refs2", "/tags"), nil, 401)
	bob.mustFail("GET", refsPath("alice", "refs2", "/tags"), nil, 404)
	alice.mustStatus("POST", refsPath("alice", "refs2", "/collabs"),
		map[string]string{"username": "bob", "permission": "read"}, 200)
	bob.mustStatus("GET", refsPath("alice", "refs2", "/tags"), nil, 200)
	bob.mustFail("POST", refsPath("alice", "refs2", "/refs"),
		map[string]any{"type": "tag", "name": "v1"}, 404)
	// write 协作者可以创建
	alice.mustStatus("POST", refsPath("alice", "refs2", "/collabs"),
		map[string]string{"username": "bob", "permission": "write"}, 200)
	bob.mustStatus("POST", refsPath("alice", "refs2", "/refs"),
		map[string]any{"type": "tag", "name": "v1"}, 201)
}
