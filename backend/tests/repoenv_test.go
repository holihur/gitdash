package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
)

type repoEnvVar struct {
	Key       string `json:"key"`
	Value     string `json:"value"`
	CreatedAt string `json:"created_at"`
}

// listRepoEnv 请求仓库环境变量数组。
func listRepoEnv(t *testing.T, c *Client, owner, repo string) []repoEnvVar {
	t.Helper()
	req, err := http.NewRequest("GET", c.env.BaseURL+"/api/users/"+owner+"/repos/"+repo+"/env", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("list env: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		t.Fatalf("list env: status %d", resp.StatusCode)
	}
	var vars []repoEnvVar
	if err := json.NewDecoder(resp.Body).Decode(&vars); err != nil {
		t.Fatalf("decode env: %v", err)
	}
	return vars
}

func setRepoEnv(t *testing.T, c *Client, owner, repo, key, value string) []repoEnvVar {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"key": key, "value": value})
	req, err := http.NewRequest("PUT", c.env.BaseURL+"/api/users/"+owner+"/repos/"+repo+"/env", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("set env: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		t.Fatalf("set env: status %d", resp.StatusCode)
	}
	var vars []repoEnvVar
	if err := json.NewDecoder(resp.Body).Decode(&vars); err != nil {
		t.Fatalf("decode env: %v", err)
	}
	return vars
}

func TestRepoEnvVarsCRUD(t *testing.T) {
	env := start(t)
	alice := register(t, env, "alice", "alice-pass-123")
	alice.mustStatus("POST", "/repos", map[string]string{"name": "demo"}, 201)

	// 初始为空
	if got := listRepoEnv(t, alice, "alice", "demo"); len(got) != 0 {
		t.Fatalf("initial env = %v", got)
	}

	// 新增
	vars := setRepoEnv(t, alice, "alice", "demo", "GOFLAGS", "-mod=mod")
	if len(vars) != 1 || vars[0].Key != "GOFLAGS" || vars[0].Value != "-mod=mod" {
		t.Fatalf("after set = %v", vars)
	}

	// 覆盖（upsert）
	vars = setRepoEnv(t, alice, "alice", "demo", "GOFLAGS", "-race")
	if len(vars) != 1 || vars[0].Value != "-race" {
		t.Fatalf("after overwrite = %v", vars)
	}

	// 第二个变量
	setRepoEnv(t, alice, "alice", "demo", "CGO_ENABLED", "0")
	vars = listRepoEnv(t, alice, "alice", "demo")
	if len(vars) != 2 {
		t.Fatalf("expected 2 vars, got %v", vars)
	}

	// 非法 key 拒绝
	alice.mustFail("PUT", "/users/alice/repos/demo/env", map[string]string{"key": "1BAD", "value": "x"}, 400)

	// 删除
	code, _ := alice.do("DELETE", "/users/alice/repos/demo/env/GOFLAGS", nil)
	if code != 200 {
		t.Fatalf("delete env: status %d", code)
	}
	if vars := listRepoEnv(t, alice, "alice", "demo"); len(vars) != 1 || vars[0].Key != "CGO_ENABLED" {
		t.Fatalf("after delete = %v", vars)
	}

	// 删除不存在的变量 → 404
	alice.mustFail("DELETE", "/users/alice/repos/demo/env/NOPE", nil, 404)
}

func TestRepoEnvVarOwnerOnly(t *testing.T) {
	env := start(t)
	alice := register(t, env, "alice", "alice-pass-123")
	bob := register(t, env, "bob", "bob-pass-123")
	alice.mustStatus("POST", "/repos", map[string]string{"name": "secret-repo"}, 201)
	setRepoEnv(t, alice, "alice", "secret-repo", "TOKEN", "s3cr3t")

	// bob 读/写/删一律 404（无权限与不存在同语义）
	bob.mustFail("GET", "/users/alice/repos/secret-repo/env", nil, 404)
	bob.mustFail("PUT", "/users/alice/repos/secret-repo/env", map[string]string{"key": "X", "value": "1"}, 404)
	bob.mustFail("DELETE", "/users/alice/repos/secret-repo/env/TOKEN", nil, 404)
}

func TestRepoEnvVarCascadeOnDelete(t *testing.T) {
	env := start(t)
	alice := register(t, env, "alice", "alice-pass-123")
	alice.mustStatus("POST", "/repos", map[string]string{"name": "doomed"}, 201)
	setRepoEnv(t, alice, "alice", "doomed", "FOO", "bar")

	vars, err := env.Store.ListRepoEnvVars("alice", "doomed")
	if err != nil || len(vars) != 1 {
		t.Fatalf("pre-delete store vars = %v, err = %v", vars, err)
	}

	alice.mustStatus("DELETE", "/repos/doomed", nil, 204)

	vars, err = env.Store.ListRepoEnvVars("alice", "doomed")
	if err != nil || len(vars) != 0 {
		t.Fatalf("post-delete store vars = %v, err = %v", vars, err)
	}
}
