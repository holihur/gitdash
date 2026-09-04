package tests

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestRepoCreateFromReadmeTemplate(t *testing.T) {
	env := start(t)
	alice := register(t, env, "alice", "alice-pass-123")

	m := alice.mustStatus("POST", "/repos",
		map[string]string{"name": "tpl", "template": "readme"}, 201)
	if m["name"] != "tpl" {
		t.Fatalf("create = %v", m)
	}

	// 分支存在（非空仓库）
	req, _ := http.NewRequest("GET", env.BaseURL+"/api/repos/tpl/branches", nil)
	req.Header.Set("Authorization", "Bearer "+alice.token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var branches []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&branches); err != nil || len(branches) != 1 {
		t.Fatalf("branches = %v err=%v", branches, err)
	}
	if branches[0]["name"] != "main" {
		t.Fatalf("default branch = %v", branches[0])
	}

	// README.md 内容由仓库名生成
	b := alice.mustStatus("GET", "/repos/tpl/blob?ref=main&path=README.md", nil, 200)
	if b["content"] != "# tpl\n" {
		t.Fatalf("readme content = %q", b["content"])
	}

	// commits 有一条 Initial commit
	csRaw := rawGet(t, alice, "/repos/tpl/commits?ref=main")
	if !containsStr(csRaw, "Initial commit") {
		t.Fatalf("commits = %s", csRaw)
	}
}

func TestRepoCreateTemplateValidation(t *testing.T) {
	env := start(t)
	alice := register(t, env, "alice", "alice-pass-123")

	// 非法 template
	alice.mustFail("POST", "/repos",
		map[string]string{"name": "x1", "template": "nope"}, 400)
	// 失败后不留仓库与磁盘目录
	alice.mustFail("GET", "/repos/x1", nil, 404)

	// 空 template（默认）仍是空仓库
	alice.mustStatus("POST", "/repos",
		map[string]string{"name": "empty"}, 201)
	empty := rawGet(t, alice, "/repos/empty/branches")
	if containsStr(empty, "main") {
		t.Fatalf("empty repo should have no branches: %s", empty)
	}
}
