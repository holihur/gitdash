package gitsvc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidName(t *testing.T) {
	cases := []struct {
		name string
		ok   bool
	}{
		{"demo", true},
		{"my-repo", true},
		{"repo.name_01", true},
		{"", false},
		{"-leading", false},
		{".hidden", false},
		{"..", false},
		{"a/b", false},
		{"a b", false},
		{"a\tb", false},
	}
	for _, c := range cases {
		if got := ValidName(c.name); got != c.ok {
			t.Errorf("ValidName(%q) = %v, want %v", c.name, got, c.ok)
		}
	}
}

func TestValidRef(t *testing.T) {
	cases := []struct {
		ref string
		ok  bool
	}{
		{"main", true},
		{"feature/x", true},
		{"v1.0.0", true},
		{"", false},
		{"-evil", false},
		{"a..b", false},
		{"has space", false},
	}
	for _, c := range cases {
		if got := ValidRef(c.ref); got != c.ok {
			t.Errorf("ValidRef(%q) = %v, want %v", c.ref, got, c.ok)
		}
	}
}

func TestCleanPath(t *testing.T) {
	if p, err := CleanPath("src/main.go"); err != nil || p != "src/main.go" {
		t.Errorf("CleanPath simple = %q, %v", p, err)
	}
	if p, err := CleanPath("/src/main.go/"); err != nil || p != "src/main.go" {
		t.Errorf("CleanPath trim = %q, %v", p, err)
	}
	if _, err := CleanPath("a/../b"); err == nil {
		t.Error("CleanPath traversal should fail")
	}
	if _, err := CleanPath("a//b"); err == nil {
		t.Error("CleanPath empty segment should fail")
	}
	if p, err := CleanPath(""); err != nil || p != "" {
		t.Errorf("CleanPath empty = %q, %v", p, err)
	}
}

func TestIsEmptyRepo(t *testing.T) {
	dir := t.TempDir()
	if err := Init(dir); err != nil {
		t.Fatal(err)
	}
	if err := CreateBare("alice", "empty"); err != nil {
		t.Fatal(err)
	}
	if !IsEmptyRepo("alice", "empty") {
		t.Fatal("freshly created bare repo should be empty")
	}
	if err := InitTemplate("alice", "empty"); err != nil {
		t.Fatal(err)
	}
	if IsEmptyRepo("alice", "empty") {
		t.Fatal("repo with an initial commit should not be empty")
	}
}

func TestForkRepoPreservesRefs(t *testing.T) {
	dir := t.TempDir()
	if err := Init(dir); err != nil {
		t.Fatal(err)
	}
	if err := CreateBare("alice", "src"); err != nil {
		t.Fatal(err)
	}
	if err := InitTemplate("alice", "src"); err != nil {
		t.Fatal(err)
	}
	if err := ForkRepo("alice", "src", "bob", "forked"); err != nil {
		t.Fatal(err)
	}
	if !Exists("bob", "forked") {
		t.Fatal("fork repo should exist on disk")
	}
	bs, err := Branches("bob", "forked")
	if err != nil {
		t.Fatal(err)
	}
	if len(bs) != 1 || bs[0].Name != "main" {
		t.Fatalf("fork branches = %+v, want [main]", bs)
	}
	// 源仓库不受影响
	if !Exists("alice", "src") {
		t.Fatal("source repo should still exist")
	}
}

func TestImportRepoFromLocalPath(t *testing.T) {
	dir := t.TempDir()
	if err := Init(dir); err != nil {
		t.Fatal(err)
	}
	if err := CreateBare("alice", "upstream"); err != nil {
		t.Fatal(err)
	}
	if err := InitTemplate("alice", "upstream"); err != nil {
		t.Fatal(err)
	}
	// 从本地 bare 路径导入（模拟远程 URL）
	if err := ImportRepo(RepoPath("alice", "upstream"), "bob", "imported", ""); err != nil {
		t.Fatal(err)
	}
	if !Exists("bob", "imported") {
		t.Fatal("imported repo should exist on disk")
	}
	bs, err := Branches("bob", "imported")
	if err != nil {
		t.Fatal(err)
	}
	if len(bs) != 1 || bs[0].Name != "main" {
		t.Fatalf("imported branches = %+v, want [main]", bs)
	}
}

