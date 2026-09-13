package tests

import "testing"

// TestProfileRepoCreatedOnRegister 验证注册时自动创建同名公开仓库（<username>/<username>）。
// 集成测试夹具默认关闭该行为（GITDASH_PROFILE_REPO=0），这里显式打开。
func TestProfileRepoCreatedOnRegister(t *testing.T) {
	env := start(t)
	t.Setenv("GITDASH_PROFILE_REPO", "1")
	alice := register(t, env, "alice", "alice-pass-123")

	m := alice.mustStatus("GET", "/users/alice/repos/alice", nil, 200)
	if m["owner"] != "alice" || m["name"] != "alice" {
		t.Fatalf("profile repo = %v", m)
	}
	if m["private"] != false {
		t.Fatalf("profile repo should be public: %v", m)
	}

	found := false
	for _, r := range listRepos(t, alice) {
		if r.Owner == "alice" && r.Name == "alice" {
			found = true
		}
	}
	if !found {
		t.Fatal("profile repo not in repo list")
	}

	// README 模板已初始化：main 分支存在
	alice.mustStatus("GET", "/users/alice/repos/alice/branches", nil, 200)
}
