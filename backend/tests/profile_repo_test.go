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

// TestOrgRepoCreatedOnCreate 验证创建组织时自动创建同名公开仓库（<org>/<org>）。
func TestOrgRepoCreatedOnCreate(t *testing.T) {
	env := start(t)
	t.Setenv("GITDASH_PROFILE_REPO", "1")
	alice := register(t, env, "alice", "alice-pass-123")

	alice.mustStatus("POST", "/orgs", map[string]string{"name": "acme", "display": "ACME"}, 201)

	m := alice.mustStatus("GET", "/users/acme/repos/acme", nil, 200)
	if m["owner"] != "acme" || m["name"] != "acme" {
		t.Fatalf("org repo = %v", m)
	}
	if m["private"] != false {
		t.Fatalf("org repo should be public: %v", m)
	}
	// 组织 owner 对组织仓库应具备 owner 角色（前端据此展示设置页）
	if m["role"] != "owner" {
		t.Fatalf("org owner should have owner role on org repo: %v", m)
	}
	// 仅 owner 可访问的仓库设置接口对组织 owner 应放行
	alice.mustStatus("GET", "/users/acme/repos/acme/webhooks", nil, 200)
	alice.mustStatus("GET", "/users/acme/repos/acme/branch-protections", nil, 200)
	// README 模板已初始化：main 分支存在
	alice.mustStatus("GET", "/users/acme/repos/acme/branches", nil, 200)
}
