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
	SHA     string `json:"sha"`
	Author  string `json:"author"`
	Message string `json:"message"`
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

	// 空仓库
	alice.mustStatus("POST", "/repos", map[string]string{"name": "empty"}, 201)
	if bs := getJSON[[]Branch](t, alice, "/repos/empty/branches", 200); len(bs) != 0 {
		t.Fatalf("empty repo branches = %+v", bs)
	}
}

func TestBrowsingOwnership(t *testing.T) {
	requireBins(t, "git")
	env := start(t)
	alice := register(t, env, "alice", "alice-pass-123")
	bob := register(t, env, "bob", "bob-pass-123456")

	alice.mustStatus("POST", "/repos", map[string]string{"name": "browsed"}, 201)
	seedCommits(t, env, "alice", "browsed", map[string]string{"README.md": "x"})

	bob.mustFail("GET", "/repos/browsed/tree?ref=main", nil, 404)
	bob.mustFail("GET", "/repos/browsed/blob?ref=main&path=README.md", nil, 404)
	bob.mustFail("GET", "/repos/browsed/commits?ref=main", nil, 404)
}
