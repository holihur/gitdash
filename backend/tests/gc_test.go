package tests

import "testing"

// TestRepoGC 验证仓库 gc 接口：所有者可执行并返回前后占用，非所有者 / 未登录被拒。
func TestRepoGC(t *testing.T) {
	env := start(t)
	alice := register(t, env, "alice", "alice-pass-123")
	bob := register(t, env, "bobby", "bob-pass-123")

	alice.mustStatus("POST", "/repos", map[string]string{"name": "gcrepo"}, 201)
	// 写入一些对象，确保仓库里有内容可打包
	writeCommit(t, alice, "alice", "gcrepo", map[string]any{
		"branch":  "main",
		"message": "add files",
		"changes": []any{
			map[string]any{"path": "README.md", "action": "create", "content": "# gc"},
			map[string]any{"path": "docs/a.md", "action": "create", "content": "# a"},
		},
	}, 201)

	res := alice.mustStatus("POST", "/users/alice/repos/gcrepo/gc", nil, 200)
	for _, k := range []string{"before_bytes", "after_bytes", "freed_bytes"} {
		if _, ok := res[k]; !ok {
			t.Fatalf("gc response missing %q: %v", k, res)
		}
	}

	// 简写路由：当前用户名下的仓库
	alice.mustStatus("POST", "/repos/gcrepo/gc", nil, 200)

	// 仅所有者可执行
	bob.mustFail("POST", "/users/alice/repos/gcrepo/gc", nil, 404)
	// 未登录
	(&Client{env: env}).mustFail("POST", "/users/alice/repos/gcrepo/gc", nil, 401)
}
