package tests

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

// TestPullRequestNonFFMergeAndCommitDiff 覆盖 merge/squash 合并与 commit diff 接口。
func TestPullRequestNonFFMergeAndCommitDiff(t *testing.T) {
	requireBins(t, "git", "ssh", "ssh-keygen")
	env := start(t)
	alice := register(t, env, "alice", "alice-pass-123")
	alice.mustStatus("POST", "/repos", map[string]string{"name": "pr2"}, 201)

	key, pub := genKey(t, env.DataDir, "alice_key")
	alice.mustStatus("POST", "/keys", map[string]string{"name": "e2e", "public_key": pub}, 201)

	work := t.TempDir()
	gitDo(t, work, key, "clone", "-q", fmt.Sprintf("ssh://git@127.0.0.1:%s/pr2.git", env.SSHPort), "pr")
	repo := filepath.Join(work, "pr")
	comm := func(msg string) {
		gitDo(t, repo, key, "add", "-A")
		gitDo(t, repo, key, "-c", "user.name=alice", "-c", "user.email=a@b.c", "commit", "-q", "-m", msg)
	}
	write := func(f, body string) {
		if err := os.WriteFile(filepath.Join(repo, f), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("base.txt", "base")
	comm("base")
	gitDo(t, repo, key, "push", "-q", "origin", "HEAD")

	// 分支 a 与 main 分叉
	gitDo(t, repo, key, "checkout", "-q", "-b", "branch-a")
	write("a.txt", "change from a")
	comm("a work")
	gitDo(t, repo, key, "push", "-q", "-u", "origin", "branch-a")
	gitDo(t, repo, key, "checkout", "-q", "main")
	write("m.txt", "main work")
	comm("main work")
	gitDo(t, repo, key, "push", "-q", "origin", "HEAD")

	// 无 method：409；squash：成功且目标含改动
	alice.mustStatus("POST", prPath("alice", "pr2", ""),
		map[string]string{"title": "sq", "source_branch": "branch-a", "target_branch": "main"}, 201)
	alice.mustFail("POST", prPath("alice", "pr2", "/1/merge"), nil, 409)
	m := alice.mustStatus("POST", prPath("alice", "pr2", "/1/merge"),
		map[string]string{"method": "squash"}, 200)
	if m["state"] != "merged" {
		t.Fatalf("squash merge = %v", m)
	}
	if !containsStr(rawGet(t, alice, "/repos/pr2/commits?ref=main"), "Merge pull request") {
		t.Fatal("squash 后应存在 Merge pull request 提交")
	}
	gitDo(t, repo, key, "fetch", "-q", "origin")
	out := runCmd(t, repo, nil, "git", "show", "origin/main:a.txt")
	if !strings.Contains(out, "change from a") {
		t.Fatalf("squash 后 main 缺少改动: %q", out)
	}

	// 新分支再次分叉后用 merge（merge commit）合并
	gitDo(t, repo, key, "fetch", "-q", "origin")
	gitDo(t, repo, key, "checkout", "-q", "-B", "main", "origin/main")
	gitDo(t, repo, key, "checkout", "-q", "-b", "branch-b")
	write("b.txt", "b content")
	comm("b work")
	gitDo(t, repo, key, "push", "-q", "-u", "origin", "branch-b")
	gitDo(t, repo, key, "checkout", "-q", "main")
	write("m2.txt", "main again")
	comm("main again")
	gitDo(t, repo, key, "push", "-q", "origin", "HEAD")
	alice.mustStatus("POST", prPath("alice", "pr2", ""),
		map[string]string{"title": "mc", "source_branch": "branch-b", "target_branch": "main"}, 201)
	merged := alice.mustStatus("POST", prPath("alice", "pr2", "/2/merge"),
		map[string]string{"method": "merge"}, 200)
	if merged["state"] != "merged" {
		t.Fatalf("merge commit = %v", merged)
	}
	// 合并后存在 merge commit（提交数比 squash 场景多）
	if !containsStr(rawGet(t, alice, "/repos/pr2/commits?ref=main"), "Merge pull request") {
		t.Fatal("merge commit 缺失")
	}
	// 非法 method
	gitDo(t, repo, key, "fetch", "-q", "origin")
	gitDo(t, repo, key, "checkout", "-q", "-B", "main", "origin/main")
	gitDo(t, repo, key, "checkout", "-q", "-b", "branch-c")
	write("c.txt", "c")
	comm("c work")
	gitDo(t, repo, key, "push", "-q", "-u", "origin", "branch-c")
	gitDo(t, repo, key, "checkout", "-q", "main")
	write("m3.txt", "m3")
	comm("m3")
	gitDo(t, repo, key, "push", "-q", "origin", "HEAD")
	alice.mustStatus("POST", prPath("alice", "pr2", ""),
		map[string]string{"title": "rb", "source_branch": "branch-c", "target_branch": "main"}, 201)
	// 真非法 method
	alice.mustFail("POST", prPath("alice", "pr2", "/3/merge"),
		map[string]string{"method": "bogus"}, 400)
	// rebase 合并：成功且 main 含 c 改动
	rb := alice.mustStatus("POST", prPath("alice", "pr2", "/3/merge"),
		map[string]string{"method": "rebase"}, 200)
	if rb["state"] != "merged" {
		t.Fatalf("rebase merge = %v", rb)
	}
	gitDo(t, repo, key, "fetch", "-q", "origin")
	outC := runCmd(t, repo, nil, "git", "show", "origin/main:c.txt")
	if !strings.Contains(outC, "c") {
		t.Fatalf("rebase 后 main 缺少 c.txt: %q", outC)
	}
	// commit diff：取 main 上一个提交，diff 应包含对应文件
	commitsRaw := rawGet(t, alice, "/repos/pr2/commits?ref=main")
	var cs []map[string]any
	if err := json.Unmarshal([]byte(commitsRaw), &cs); err != nil || len(cs) == 0 {
		t.Fatalf("commits decode: %v (%s)", err, commitsRaw)
	}
	sha, _ := cs[0]["sha"].(string)
	cd := alice.mustStatus("GET", fmt.Sprintf("/users/alice/repos/pr2/commits/%s/diff", sha), nil, 200)
	if cd["patch"] == nil {
		t.Fatalf("commit diff = %v", cd)
	}
	// 非法 sha
	alice.mustFail("GET", "/users/alice/repos/pr2/commits/badshadiff/diff", nil, 400)
	alice.mustFail("GET", fmt.Sprintf("/users/alice/repos/pr2/commits/%s/diff", "zz"+sha[2:]), nil, 400)
}
