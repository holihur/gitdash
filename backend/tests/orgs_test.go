package tests

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestOrgNamespaceLifecycle(t *testing.T) {
	env := start(t)
	alice := register(t, env, "alice", "alice-pass-123")
	bob := register(t, env, "bob", "bob-pass-123456")
	carol := register(t, env, "carol", "carol-pass-123456")

	// 创建组织（creator 自动成为 owner）
	m := alice.mustStatus("POST", "/orgs", map[string]string{"name": "acme", "display": "ACME"}, 201)
	if m["name"] != "acme" {
		t.Fatalf("create org = %v", m)
	}
	// 用户名占用 / 重复组织名
	alice.mustFail("POST", "/orgs", map[string]string{"name": "acme"}, 409)
	// 已有用户名不能注册组织
	alice.mustFail("POST", "/orgs", map[string]string{"name": "bob"}, 409)
	// 组织名不能再注册用户
	bob.mustFail("POST", "/auth/register",
		map[string]string{"username": "acme", "password": "pass12345678"}, 409)

	// 我的组织列表带 role
	raw := rawGet(t, alice, "/orgs")
	var orgs []map[string]any
	if err := json.Unmarshal([]byte(raw), &orgs); err != nil || len(orgs) != 1 || orgs[0]["role"] != "owner" {
		t.Fatalf("orgs = %s err=%v", raw, err)
	}

	// 在组织下建仓库（namespace）
	r := alice.mustStatus("POST", "/repos",
		map[string]any{"name": "web", "namespace": "acme"}, 201)
	if r["owner"] != "acme" {
		t.Fatalf("repo owner = %v", r)
	}
	alice.mustStatus("POST", "/users/acme/repos/web/issues", map[string]string{"title": "hi"}, 201)

	// 加成员
	alice.mustStatus("POST", "/orgs/acme/members",
		map[string]string{"username": "bob", "role": "member"}, 200)
	// bob 可读写组织仓库
	bob.mustStatus("GET", "/users/acme/repos/web/issues", nil, 200)
	bob.mustStatus("POST", "/users/acme/repos/web/issues", map[string]string{"title": "by bob"}, 201)
	// bob 也可在组织下建仓库
	bob.mustStatus("POST", "/repos",
		map[string]any{"name": "cli", "namespace": "acme"}, 201)
	// bob 不能管理成员/删除组织
	bob.mustFail("POST", "/orgs/acme/members", map[string]string{"username": "carol"}, 404)
	bob.mustFail("DELETE", "/orgs/acme", nil, 404)

	// 非成员（carol）访问组织仓库 → 404（私有默认）
	carol.mustFail("GET", "/users/acme/repos/web/issues", nil, 404)
	carol.mustFail("POST", "/repos", map[string]any{"name": "x", "namespace": "acme"}, 403)
	carol.mustFail("GET", "/orgs/acme/repos", nil, 404)

	// 公开组织仓库：非成员也可读
	alice.mustStatus("POST", "/users/acme/repos/web/visibility",
		map[string]any{"private": false}, 200)
	carol.mustStatus("GET", "/users/acme/repos/web/issues", nil, 200)
	carol.mustFail("POST", "/users/acme/repos/web/issues", map[string]string{"title": "x"}, 404)

	// 组织下仓库列表（成员视角）
	rawRepos := rawGet(t, alice, "/orgs/acme/repos")
	var orgRepos map[string]any
	if err := json.Unmarshal([]byte(rawRepos), &orgRepos); err != nil {
		t.Fatalf("org repos decode: %v", err)
	}
	if orgRepos["role"] != "owner" {
		t.Fatalf("org repos role = %v", orgRepos["role"])
	}

	// owner 移除成员；删除组织前需先清空仓库（拒绝）
	alice.mustStatus("DELETE", "/orgs/acme/members/bob", nil, 204)
	bob.mustFail("GET", "/orgs/acme/repos", nil, 404)
	alice.mustFail("DELETE", "/orgs/acme", nil, 409)

	// 清理后删除组织
	alice.mustStatus("DELETE", "/users/acme/repos/web", nil, 204)
	alice.mustStatus("DELETE", "/users/acme/repos/cli", nil, 204)
	alice.mustStatus("DELETE", "/orgs/acme", nil, 204)
	alice.mustFail("GET", "/orgs/acme", nil, 404)

	_ = fmt.Sprint
}
