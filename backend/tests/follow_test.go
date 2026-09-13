package tests

import "testing"

func TestUserFollow(t *testing.T) {
	env := start(t)
	alice := register(t, env, "alice", "alice-pass-123")
	bobby := register(t, env, "bobby", "bob-pass-123456")

	// bobby 关注 alice
	m := bobby.mustStatus("POST", "/users/alice/follow", nil, 200)
	if m["followers"].(float64) != 1 || m["is_following"] != true {
		t.Fatalf("follow = %v", m)
	}
	// 重复关注幂等
	m = bobby.mustStatus("POST", "/users/alice/follow", nil, 200)
	if m["followers"].(float64) != 1 {
		t.Fatalf("idempotent follow = %v", m)
	}

	// alice 视角主页：1 个粉丝、0 个关注、is_following=false（不能关注自己）
	m = alice.mustStatus("GET", "/users/alice", nil, 200)
	if m["username"] != "alice" || m["followers"].(float64) != 1 || m["following"].(float64) != 0 {
		t.Fatalf("alice profile = %v", m)
	}
	if m["is_following"] != false {
		t.Fatalf("self is_following = %v", m["is_following"])
	}
	// bobby 视角主页
	m = bobby.mustStatus("GET", "/users/alice", nil, 200)
	if m["is_following"] != true {
		t.Fatalf("bobby view = %v", m)
	}

	// 不能关注自己 / 不能关注不存在的用户
	alice.mustFail("POST", "/users/alice/follow", nil, 400)
	bobby.mustFail("POST", "/users/ghost/follow", nil, 404)
	alice.mustFail("GET", "/users/ghost", nil, 404)
	alice.mustFail("GET", "/users/ghost/followers", nil, 404)

	// 粉丝 / 关注列表
	bobby.mustStatus("GET", "/users/alice/followers", nil, 200)
	bobby.mustStatus("GET", "/users/alice/following", nil, 200)

	// 取消关注（幂等）
	m = bobby.mustStatus("DELETE", "/users/alice/follow", nil, 200)
	if m["followers"].(float64) != 0 || m["is_following"] != false {
		t.Fatalf("unfollow = %v", m)
	}
	bobby.mustStatus("DELETE", "/users/alice/follow", nil, 200)
}

func TestUserProfileRepoVisibility(t *testing.T) {
	env := start(t)
	alice := register(t, env, "alice", "alice-pass-123")
	bobby := register(t, env, "bobby", "bob-pass-123456")

	alice.mustStatus("POST", "/repos", map[string]string{"name": "pub"}, 201)
	alice.mustStatus("POST", "/repos", map[string]string{"name": "priv"}, 201)
	alice.mustStatus("POST", "/users/alice/repos/pub/visibility", map[string]bool{"private": false}, 200)

	// 他人只看到公开仓库
	m := bobby.mustStatus("GET", "/users/alice", nil, 200)
	if repos, _ := m["repos"].([]any); len(repos) != 1 {
		t.Fatalf("public profile repos = %v", m["repos"])
	}
	// 本人看到全部
	m = alice.mustStatus("GET", "/users/alice", nil, 200)
	if repos, _ := m["repos"].([]any); len(repos) != 2 {
		t.Fatalf("own profile repos = %v", m["repos"])
	}
}
