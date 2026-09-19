package tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

type Branch struct {
	Name   string `json:"name"`
	IsHead bool   `json:"is_head"`
}

type Entry struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Size int64  `json:"size"`
}

type Commit struct {
	SHA     string   `json:"sha"`
	Author  string   `json:"author"`
	Message string   `json:"message"`
	Parents []string `json:"parents"`
	Refs    []string `json:"refs"`
}

// TestCommitsGraphAndSearch 覆盖提交图数据（parents/refs）与搜索过滤。
func TestCommitsGraphAndSearch(t *testing.T) {
	env := start(t)
	alice := register(t, env, "alice", "alice-pass-123")
	alice.mustStatus("POST", "/repos", map[string]string{"name": "graph"}, 201)

	c1 := writeCommit(t, alice, "alice", "graph", map[string]any{
		"branch":  "main",
		"message": "first commit",
		"changes": []any{map[string]any{"path": "a.txt", "action": "create", "content": "1"}},
	}, 201)["sha"].(string)
	c2 := writeCommit(t, alice, "alice", "graph", map[string]any{
		"branch":  "main",
		"message": "Add Feature X",
		"changes": []any{map[string]any{"path": "b.txt", "action": "create", "content": "2"}},
	}, 201)["sha"].(string)

	cs := getJSON[[]Commit](t, alice, "/repos/graph/commits?ref=main", 200)
	if len(cs) != 2 || cs[0].SHA != c2 || cs[1].SHA != c1 {
		t.Fatalf("commits = %+v", cs)
	}
	if len(cs[0].Parents) != 1 || cs[0].Parents[0] != c1 {
		t.Fatalf("parents = %v, want [%s]", cs[0].Parents, c1)
	}
	if len(cs[1].Parents) != 0 {
		t.Fatalf("root commit should have no parents, got %v", cs[1].Parents)
	}
	foundRef := false
	for _, r := range cs[0].Refs {
		if r == "HEAD -> main" {
			foundRef = true
		}
	}
	if !foundRef {
		t.Fatalf("refs = %v, want HEAD -> main", cs[0].Refs)
	}

	// 搜索：提交信息（大小写不敏感）
	if m := getJSON[[]Commit](t, alice, "/repos/graph/commits?ref=main&q=feature", 200); len(m) != 1 || m[0].SHA != c2 {
		t.Fatalf("message search = %+v", m)
	}
	// 搜索：作者
	if a := getJSON[[]Commit](t, alice, "/repos/graph/commits?ref=main&q=ALICE", 200); len(a) != 2 {
		t.Fatalf("author search = %+v", a)
	}
	// 搜索：sha 前缀
	if s := getJSON[[]Commit](t, alice, "/repos/graph/commits?ref=main&q="+c1[:8], 200); len(s) != 1 || s[0].SHA != c1 {
		t.Fatalf("sha search = %+v", s)
	}
	// 搜索无命中
	if n := getJSON[[]Commit](t, alice, "/repos/graph/commits?ref=main&q=zzz-nope", 200); len(n) != 0 {
		t.Fatalf("no-match search = %+v", n)
	}
}

