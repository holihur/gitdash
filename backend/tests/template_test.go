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
	defer func() { _ = resp.Body.Close() }()
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

	// 默认模版同时写入示例流水线 .gitdash.yml
	p := alice.mustStatus("GET", "/repos/tpl/blob?ref=main&path=.gitdash.yml", nil, 200)
	if !containsStr(p["content"].(string), "Hello from gitdash pipeline") {
		t.Fatalf(".gitdash.yml content = %q", p["content"])
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

func TestRepoConvertToTemplateAndUse(t *testing.T) {
	env := start(t)
	alice := register(t, env, "alice", "alice-pass-123")

	// 创建源仓库并写入 README
	alice.mustStatus("POST", "/repos",
		map[string]string{"name": "base", "template": "readme"}, 201)

	// 非模版仓库不能作为模版源
	alice.mustFail("POST", "/repos", map[string]any{
		"name": "bad", "template_owner": "alice", "template_name": "base",
	}, 400)

	// 转为模版仓库
	m := alice.mustStatus("POST", "/users/alice/repos/base/template",
		map[string]bool{"is_template": true}, 200)
	if m["is_template"] != true {
		t.Fatalf("is_template = %v", m["is_template"])
	}

	// 模版仓库出现在 /templates 列表
	tplsRaw := rawGet(t, alice, "/templates")
	if !containsStr(tplsRaw, `"name":"base"`) {
		t.Fatalf("templates = %s", tplsRaw)
	}

	// 从模版仓库创建新仓库（克隆内容）
	alice.mustStatus("POST", "/repos", map[string]any{
		"name": "from-tpl", "template_owner": "alice", "template_name": "base",
	}, 201)

	// 新仓库继承了源仓库内容
	b := alice.mustStatus("GET", "/repos/from-tpl/blob?ref=main&path=README.md", nil, 200)
	if b["content"] != "# base\n" {
		t.Fatalf("cloned readme content = %q", b["content"])
	}

	// 新仓库默认不是模版
	got := alice.mustStatus("GET", "/repos/from-tpl", nil, 200)
	if got["is_template"] != false {
		t.Fatalf("new repo should not be template: %v", got)
	}

	// 取消模版标记
	m = alice.mustStatus("POST", "/users/alice/repos/base/template",
		map[string]bool{"is_template": false}, 200)
	if m["is_template"] != false {
		t.Fatalf("is_template after off = %v", m["is_template"])
	}
	// 取消后不能再作为模版源
	alice.mustFail("POST", "/repos", map[string]any{
		"name": "bad2", "template_owner": "alice", "template_name": "base",
	}, 400)
}
