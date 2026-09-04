package tests

import (
	"encoding/json"
	"net/http"
	"testing"
)

type Issue struct {
	Number   int64   `json:"number"`
	Title    string  `json:"title"`
	Body     string  `json:"body"`
	State    string  `json:"state"`
	Author   string  `json:"author"`
	ClosedAt *string `json:"closed_at"`
}

// listIssues 请求数组形式的 issue 列表。
func listIssues(t *testing.T, c *Client, repo string) []Issue {
	t.Helper()
	req, err := http.NewRequest("GET", c.env.BaseURL+"/api/repos/"+repo+"/issues", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("list issues: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("list issues: status %d", resp.StatusCode)
	}
	var issues []Issue
	if err := json.NewDecoder(resp.Body).Decode(&issues); err != nil {
		t.Fatalf("decode issues: %v", err)
	}
	return issues
}

func TestIssueLifecycle(t *testing.T) {
	env := start(t)
	alice := register(t, env, "alice", "alice-pass-123")
	alice.mustStatus("POST", "/repos", map[string]string{"name": "demo"}, 201)

	// 空标题拒绝
	alice.mustFail("POST", "/repos/demo/issues", map[string]string{"title": "  "}, 400)

	// 创建两个 issue，number 递增
	m1 := alice.mustStatus("POST", "/repos/demo/issues",
		map[string]string{"title": "bug: crash on login", "body": "steps: 1. login"}, 201)
	if m1["number"] != float64(1) || m1["state"] != "open" || m1["author"] != "alice" {
		t.Fatalf("create #1 = %v", m1)
	}
	m2 := alice.mustStatus("POST", "/repos/demo/issues",
		map[string]string{"title": "feature: dark mode"}, 201)
	if m2["number"] != float64(2) {
		t.Fatalf("create #2 = %v", m2)
	}

	// 关闭 -> 重新打开
	closed := alice.mustStatus("PATCH", "/repos/demo/issues/1",
		map[string]string{"state": "closed"}, 200)
	if closed["state"] != "closed" || closed["closed_at"] == nil {
		t.Fatalf("close = %v", closed)
	}
	reopened := alice.mustStatus("PATCH", "/repos/demo/issues/1",
		map[string]string{"state": "open"}, 200)
	if reopened["state"] != "open" || reopened["closed_at"] != nil {
		t.Fatalf("reopen = %v", reopened)
	}

	// 非法 state / 不存在的 issue
	alice.mustFail("PATCH", "/repos/demo/issues/1", map[string]string{"state": "weird"}, 400)
	alice.mustFail("PATCH", "/repos/demo/issues/99", map[string]string{"state": "closed"}, 404)

	// 列表：open 在前
	issues := listIssues(t, alice, "demo")
	if len(issues) != 2 || issues[0].Number != 2 || issues[0].State != "open" {
		t.Fatalf("issues = %+v", issues)
	}
}

func TestIssueDeletedWithRepo(t *testing.T) {
	env := start(t)
	alice := register(t, env, "alice", "alice-pass-123")
	alice.mustStatus("POST", "/repos", map[string]string{"name": "demo"}, 201)
	alice.mustStatus("POST", "/repos/demo/issues", map[string]string{"title": "x"}, 201)

	alice.mustStatus("DELETE", "/repos/demo", nil, 204)
	alice.mustFail("GET", "/repos/demo/issues", nil, 404)

	// 同名重建后不残留旧 issue
	alice.mustStatus("POST", "/repos", map[string]string{"name": "demo"}, 201)
	if got := len(listIssues(t, alice, "demo")); got != 0 {
		t.Fatalf("issues survived repo delete: %+v", listIssues(t, alice, "demo"))
	}
}

func TestIssueRequiresRepo(t *testing.T) {
	env := start(t)
	alice := register(t, env, "alice", "alice-pass-123")
	alice.mustFail("GET", "/repos/nope/issues", nil, 404)
	alice.mustFail("POST", "/repos/nope/issues", map[string]string{"title": "x"}, 404)
	alice.mustFail("PATCH", "/repos/nope/issues/1", map[string]string{"state": "closed"}, 404)
}

func TestIssueUserIsolation(t *testing.T) {
	env := start(t)
	alice := register(t, env, "alice", "alice-pass-123")
	bob := register(t, env, "bob", "bob-pass-123456")

	alice.mustStatus("POST", "/repos", map[string]string{"name": "demo"}, 201)
	alice.mustStatus("POST", "/repos/demo/issues", map[string]string{"title": "alice's bug"}, 201)

	// bob 看不到也改不了 alice 仓库里的 issue（仓库 404 优先）
	bob.mustFail("GET", "/repos/demo/issues", nil, 404)
	bob.mustFail("PATCH", "/repos/demo/issues/1", map[string]string{"state": "closed"}, 404)

	// 同名仓库各自独立
	bob.mustStatus("POST", "/repos", map[string]string{"name": "demo"}, 201)
	bob.mustStatus("POST", "/repos/demo/issues", map[string]string{"title": "bob's bug"}, 201)
	if got := len(listIssues(t, bob, "demo")); got != 1 {
		t.Fatalf("bob issues = %d", got)
	}
	if got := len(listIssues(t, alice, "demo")); got != 1 {
		t.Fatalf("alice issues = %d", got)
	}
}
