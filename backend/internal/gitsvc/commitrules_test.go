package gitsvc

import "testing"

func TestCompileCommitIdentityRule(t *testing.T) {
	if _, err := CompileCommitIdentityRule("[", ""); err == nil {
		t.Fatal("invalid name pattern should error")
	}
	if _, err := CompileCommitIdentityRule("", "("); err == nil {
		t.Fatal("invalid email pattern should error")
	}
	if r, _ := CompileCommitIdentityRule("", ""); r.Active() {
		t.Fatal("empty rule should be inactive")
	}

	r, err := CompileCommitIdentityRule("^[A-Za-z]+$", `^[^@]+@example\.com$`)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if !r.Active() {
		t.Fatal("rule should be active")
	}

	ok := commitIdentity{
		sha: "abcdef1234567", authorName: "Alice", authorEmail: "a@example.com",
		committerName: "Bob", committerEmail: "b@example.com",
	}
	if err := r.check(ok); err != nil {
		t.Fatalf("matching identity should pass: %v", err)
	}
	bad := ok
	bad.authorName = "Alice99"
	if err := r.check(bad); err == nil {
		t.Fatal("bad author name should fail")
	}
	badEmail := ok
	badEmail.committerEmail = "b@other.com"
	if err := r.check(badEmail); err == nil {
		t.Fatal("bad committer email should fail")
	}

	// 整体匹配：pattern "alice" 不应匹配 "malice"
	r2, _ := CompileCommitIdentityRule("alice", "")
	if err := r2.check(commitIdentity{sha: "x", authorName: "malice", committerName: "alice"}); err == nil {
		t.Fatal("substring should not satisfy full-match rule")
	}
}
