package gitsvc

import (
	"context"
	"path/filepath"
	"testing"
)

// TestSetBackendRejectsNil covers the defensive nil check.
func TestSetBackendRejectsNil(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("SetBackend(nil) should panic")
		}
	}()
	SetBackend(nil)
}

// TestCLIBackendOperations is a wiring smoke test: it invokes every Backend
// operation through the package-level API against a real (temporary) bare
// repository, so the abstraction stays fully connected and alternative
// backends have a reference suite to run. Errors from intentionally adversarial
// inputs are ignored; the point is that every path can be reached.
func TestCLIBackendOperations(t *testing.T) {
	prev := CurrentBackend()
	SetBackend(NewCLI())
	t.Cleanup(func() { SetBackend(prev) })

	dir := t.TempDir()
	if err := Init(dir); err != nil {
		t.Fatal(err)
	}
	_ = ReposDir()
	_ = SpoolDir()

	// lifecycle / storage
	if err := CreateBare("alice", "demo"); err != nil {
		t.Fatal(err)
	}
	if err := InitTemplate("alice", "demo"); err != nil {
		t.Fatal(err)
	}
	_ = EnsureHooks()
	path := RepoPath("alice", "demo")
	_ = Exists("alice", "demo")
	_ = IsEmptyRepo("alice", "demo")
	if err := CreateBare("alice", "empty"); err != nil {
		t.Fatal(err)
	}
	_ = ForkRepo("alice", "demo", "bob", "forked")
	_ = ImportRepo(path, "bob", "imported", "", "")
	target := filepath.Join(t.TempDir(), "mirror.git")
	_, _ = GitOut("", "init", "--bare", "--initial-branch=main", target)
	_ = PushMirror("alice", "demo", target, "")
	_, _ = RepoSize("alice", "demo")
	_, _ = GC("alice", "demo")
	if err := CreateBare("alice", "deleteme"); err == nil {
		_ = Delete("alice", "deleteme")
	}

	// refs
	head, err := HeadBranch("alice", "demo")
	if err != nil || head == "" {
		t.Fatalf("HeadBranch = %q, %v", head, err)
	}
	_, _ = Branches("alice", "demo")
	_, _ = Tags("alice", "demo")
	sha, _ := RevSHA("alice", "demo", head)
	_, _ = CreateRef("alice", "demo", "branch", "feature", head)
	_, _ = CreateRef("alice", "demo", "tag", "v1.0.0", head)
	_, _ = WriteCommit("alice", "demo", "feature", "feature work", "alice",
		[]FileChange{{Path: "feature.txt", Action: "create", Content: "feature\n"}})
	_ = DeleteRef("alice", "demo", "tag", "v1.0.0")
	_ = SetHeadBranch("alice", "demo", "feature")
	_ = SetHeadBranch("alice", "demo", "main")
	InvalidateRefs("alice", "demo")

	// reading
	_, _ = Tree("alice", "demo", "main", "")
	_, _ = ListDir("alice", "demo", "main", "")
	_, _ = ReadBlob("alice", "demo", "main", "README.md")
	_, _ = BlameFile("alice", "demo", "main", "README.md")
	_, _ = RawCommit("alice", "demo", sha)
	_ = RawCommits("alice", "demo", []string{sha})
	_, _ = Commits("alice", "demo", "main", 10, 0, "")
	_, _ = LastCommit("alice", "demo", "main", "")
	_, _, _ = CommitDiff("alice", "demo", sha)
	_, _ = DiffStats("alice", "demo", head, "feature")
	_, _ = DiffPatch("alice", "demo", head, "feature")
	_, _ = Search("alice", "demo", "gitdash", "main", 10)
	_, _ = SearchWith(context.Background(), "alice", "demo", "gitdash", SearchOpts{Ref: "main"})

	// writing / history rewrite
	_ = CanFastForward("alice", "demo", "main", "feature")
	_, _ = MergeCheck("alice", "demo", "main", "feature")
	_, _ = MergeFastForward("alice", "demo", "main", "feature")
	_, _ = MergeNonFF("alice", "demo", "main", "feature", "merge feature", "alice", "merge")
	_, _ = MergeRebase("alice", "demo", "main", "feature", "alice")
	_, _ = RevertCommit("alice", "demo", "main", sha, "revert", "alice")
	_, _, _ = ApplyPatchSeries("alice", "demo", "main", []byte("not a patch"))
}

// recordingBackend embeds the CLI backend so it satisfies every Backend method
// without boilerplate, and records/overrides the operations under test.
type recordingBackend struct {
	Backend
	headBranchCalls int
}

func (r *recordingBackend) HeadBranch(owner, name string) (string, error) {
	r.headBranchCalls++
	return "from-fake", nil
}

// TestBackendSwap verifies that the package-level helpers dispatch through the
// installed backend, which is the whole point of the abstraction: an
// alternative implementation only needs to be registered and set.
func TestBackendSwap(t *testing.T) {
	prev := CurrentBackend()
	t.Cleanup(func() { SetBackend(prev) })

	fake := &recordingBackend{Backend: NewCLI()}
	SetBackend(fake)

	if got := CurrentBackend(); got != Backend(fake) {
		t.Fatalf("CurrentBackend = %T, want the installed fake", got)
	}
	branch, err := HeadBranch("alice", "demo")
	if err != nil {
		t.Fatal(err)
	}
	if branch != "from-fake" {
		t.Fatalf("HeadBranch = %q, want %q", branch, "from-fake")
	}
	if fake.headBranchCalls != 1 {
		t.Fatalf("fake backend received %d calls, want 1", fake.headBranchCalls)
	}
}

// TestBackendRegistry verifies named backends can be registered and built.
func TestBackendRegistry(t *testing.T) {
	RegisterBackend("fake-test", func() Backend { return &cliBackend{} })
	b, err := NewBackend("fake-test")
	if err != nil {
		t.Fatalf("NewBackend registered = %v", err)
	}
	if _, ok := b.(*cliBackend); !ok {
		t.Fatalf("NewBackend = %T, want *cliBackend", b)
	}
	if _, err := NewBackend("does-not-exist"); err == nil {
		t.Fatal("NewBackend(unknown) should fail")
	}
}

// TestDefaultBackendIsCLI is a guard so the default stays usable without any
// explicit registration.
func TestDefaultBackendIsCLI(t *testing.T) {
	if CurrentBackend() == nil {
		t.Fatal("default backend must not be nil")
	}
	b, err := NewBackend("cli")
	if err != nil {
		t.Fatalf("cli backend must be pre-registered: %v", err)
	}
	if b == nil {
		t.Fatal("cli backend factory returned nil")
	}
}