func TestWriteCommitWithRelativeDataDir(t *testing.T) {
	// 回归：GITDASH_DATA 为相对路径时 reposDir 也是相对路径，
	// WriteCommit 在临时工作区 fetch origin，origin 若为相对路径会在临时目录下解析失败。
	rel := filepath.Join("testdata-relative", t.Name())
	if err := os.MkdirAll(rel, 0o755); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll("testdata-relative") }()
	if err := Init(rel); err != nil {
		t.Fatal(err)
	}
	if err := CreateBare("alice", "src"); err != nil {
		t.Fatal(err)
	}
	if err := InitTemplate("alice", "src"); err != nil {
		t.Fatal(err)
	}
	sha, err := WriteCommit("alice", "src", "main", "add file", "alice",
		[]FileChange{{Path: "a.txt", Action: "create", Content: "hello"}})
	if err != nil {
		t.Fatal(err)
	}
	if sha == "" {
		t.Fatal("empty commit sha")
	}
	bs, err := Branches("alice", "src")
	if err != nil {
		t.Fatal(err)
	}
	if len(bs) != 1 || bs[0].Name != "main" {
		t.Fatalf("branches = %+v, want [main]", bs)
	}
}

func TestLastCommit(t *testing.T) {
	dir := t.TempDir()
	if err := Init(dir); err != nil {
		t.Fatal(err)
	}
	if err := CreateBare("alice", "src"); err != nil {
		t.Fatal(err)
	}
	if err := InitTemplate("alice", "src"); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteCommit("alice", "src", "main", "add root file", "alice",
		[]FileChange{{Path: "a.txt", Action: "create", Content: "hello"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteCommit("alice", "src", "main", "touch nested", "bob",
		[]FileChange{{Path: "docs/b.md", Action: "create", Content: "x"}}); err != nil {
		t.Fatal(err)
	}

	// 根目录整体最后提交
	root, err := LastCommit("alice", "src", "main", "")
	if err != nil || root == nil {
		t.Fatalf("root last commit: %+v, %v", root, err)
	}
	if root.Message != "touch nested" || root.Author != "bob" || root.SHA == "" || root.Date == "" {
		t.Fatalf("root = %+v", root)
	}

	// 目录聚合：docs 的最后提交
	docs, err := LastCommit("alice", "src", "main", "docs")
	if err != nil || docs == nil || docs.Message != "touch nested" {
		t.Fatalf("docs = %+v, %v", docs, err)
	}

	// 文件：a.txt 的最后提交
	a, err := LastCommit("alice", "src", "main", "a.txt")
	if err != nil || a == nil || a.Message != "add root file" {
		t.Fatalf("a.txt = %+v, %v", a, err)
	}
}

func TestSSHEnvAlwaysAcceptsNewHostKeys(t *testing.T) {
	// issue #8: 无私钥的 SSH 远端也必须注入 accept-new，避免卡在 host key 确认。
	env, cleanup, err := sshEnv("git@github.com:owner/repo.git", "")
	if err != nil {
		t.Fatal(err)
	}
	if cleanup != nil {
		t.Fatal("no-key sshEnv should not return cleanup")
	}
	if len(env) != 1 || !strings.HasPrefix(env[0], "GIT_SSH_COMMAND=") {
		t.Fatalf("env = %v", env)
	}
	cmd := strings.TrimPrefix(env[0], "GIT_SSH_COMMAND=")
	if !strings.Contains(cmd, "-o StrictHostKeyChecking=accept-new") {
		t.Fatalf("ssh command %q missing accept-new", cmd)
	}
	if strings.Contains(cmd, "-i") || strings.Contains(cmd, "IdentitiesOnly") {
		t.Fatalf("no-key ssh command %q should not contain -i/IdentitiesOnly", cmd)
	}
	// ssh:// 前缀同样识别为 SSH。
	if env, _, err := sshEnv("ssh://git@github.com/owner/repo.git", ""); err != nil || len(env) != 1 {
		t.Fatalf("ssh:// remote env = %v, err = %v", env, err)
	}

	// 非 SSH 远端（https / git / 本地路径）不注入，避免干扰其它协议。
	for _, u := range []string{"https://github.com/owner/repo.git", "git://host/repo.git", "/tmp/local.git"} {
		if env, cleanup, err := sshEnv(u, ""); err != nil || env != nil || cleanup != nil {
			t.Fatalf("non-ssh %q should not inject env: env=%v hasCleanup=%v err=%v", u, env, cleanup != nil, err)
		}
	}

	// 有私钥时仍需指定专用 key（即使远端不是 SSH，只要给了 key 就用）。
	env, cleanup, err = sshEnv("git@github.com:owner/repo.git", "ssh-ed25519 AAAA fake\n")
	if err != nil {
		t.Fatal(err)
	}
	if cleanup == nil {
		t.Fatal("with-key sshEnv should return cleanup")
	}
	defer cleanup()
	if len(env) != 1 || !strings.HasPrefix(env[0], "GIT_SSH_COMMAND=") {
		t.Fatalf("env = %v", env)
	}
	cmd = strings.TrimPrefix(env[0], "GIT_SSH_COMMAND=")
	if !strings.Contains(cmd, "-o StrictHostKeyChecking=accept-new") || !strings.Contains(cmd, "-i") || !strings.Contains(cmd, "IdentitiesOnly=yes") {
		t.Fatalf("with-key ssh command %q missing options", cmd)
	}
}

func TestPushMirror(t *testing.T) {
	dir := t.TempDir()
	if err := Init(dir); err != nil {
		t.Fatal(err)
	}
	if err := CreateBare("alice", "src"); err != nil {
		t.Fatal(err)
	}
	if err := InitTemplate("alice", "src"); err != nil {
		t.Fatal(err)
	}
	// 目标：一个空 bare 仓库（模拟第三方远程）
	target := filepath.Join(t.TempDir(), "remote.git")
	if _, err := gitOut("", "init", "--bare", "--initial-branch=main", target); err != nil {
		t.Fatal(err)
	}
	if err := PushMirror("alice", "src", target, ""); err != nil {
		t.Fatal(err)
	}
	out, err := gitOut(target, "for-each-ref", "--format=%(refname:short)", "refs/heads")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "main") {
		t.Fatalf("target branches = %q, want main", out)
	}
}

func TestRevertCommit(t *testing.T) {
	dir := t.TempDir()
	if err := Init(dir); err != nil {
		t.Fatal(err)
	}
	if err := CreateBare("alice", "rev"); err != nil {
		t.Fatal(err)
	}
	if err := InitTemplate("alice", "rev"); err != nil {
		t.Fatal(err)
	}
	// v1 -> v2
	if _, err := WriteCommit("alice", "rev", "main", "add", "alice",
		[]FileChange{{Path: "a.txt", Action: "create", Content: "v1"}}); err != nil {
		t.Fatal(err)
	}
	second, err := WriteCommit("alice", "rev", "main", "edit", "alice",
		[]FileChange{{Path: "a.txt", Action: "update", Content: "v2"}})
	if err != nil {
		t.Fatal(err)
	}
	newSHA, err := RevertCommit("alice", "rev", "main", second, "", "alice")
	if err != nil {
		t.Fatal(err)
	}
	if len(newSHA) != 40 {
		t.Fatalf("revert sha = %q", newSHA)
	}
	out, err := gitOut(RepoPath("alice", "rev"), "show", "main:a.txt")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != "v1" {
		t.Fatalf("content after revert = %q, want v1", out)
	}
}

// TestRevertMergeCommit 验证 merge 提交按第一父提交撤销（-m 1）。
func TestRevertMergeCommit(t *testing.T) {
	dir := t.TempDir()
	if err := Init(dir); err != nil {
		t.Fatal(err)
	}
	if err := CreateBare("alice", "m"); err != nil {
		t.Fatal(err)
	}
	if err := InitTemplate("alice", "m"); err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	runGit := func(args ...string) string {
		t.Helper()
		out, err := gitOut(work, args...)
		if err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
		return out
	}
	if _, err := gitOut("", "clone", "-q", RepoPath("alice", "m"), work); err != nil {
		t.Fatal(err)
	}
	runGit("config", "user.name", "alice")
	runGit("config", "user.email", "alice@example.com")
	// feature 分支新增 feature.txt
	runGit("checkout", "-q", "-b", "feature")
	if err := os.WriteFile(filepath.Join(work, "feature.txt"), []byte("feature"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit("add", "-A")
	runGit("commit", "-q", "-m", "feature")
	// main 分支新增 main.txt
	runGit("checkout", "-q", "main")
	if err := os.WriteFile(filepath.Join(work, "main.txt"), []byte("main"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit("add", "-A")
	runGit("commit", "-q", "-m", "main side")
	runGit("merge", "--no-ff", "-q", "-m", "merge feature", "feature")
	mergeSHA := strings.TrimSpace(runGit("rev-parse", "HEAD"))
	runGit("push", "-q", "origin", "HEAD:refs/heads/main")

	if _, err := RevertCommit("alice", "m", "main", mergeSHA, "", "alice"); err != nil {
		t.Fatal(err)
	}
	// 撤销 merge 后 feature.txt 消失，main.txt 仍在
	if _, err := gitOut(RepoPath("alice", "m"), "cat-file", "-e", "main:feature.txt"); err == nil {
		t.Fatal("feature.txt should be gone after reverting merge")
	}
	if _, err := gitOut(RepoPath("alice", "m"), "cat-file", "-e", "main:main.txt"); err != nil {
		t.Fatalf("main.txt should remain: %v", err)
	}
}