// seedCommits 用 file:// 协议向 bare 仓库推入初始提交（不依赖 ssh）。
func seedCommits(t *testing.T, env *Env, owner, name string, files map[string]string) {
	t.Helper()
	requireBins(t, "git")

	work := t.TempDir()
	runCmd(t, work, nil, "git", "init", "-q", "-b", "main", ".")
	for path, content := range files {
		full := filepath.Join(work, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runCmd(t, work, nil, "git", "add", "-A")
	runCmd(t, work, nil, "git", "-c", "user.name=seed", "-c", "user.email=seed@example.com", "commit", "-q", "-m", "seed commit")
	runCmd(t, work, nil, "git", "push", "-q", fmt.Sprintf("file://%s", repoOnDisk(env, owner, name)), "main")
}

func getJSON[T any](t *testing.T, c *Client, path string, wantStatus int) T {
	t.Helper()
	req, err := http.NewRequest("GET", c.env.BaseURL+"/api"+path, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != wantStatus {
		t.Fatalf("GET %s: status %d, want %d", path, resp.StatusCode, wantStatus)
	}
	var out T
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return out
}

func TestCodeBrowsing(t *testing.T) {
	requireBins(t, "git")
	env := start(t)
	alice := register(t, env, "alice", "alice-pass-123")

	alice.mustStatus("POST", "/repos", map[string]string{"name": "browsed"}, 201)
	seedCommits(t, env, "alice", "browsed", map[string]string{
		"README.md":        "# browsed repo",
		"src/main.go":      "package main\n",
		"src/util/util.go": "package util\n",
	})

	branches := getJSON[[]Branch](t, alice, "/repos/browsed/branches", 200)
	if len(branches) != 1 || branches[0].Name != "main" || !branches[0].IsHead {
		t.Fatalf("branches = %+v", branches)
	}

	// 根目录
	root := getJSON[map[string]any](t, alice, "/repos/browsed/tree?ref=main", 200)
	entries := root["entries"].([]any)
	if len(entries) != 2 {
		t.Fatalf("root entries = %v", entries)
	}

	// 子目录（main.go + util/）
	sub := getJSON[map[string]any](t, alice, "/repos/browsed/tree?ref=main&path=src", 200)
	subEntries := sub["entries"].([]any)
	if len(subEntries) != 2 {
		t.Fatalf("src entries = %v", subEntries)
	}

	// 文件内容
	blob := getJSON[map[string]any](t, alice, "/repos/browsed/blob?ref=main&path=README.md", 200)
	if blob["content"] != "# browsed repo" || blob["encoding"] != "utf-8" {
		t.Fatalf("blob = %v", blob)
	}

	// 提交历史
	commits := getJSON[[]Commit](t, alice, "/repos/browsed/commits?ref=main", 200)
	if len(commits) != 1 || commits[0].Message != "seed commit" || commits[0].SHA == "" {
		t.Fatalf("commits = %+v", commits)
	}

	// blame
	blame := getJSON[map[string]any](t, alice, "/repos/browsed/blame?ref=main&path=README.md", 200)
	bl, ok := blame["lines"].([]any)
	if !ok || len(bl) != 1 {
		t.Fatalf("blame lines = %v", blame)
	}
	line := bl[0].(map[string]any)
	if line["content"] != "# browsed repo" || line["commit"] == "" {
		t.Fatalf("blame line = %v", line)
	}
	cmts := blame["commits"].(map[string]any)
	if c := cmts[line["commit"].(string)].(map[string]any); c["author"] == "" || c["message"] == "" {
		t.Fatalf("blame commit = %v", c)
	}
	alice.mustFail("GET", "/repos/browsed/blame?ref=main&path=missing.txt", nil, 400)
	alice.mustFail("GET", "/repos/browsed/blame?ref=nope&path=README.md", nil, 400)

	// 异常输入
	alice.mustFail("GET", "/repos/browsed/tree?ref=nope", nil, 400)
	alice.mustFail("GET", "/repos/browsed/blob?ref=main&path=missing.txt", nil, 400)
	alice.mustFail("GET", "/repos/browsed/tree?ref=main&path=../escape", nil, 400)

	// limit 超限时 clamp 到上限（不报错）
	if cs := getJSON[[]Commit](t, alice, "/repos/browsed/commits?ref=main&limit=99999", 200); len(cs) != 1 {
		t.Fatalf("clamped commits = %+v", cs)
	}

	// 空仓库：浏览接口应返回空结果，而不是泄漏原始 git 报错
	alice.mustStatus("POST", "/repos", map[string]string{"name": "empty"}, 201)
	if bs := getJSON[[]Branch](t, alice, "/repos/empty/branches", 200); len(bs) != 0 {
		t.Fatalf("empty repo branches = %+v", bs)
	}
	// ref 省略时回退到默认分支（main），空仓库仍返回空目录
	if et := getJSON[map[string]any](t, alice, "/repos/empty/tree", 200); len(et["entries"].([]any)) != 0 {
		t.Fatalf("empty repo tree = %v", et)
	}
	if et := getJSON[map[string]any](t, alice, "/repos/empty/tree?ref=main", 200); len(et["entries"].([]any)) != 0 {
		t.Fatalf("empty repo tree(ref=main) = %v", et)
	}
	if cs := getJSON[[]Commit](t, alice, "/repos/empty/commits?ref=main", 200); len(cs) != 0 {
		t.Fatalf("empty repo commits = %+v", cs)
	}
	alice.mustFail("GET", "/repos/empty/blob?ref=main&path=README.md", nil, 404)
	alice.mustFail("GET", "/repos/empty/blame?ref=main&path=README.md", nil, 404)
}

func TestBrowsingOwnership(t *testing.T) {
	requireBins(t, "git")
	env := start(t)
	alice := register(t, env, "alice", "alice-pass-123")
	bob := register(t, env, "bobby", "bob-pass-123456")

	alice.mustStatus("POST", "/repos", map[string]string{"name": "browsed"}, 201)
	seedCommits(t, env, "alice", "browsed", map[string]string{"README.md": "x"})

	bob.mustFail("GET", "/repos/browsed/tree?ref=main", nil, 404)
	bob.mustFail("GET", "/repos/browsed/blob?ref=main&path=README.md", nil, 404)
	bob.mustFail("GET", "/repos/browsed/commits?ref=main", nil, 404)
}
