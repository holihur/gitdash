package tests

import "testing"

func TestRepoPinsAPI(t *testing.T) {
	env := start(t)
	alice := register(t, env, "alice", "alice-pass-123")

	for _, name := range []string{"alpha", "beta", "gamma"} {
		alice.mustStatus("POST", "/repos", map[string]any{"name": name, "private": false}, 201)
	}

	// 初始为空
	list := alice.mustStatus("GET", "/me/pins", nil, 200)
	if arr, ok := list["pins"].([]any); !ok || len(arr) != 0 {
		t.Fatalf("initial pins = %v", list)
	}
	if list["limit"].(float64) != 6 {
		t.Fatalf("pins limit = %v", list["limit"])
	}

	// 置顶 gamma
	pinned := alice.mustStatus("POST", "/me/pins", map[string]string{"repo": "gamma"}, 200)
	arr, _ := pinned["pins"].([]any)
	if len(arr) != 1 {
		t.Fatalf("pins after pin = %v", pinned)
	}
	first, _ := arr[0].(map[string]any)
	if first["name"] != "gamma" || first["pinned"] != true {
		t.Fatalf("pinned repo = %v", first)
	}

	// 幂等：再次置顶数量不变
	pinned = alice.mustStatus("POST", "/me/pins", map[string]string{"repo": "gamma"}, 200)
	if arr, _ := pinned["pins"].([]any); len(arr) != 1 {
		t.Fatalf("idempotent pins = %v", pinned)
	}

	// 不存在的仓库
	alice.mustFail("POST", "/me/pins", map[string]string{"repo": "nope"}, 404)
	// 缺少仓库名
	alice.mustFail("POST", "/me/pins", map[string]string{"repo": ""}, 400)

	// 主页：置顶仓库排在最前并带 pinned 标记
	profile := alice.mustStatus("GET", "/users/alice", nil, 200)
	repos, _ := profile["repos"].([]any)
	if len(repos) == 0 {
		t.Fatalf("profile repos = %v", profile)
	}
	top, _ := repos[0].(map[string]any)
	if top["name"] != "gamma" || top["pinned"] != true {
		t.Fatalf("profile first repo = %v", top)
	}

	// 他人视角同样置顶（公开仓库）
	bob := register(t, env, "bobby", "bobby-pass-1234")
	bobProfile := bob.mustStatus("GET", "/users/alice", nil, 200)
	bobRepos, _ := bobProfile["repos"].([]any)
	btop, _ := bobRepos[0].(map[string]any)
	if btop["name"] != "gamma" || btop["pinned"] != true {
		t.Fatalf("bob view first repo = %v", btop)
	}

	// 不能置顶他人仓库
	alice.mustFail("POST", "/me/pins",
		map[string]string{"owner": "bobby", "repo": "bobby"}, 403)

	// 取消置顶
	unpinned := alice.mustStatus("DELETE", "/me/pins/alice/gamma", nil, 200)
	if arr, _ := unpinned["pins"].([]any); len(arr) != 0 {
		t.Fatalf("pins after unpin = %v", unpinned)
	}
	// 幂等
	alice.mustStatus("DELETE", "/me/pins/alice/gamma", nil, 200)

	// 未认证
	(&Client{env: env}).mustFail("GET", "/me/pins", nil, 401)
}

func TestRepoPinsLimit(t *testing.T) {
	env := start(t)
	alice := register(t, env, "lslimit", "alice-pass-123")
	names := []string{"r1", "r2", "r3", "r4", "r5", "r6", "r7"}
	for _, name := range names {
		alice.mustStatus("POST", "/repos", map[string]any{"name": name, "private": true}, 201)
	}
	for _, name := range names[:6] {
		alice.mustStatus("POST", "/me/pins", map[string]string{"repo": name}, 200)
	}
	alice.mustFail("POST", "/me/pins", map[string]string{"repo": names[6]}, 409)

	// 取消一个后可再置顶
	alice.mustStatus("DELETE", "/me/pins/lslimit/r1", nil, 200)
	alice.mustStatus("POST", "/me/pins", map[string]string{"repo": names[6]}, 200)
}
