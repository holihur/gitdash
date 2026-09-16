package tests

import (
	"strings"
	"testing"
)

func TestOrgFollowAndProfile(t *testing.T) {
	env := start(t)
	alice := register(t, env, "alice", "alice-pass-123")
	bobby := register(t, env, "bobby", "bob-pass-123456")

	// alice 创建组织 acme，并建一个公开仓库和一个私有仓库
	alice.mustStatus("POST", "/orgs", map[string]string{"name": "acme", "display": "ACME Inc"}, 201)
	alice.mustStatus("POST", "/repos", map[string]any{"name": "pub", "namespace": "acme"}, 201)
	alice.mustStatus("POST", "/repos", map[string]any{"name": "priv", "namespace": "acme"}, 201)
	alice.mustStatus("POST", "/users/acme/repos/pub/visibility", map[string]any{"private": false}, 200)

	// 非成员（bobby）可见公开主页：仅公开仓库、无角色、未关注
	m := bobby.mustStatus("GET", "/orgs/acme/profile", nil, 200)
	if m["name"] != "acme" || m["display"] != "ACME Inc" {
		t.Fatalf("org profile = %v", m)
	}
	if m["role"] != "" || m["is_following"] != false || m["followers"].(float64) != 0 {
		t.Fatalf("non-member state = %v", m)
	}
	if repos, _ := m["repos"].([]any); len(repos) != 1 {
		t.Fatalf("public profile repos = %v", m["repos"])
	}

	// owner 看到全部仓库与 role
	m = alice.mustStatus("GET", "/orgs/acme/profile", nil, 200)
	if m["role"] != "owner" {
		t.Fatalf("owner role = %v", m["role"])
	}
	if repos, _ := m["repos"].([]any); len(repos) != 2 {
		t.Fatalf("member profile repos = %v", m["repos"])
	}

	// bobby 关注 acme
	m = bobby.mustStatus("POST", "/orgs/acme/follow", nil, 200)
	if m["followers"].(float64) != 1 || m["is_following"] != true {
		t.Fatalf("follow org = %v", m)
	}
	// 重复关注幂等
	m = bobby.mustStatus("POST", "/orgs/acme/follow", nil, 200)
	if m["followers"].(float64) != 1 {
		t.Fatalf("idempotent follow = %v", m)
	}

	// 粉丝列表
	raw := rawGet(t, bobby, "/orgs/acme/followers")
	if !strings.Contains(raw, "bobby") {
		t.Fatalf("org followers = %s", raw)
	}

	// 主页反映关注状态
	m = bobby.mustStatus("GET", "/orgs/acme/profile", nil, 200)
	if m["followers"].(float64) != 1 || m["is_following"] != true {
		t.Fatalf("profile follow state = %v", m)
	}

	// 组织关注与用户关注相互独立：bobby 的 following 仍为 0
	m = bobby.mustStatus("GET", "/users/bobby", nil, 200)
	if m["following"].(float64) != 0 {
		t.Fatalf("user following should not count org follows = %v", m["following"])
	}

	// 取消关注（幂等）
	m = bobby.mustStatus("DELETE", "/orgs/acme/follow", nil, 200)
	if m["followers"].(float64) != 0 || m["is_following"] != false {
		t.Fatalf("unfollow org = %v", m)
	}
	bobby.mustStatus("DELETE", "/orgs/acme/follow", nil, 200)

	// 不存在的组织
	bobby.mustFail("GET", "/orgs/ghost/profile", nil, 404)
	bobby.mustFail("POST", "/orgs/ghost/follow", nil, 404)
	bobby.mustFail("GET", "/orgs/ghost/followers", nil, 404)
}
