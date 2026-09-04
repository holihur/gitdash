package tests

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func prPath(owner, repo, suffix string) string {
	return fmt.Sprintf("/users/%s/repos/%s/pulls%s", owner, repo, suffix)
}

func gitDo(t *testing.T, dir string, key string, args ...string) {
	t.Helper()
	runCmd(t, dir, sshEnv(key), "git", args...)
}

func TestPullRequestLifecycle(t *testing.T) {
	requireBins(t, "git", "ssh", "ssh-keygen")
	env := start(t)
	alice := register(t, env, "alice", "alice-pass-123")
	alice.mustStatus("POST", "/repos", map[string]string{"name": "pr"}, 201)

	key, pub := genKey(t, env.DataDir, "alice_key")
	alice.mustStatus("POST", "/keys", map[string]string{"name": "e2e", "public_key": pub}, 201)

	work := t.TempDir()
	gitDo(t, work, key, "clone", "-q", fmt.Sprintf("ssh://git@127.0.0.1:%s/pr.git", env.SSHPort), "pr")
	repo := filepath.Join(work, "pr")

	// main 上第一个提交
	write := func(f, body string) {
		if err := os.WriteFile(filepath.Join(repo, f), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		gitDo(t, repo, key, "add", "-A")
		gitDo(t, repo, key, "-c", "user.name=alice", "-c", "user.email=a@b.c", "commit", "-q", "-m", f)
	}
	write("base.txt", "base")
	gitDo(t, repo, key, "push", "-q", "origin", "HEAD")

	// 功能分支 feat
	gitDo(t, repo, key, "checkout", "-q", "-b", "feat")
	write("feature.txt", "feature change\nline2")
	gitDo(t, repo, key, "push", "-q", "-u", "origin", "feat")

	// 创建 PR
	m := alice.mustStatus("POST", prPath("alice", "pr", ""),
		map[string]string{"title": "add feature", "body": "desc", "source_branch": "feat", "target_branch": "main"}, 201)
	num := int64(m["number"].(float64))
	if num != 1 || m["state"] != "open" || m["head_sha"] == "" {
		t.Fatalf("create pr = %v", m)
	}

	// 列表 & 详情
	listRaw := rawGet(t, alice, prPath("alice", "pr", ""))
	var list []map[string]any
	if err := json.Unmarshal([]byte(listRaw), &list); err != nil || len(list) != 1 {
		t.Fatalf("list pulls = %s err=%v", listRaw, err)
	}
	alice.mustStatus("GET", prPath("alice", "pr", "/1"), nil, 200)

	// diff：文件与补丁
	diffRaw := rawGet(t, alice, prPath("alice", "pr", "/1/diff"))
	var diff struct {
		Files []map[string]any `json:"files"`
		Patch string           `json:"patch"`
	}
	if err := json.Unmarshal([]byte(diffRaw), &diff); err != nil {
		t.Fatalf("diff decode: %v (%s)", err, diffRaw)
	}
	if len(diff.Files) != 1 || diff.Files[0]["path"] != "feature.txt" || diff.Files[0]["status"] != "A" {
		t.Fatalf("diff files = %v", diff.Files)
	}
	if !containsStr(diff.Patch, "+feature change") {
		t.Fatalf("patch missing content: %s", diff.Patch)
	}

	// 关闭 / 重开
	closed := alice.mustStatus("POST", prPath("alice", "pr", "/1/state"), map[string]string{"state": "closed"}, 200)
	if closed["state"] != "closed" {
		t.Fatalf("close = %v", closed)
	}
	alice.mustStatus("POST", prPath("alice", "pr", "/1/state"), map[string]string{"state": "open"}, 200)

	// 合并（fast-forward）
	merged := alice.mustStatus("POST", prPath("alice", "pr", "/1/merge"), nil, 200)
	if merged["state"] != "merged" || merged["merged_by"] != "alice" || merged["merged_at"] == nil {
		t.Fatalf("merge = %v", merged)
	}
	alice.mustFail("POST", prPath("alice", "pr", "/1/merge"), nil, 409) // 已合并不可再合
	// 主分支已包含 feature 提交
	commitsRaw := rawGet(t, alice, "/repos/pr/commits?ref=main")
	if !containsStr(commitsRaw, "feature.txt") {
		t.Fatalf("main missing merged commit: %s", commitsRaw)
	}
	// merged 不能再改 state
	alice.mustFail("POST", prPath("alice", "pr", "/1/state"), map[string]string{"state": "closed"}, 400)
}

func TestPullRequestBadPaths(t *testing.T) {
	requireBins(t, "git", "ssh", "ssh-keygen")
	env := start(t)
	alice := register(t, env, "alice", "alice-pass-123")
	bob := register(t, env, "bob", "bob-pass-123456")
	alice.mustStatus("POST", "/repos", map[string]string{"name": "pr"}, 201)

	key, pub := genKey(t, env.DataDir, "alice_key")
	alice.mustStatus("POST", "/keys", map[string]string{"name": "e2e", "public_key": pub}, 201)

	work := t.TempDir()
	gitDo(t, work, key, "clone", "-q", fmt.Sprintf("ssh://git@127.0.0.1:%s/pr.git", env.SSHPort), "pr")
	repo := filepath.Join(work, "pr")
	if err := os.WriteFile(filepath.Join(repo, "m.txt"), []byte("m"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitDo(t, repo, key, "add", "-A")
	gitDo(t, repo, key, "-c", "user.name=alice", "-c", "user.email=a@b.c", "commit", "-q", "-m", "m")
	gitDo(t, repo, key, "push", "-q", "origin", "HEAD")

	// 分支 a：与 main 分叉（main 再前进一次）
	gitDo(t, repo, key, "checkout", "-q", "-b", "branch-a")
	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitDo(t, repo, key, "add", "-A")
	gitDo(t, repo, key, "-c", "user.name=alice", "-c", "user.email=a@b.c", "commit", "-q", "-m", "a")
	gitDo(t, repo, key, "push", "-q", "-u", "origin", "branch-a")

	gitDo(t, repo, key, "checkout", "-q", "main")
	if err := os.WriteFile(filepath.Join(repo, "m2.txt"), []byte("m2"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitDo(t, repo, key, "add", "-A")
	gitDo(t, repo, key, "-c", "user.name=alice", "-c", "user.email=a@b.c", "commit", "-q", "-m", "m2")
	gitDo(t, repo, key, "push", "-q", "origin", "HEAD")

	// 分叉后不可 fast-forward 合并
	alice.mustStatus("POST", prPath("alice", "pr", ""),
		map[string]string{"title": "x", "source_branch": "branch-a", "target_branch": "main"}, 201)
	alice.mustFail("POST", prPath("alice", "pr", "/1/merge"), nil, 409)

	// 各种 bad path
	alice.mustFail("POST", prPath("alice", "pr", ""),
		map[string]string{"title": "  ", "source_branch": "branch-a", "target_branch": "main"}, 400)
	alice.mustFail("POST", prPath("alice", "pr", ""),
		map[string]string{"title": "t", "source_branch": "main", "target_branch": "main"}, 400)
	alice.mustFail("POST", prPath("alice", "pr", ""),
		map[string]string{"title": "t", "source_branch": "nope", "target_branch": "main"}, 400)
	alice.mustFail("POST", prPath("alice", "pr", ""),
		map[string]string{"title": "t", "source_branch": "main", "target_branch": "nope"}, 400)
	alice.mustFail("GET", prPath("alice", "pr", "/999"), nil, 404)
	// bob 无权限（未共享）
	bob.mustFail("GET", prPath("alice", "pr", ""), nil, 404)
	bob.mustFail("POST", prPath("alice", "pr", ""), map[string]string{"title": "x"}, 404)
}

func containsStr(s, sub string) bool {
	return len(sub) > 0 && (len(s) >= len(sub)) && (s == sub || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
