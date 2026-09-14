package gitsvc

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateBareInstallsHooks(t *testing.T) {
	dir := t.TempDir()
	if err := Init(dir); err != nil {
		t.Fatal(err)
	}
	if err := CreateBare("alice", "demo"); err != nil {
		t.Fatal(err)
	}
	hooksDir := filepath.Join(RepoPath("alice", "demo"), "hooks")

	for _, name := range []string{"post-receive", "pre-receive"} {
		fi, err := os.Stat(filepath.Join(hooksDir, name))
		if err != nil {
			t.Fatalf("%s not installed: %v", name, err)
		}
		if fi.Mode().Perm() != 0o755 {
			t.Fatalf("%s mode = %o, want 755", name, fi.Mode().Perm())
		}
	}

	post, _ := os.ReadFile(filepath.Join(hooksDir, "post-receive"))
	ps := string(post)
	if !strings.HasPrefix(ps, "#!/bin/sh") {
		t.Fatalf("post-receive missing shebang:\n%s", ps)
	}
	for _, want := range []string{`"event":"push"`, `"owner":"alice"`, `"repo":"demo"`, SpoolDir()} {
		if !strings.Contains(ps, want) {
			t.Fatalf("post-receive missing %q:\n%s", want, ps)
		}
	}

	pre, _ := os.ReadFile(filepath.Join(hooksDir, "pre-receive"))
	pr := string(pre)
	if !strings.HasPrefix(pr, "#!/bin/sh") {
		t.Fatalf("pre-receive missing shebang:\n%s", pr)
	}
	if !strings.Contains(pr, `pre-receive "alice" "demo"`) {
		t.Fatalf("pre-receive missing owner/repo args:\n%s", pr)
	}
	if !strings.Contains(pr, "while read oldrev newrev refname") {
		t.Fatalf("pre-receive missing ref loop:\n%s", pr)
	}

	// pre-receive 脚本可执行：TestMain 拦截 pre-receive 子命令并放行（无保护规则）
	cmd := exec.Command(filepath.Join(hooksDir, "pre-receive"))
	cmd.Stdin = strings.NewReader("0000000000000000000000000000000000000000 1111111111111111111111111111111111111111 refs/heads/main\n")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("pre-receive hook run: %v\n%s", err, out)
	}
}

func TestEnsureHooksReinstalls(t *testing.T) {
	dir := t.TempDir()
	if err := Init(dir); err != nil {
		t.Fatal(err)
	}
	if err := CreateBare("alice", "demo"); err != nil {
		t.Fatal(err)
	}
	hook := filepath.Join(RepoPath("alice", "demo"), "hooks", "post-receive")
	if err := os.Remove(hook); err != nil {
		t.Fatal(err)
	}
	if err := EnsureHooks(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(hook); err != nil {
		t.Fatalf("EnsureHooks did not restore post-receive: %v", err)
	}
}
