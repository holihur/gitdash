package tests

import (
	"fmt"
	"testing"
)

func TestRevertCommit(t *testing.T) {
	env := start(t)
	alice := register(t, env, "alice", "alice-pass-123")
	bob := register(t, env, "bobby", "bob-pass-123456")
	alice.mustStatus("POST", "/repos", map[string]string{"name": "rev"}, 201)

	// 初始提交 + 第二次修改
	writeCommit(t, alice, "alice", "rev", map[string]any{
		"message": "add file",
		"changes": []any{map[string]any{"path": "a.txt", "action": "create", "content": "v1"}},
	}, 201)
	target := writeCommit(t, alice, "alice", "rev", map[string]any{
		"message": "edit file",
		"changes": []any{map[string]any{"path": "a.txt", "action": "update", "content": "v2"}},
	}, 201)["sha"].(string)

	revertPath := fmt.Sprintf("/users/alice/repos/rev/commits/%s/revert", target)

	// 权限与参数校验
	bob.mustFail("POST", revertPath, map[string]any{"branch": "main"}, 404)
	alice.mustFail("POST", "/users/alice/repos/rev/commits/notasha/revert",
		map[string]any{"branch": "main"}, 400)
	alice.mustFail("POST", revertPath, map[string]any{}, 400)

	// 执行 revert
	r := alice.mustStatus("POST", revertPath, map[string]any{"branch": "main"}, 201)
	newSha, _ := r["sha"].(string)
	if len(newSha) != 40 {
		t.Fatalf("revert response = %v", r)
	}

	// 文件内容回退到 v1
	b := alice.mustStatus("GET", "/repos/rev/blob?ref=main&path=a.txt", nil, 200)
	if b["content"] != "v1" {
		t.Fatalf("content after revert = %v, want v1", b["content"])
	}

	// 历史里出现新的 Revert 提交，且为分支 tip
	commits := getJSON[[]Commit](t, alice, "/repos/rev/commits?ref=main", 200)
	if len(commits) < 3 {
		t.Fatalf("commits after revert = %+v", commits)
	}
	if commits[0].SHA != newSha {
		t.Fatalf("tip = %s, want %s", commits[0].SHA, newSha)
	}
	if commits[0].Message != `Revert "edit file"` {
		t.Fatalf("revert message = %q", commits[0].Message)
	}

	// 自定义提交信息
	r2 := alice.mustStatus("POST",
		fmt.Sprintf("/users/alice/repos/rev/commits/%s/revert", newSha),
		map[string]any{"branch": "main", "message": "undo the undo"}, 201)
	_ = r2
	commits = getJSON[[]Commit](t, alice, "/repos/rev/commits?ref=main", 200)
	if commits[0].Message != "undo the undo" {
		t.Fatalf("custom revert message = %q", commits[0].Message)
	}
}

// TestRevertCommitOnBranch 验证可把提交回退到指定分支（其它分支不受影响）。
func TestRevertCommitOnBranch(t *testing.T) {
	env := start(t)
	alice := register(t, env, "alice", "alice-pass-123")
	alice.mustStatus("POST", "/repos", map[string]string{"name": "revb"}, 201)

	writeCommit(t, alice, "alice", "revb", map[string]any{
		"branch":  "main",
		"message": "init",
		"changes": []any{map[string]any{"path": "a.txt", "action": "create", "content": "v1"}},
	}, 201)
	target := writeCommit(t, alice, "alice", "revb", map[string]any{
		"branch":  "main",
		"message": "edit",
		"changes": []any{map[string]any{"path": "a.txt", "action": "update", "content": "v2"}},
	}, 201)["sha"].(string)

	alice.mustStatus("POST",
		fmt.Sprintf("/users/alice/repos/revb/commits/%s/revert", target),
		map[string]any{"branch": "main"}, 201)

	// main 回退，dev 从未创建 → revert 到不存在的分支报错
	alice.mustFail("POST",
		fmt.Sprintf("/users/alice/repos/revb/commits/%s/revert", target),
		map[string]any{"branch": "dev"}, 400)
}
