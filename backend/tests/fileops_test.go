package tests

import (
	"encoding/json"
	"fmt"
	"testing"
)

func writeCommit(t *testing.T, c *Client, owner, repo string, body map[string]any, want int) map[string]any {
	t.Helper()
	return c.mustStatus("POST",
		fmt.Sprintf("/users/%s/repos/%s/commits", owner, repo), body, want)
}

func treeRaw(t *testing.T, c *Client, repo, ref, path string) []map[string]any {
	t.Helper()
	u := fmt.Sprintf("/repos/%s/tree?ref=%s&path=%s", repo, ref, path)
	var out map[string]any
	if err := json.Unmarshal([]byte(rawGet(t, c, u)), &out); err != nil {
		t.Fatalf("tree decode: %v", err)
	}
	entries, _ := out["entries"].([]any)
	list := []map[string]any{}
	for _, e := range entries {
		list = append(list, e.(map[string]any))
	}
	return list
}

func TestWebFileOps(t *testing.T) {
	env := start(t)
	alice := register(t, env, "alice", "alice-pass-123")
	alice.mustStatus("POST", "/repos", map[string]string{"name": "ops"}, 201)

	// 1) 空仓库创建嵌套文件 -> 自动建 main
	m := writeCommit(t, alice, "alice", "ops", map[string]any{
		"branch":  "main",
		"message": "add docs",
		"changes": []any{
			map[string]any{"path": "docs/guide.md", "action": "create", "content": "# Guide\nhello"},
			map[string]any{"path": "README.md", "action": "create", "content": "# ops"},
		},
	}, 201)
	if m["sha"] == nil || len(m["sha"].(string)) != 40 {
		t.Fatalf("commit = %v", m)
	}
	// 目录结构正确
	root := treeRaw(t, alice, "ops", "main", "")
	if len(root) != 2 {
		t.Fatalf("root = %v", root)
	}
	docs := treeRaw(t, alice, "ops", "main", "docs")
	if len(docs) != 1 || docs[0]["name"] != "guide.md" || docs[0]["type"] != "blob" {
		t.Fatalf("docs = %v", docs)
	}

	// 2) 更新内容
	writeCommit(t, alice, "alice", "ops", map[string]any{
		"message": "edit guide",
		"changes": []any{
			map[string]any{"path": "docs/guide.md", "action": "update", "content": "# Guide v2"},
		},
	}, 201)
	b := alice.mustStatus("GET", "/repos/ops/blob?ref=main&path=docs/guide.md", nil, 200)
	if b["content"] != "# Guide v2" {
		t.Fatalf("blob = %q", b["content"])
	}

	// 3) 创建“文件夹”（.gitkeep 占位）
	writeCommit(t, alice, "alice", "ops", map[string]any{
		"message": "add empty dir",
		"changes": []any{map[string]any{"path": "assets/.gitkeep", "action": "create", "content": ""}},
	}, 201)

	// 4) 递归删除目录
	writeCommit(t, alice, "alice", "ops", map[string]any{
		"message": "rm docs",
		"changes": []any{map[string]any{"path": "docs", "action": "delete_tree"}},
	}, 201)
	if got := treeRaw(t, alice, "ops", "main", "docs"); len(got) != 0 {
		t.Fatalf("docs after delete = %v", got)
	}

	// 5) 删除单个文件
	writeCommit(t, alice, "alice", "ops", map[string]any{
		"message": "rm readme",
		"changes": []any{map[string]any{"path": "README.md", "action": "delete"}},
	}, 201)
	alice.mustFail("GET", "/repos/ops/blob?ref=main&path=README.md", nil, 400)

	// 6) bad path / 空信息 / 删除不存在文件 / 非法 action
	alice.mustFail("POST", "/users/alice/repos/ops/commits",
		map[string]any{"message": "x", "changes": []any{map[string]any{"path": "../evil", "action": "create"}}}, 400)
	alice.mustFail("POST", "/users/alice/repos/ops/commits",
		map[string]any{"message": "", "changes": []any{map[string]any{"path": "a.txt", "action": "create"}}}, 400)
	alice.mustFail("POST", "/users/alice/repos/ops/commits",
		map[string]any{"message": "x", "changes": []any{map[string]any{"path": "missing.txt", "action": "delete"}}}, 400)
	alice.mustFail("POST", "/users/alice/repos/ops/commits",
		map[string]any{"message": "x", "changes": []any{map[string]any{"path": "a.txt", "action": "chmod"}}}, 400)

	// 7) 分支参数（feature 分支新建）
	writeCommit(t, alice, "alice", "ops", map[string]any{
		"branch":  "feature/x",
		"message": "work",
		"changes": []any{map[string]any{"path": "f.txt", "action": "create", "content": "1"}},
	}, 201)
	feat := treeRaw(t, alice, "ops", "feature/x", "")
	if len(feat) == 0 {
		t.Fatalf("feature branch empty")
	}
}

func TestWebFileOpsPermissions(t *testing.T) {
	env := start(t)
	alice := register(t, env, "alice", "alice-pass-123")
	bob := register(t, env, "bob", "bob-pass-123456")
	alice.mustStatus("POST", "/repos", map[string]string{"name": "ops2"}, 201)

	// 未授权 / 无写权限
	(&Client{env: env}).mustFail("POST", "/users/alice/repos/ops2/commits",
		map[string]any{"message": "x", "changes": []any{map[string]any{"path": "a.txt", "action": "create"}}}, 401)
	bob.mustFail("POST", "/users/alice/repos/ops2/commits",
		map[string]any{"message": "x", "changes": []any{map[string]any{"path": "a.txt", "action": "create"}}}, 404)

	// write 协作者可写；read 协作者不行
	alice.mustStatus("POST", "/users/alice/repos/ops2/collabs",
		map[string]string{"username": "bob", "permission": "write"}, 200)
	writeCommit(t, bob, "alice", "ops2", map[string]any{
		"message": "bob edits",
		"changes": []any{map[string]any{"path": "b.txt", "action": "create", "content": "by bob"}},
	}, 201)
	b2 := bob.mustStatus("GET", "/repos/ops2/blob?ref=main&path=b.txt", nil, 200)
	if b2["content"] != "by bob" {
		t.Fatalf("bob content = %v", b2["content"])
	}
	// 降级为 read 后不能提交
	alice.mustStatus("POST", "/users/alice/repos/ops2/collabs",
		map[string]string{"username": "bob", "permission": "read"}, 200)
	bob.mustFail("POST", "/users/alice/repos/ops2/commits",
		map[string]any{"message": "x", "changes": []any{map[string]any{"path": "c.txt", "action": "create"}}}, 404)
}
