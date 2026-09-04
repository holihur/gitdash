package gitsvc

import (
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

func TestForkRepoPreservesRefs(t *testing.T) {
	dir := t.TempDir()
	if err := Init(dir); err != nil {
		t.Fatal(err)
	}
	if err := CreateBare("alice", "src"); err != nil {
		t.Fatal(err)
	}
	if err := InitReadme("alice", "src"); err != nil {
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
	if err := InitReadme("alice", "upstream"); err != nil {
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

func TestPushMirror(t *testing.T) {
	dir := t.TempDir()
	if err := Init(dir); err != nil {
		t.Fatal(err)
	}
	if err := CreateBare("alice", "src"); err != nil {
		t.Fatal(err)
	}
	if err := InitReadme("alice", "src"); err != nil {
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
