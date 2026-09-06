package tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

// TestGlobalSearch 全局搜索：公开仓库 / 自己私有仓库 issue / 用户均可命中；他人私有仓库不可见。
func TestGlobalSearch(t *testing.T) {
	env := start(t)
	alice := register(t, env, "alice", "alice-pass-123")
	bob := register(t, env, "bob", "bob-pass-123456")

	alice.mustStatus("POST", "/repos", map[string]any{"name": "needle-pub", "private": false}, 201)
	alice.mustStatus("POST", "/repos", map[string]any{"name": "needle-priv"}, 201)
	// 公开仓库里建 issue
	alice.mustStatus("POST", "/users/alice/repos/needle-pub/issues",
		map[string]string{"title": "broken needle widget"}, 201)
	// 私有仓库里建 issue（只有 alice 能搜到）
	alice.mustStatus("POST", "/users/alice/repos/needle-priv/issues",
		map[string]string{"title": "private needle secret"}, 201)

	global := func(c *Client, q string) map[string]any {
		req, _ := http.NewRequest("GET", c.env.BaseURL+"/api/search?q="+q, nil)
		req.Header.Set("Authorization", "Bearer "+c.token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("search: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != 200 {
			t.Fatalf("search status %d", resp.StatusCode)
		}
		var m map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return m
	}
	countRepos := func(m map[string]any) int { return len(m["repos"].([]any)) }
	countIssues := func(m map[string]any) int { return len(m["issues"].([]any)) }
	countUsers := func(m map[string]any) int { return len(m["users"].([]any)) }

	// alice：1 个公开仓库命中（私有仓库不进全局 repo 搜索）+ 2 个 issue
	m := global(alice, "needle")
	if countRepos(m) != 1 || countIssues(m) != 2 {
		t.Fatalf("alice search = repos %d issues %d users %d", countRepos(m), countIssues(m), countUsers(m))
	}
	// bob：只看到公开仓库与公开 issue + 用户名命中（alice/bob）
	m = global(bob, "needle")
	if countRepos(m) != 1 || countIssues(m) != 1 {
		t.Fatalf("bob search = repos %d issues %d", countRepos(m), countIssues(m))
	}
	// 用户搜索
	m = global(alice, "bob")
	if countUsers(m) != 1 {
		t.Fatalf("user search = %d", countUsers(m))
	}
	// 缺 q 参数
	alice.mustFail("GET", "/search", nil, 400)
	// 未登录
	req, _ := http.NewRequest("GET", env.BaseURL+"/api/search?q=x", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("unauthenticated search = %d", resp.StatusCode)
	}
	fmt.Println("global search ok")
}
