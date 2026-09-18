package store

import (
	"path/filepath"
	"testing"
)

// TestIssueNumbersAreMonotonic 编号按仓库单调递增：删空 issue 后新建不复用 #1。
func TestIssueNumbersAreMonotonic(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateUser("alice", "alice-pass-123"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateRepo("alice", "demo", "d", false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateRepo("alice", "other", "d", false); err != nil {
		t.Fatal(err)
	}

	var last int64
	for i := int64(1); i <= 3; i++ {
		it, err := s.CreateIssue("alice", "demo", "alice", "t", "b")
		if err != nil {
			t.Fatal(err)
		}
		if it.Number != i {
			t.Fatalf("issue %d: got number %d", i, it.Number)
		}
		last = it.Number
	}

	// 删光该仓库所有 issue
	for n := int64(1); n <= last; n++ {
		if err := s.DeleteIssue("alice", "demo", n); err != nil {
			t.Fatal(err)
		}
	}

	// 新建编号必须继续递增，而不是回到 1
	it, err := s.CreateIssue("alice", "demo", "alice", "after delete", "b")
	if err != nil {
		t.Fatal(err)
	}
	if it.Number != last+1 {
		t.Fatalf("number reset after delete: got %d, want %d", it.Number, last+1)
	}

	// 编号按仓库独立：另一个仓库从 1 开始
	other, err := s.CreateIssue("alice", "other", "alice", "first", "b")
	if err != nil {
		t.Fatal(err)
	}
	if other.Number != 1 {
		t.Fatalf("other repo first issue = %d, want 1", other.Number)
	}
}

// TestIssueNumberSeedsFromExistingData 计数器首次分配以存量最大编号为基准（兼容旧库）。
func TestIssueNumberSeedsFromExistingData(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateUser("alice", "alice-pass-123"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateRepo("alice", "demo", "d", false); err != nil {
		t.Fatal(err)
	}
	// 模拟存量库：已有编号 5 的 issue，但没有 repo_counters 行。
	if err := s.db.Create(&issueRow{
		Owner: "alice", Repo: "demo", Number: 5, Title: "x", State: "open", Author: "alice",
		CreatedAt: now(), UpdatedAt: now(),
	}).Error; err != nil {
		t.Fatal(err)
	}
	it, err := s.CreateIssue("alice", "demo", "alice", "t", "b")
	if err != nil {
		t.Fatal(err)
	}
	if it.Number != 6 {
		t.Fatalf("seed from existing: got %d, want 6", it.Number)
	}
}

// TestRepoCountersDeletedWithRepo 删除仓库时计数器一并清理：同名重建后从 1 重新开始。
func TestRepoCountersDeletedWithRepo(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateUser("alice", "alice-pass-123"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateRepo("alice", "demo", "d", false); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if _, err := s.CreateIssue("alice", "demo", "alice", "t", "b"); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.DeleteRepo("alice", "demo"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateRepo("alice", "demo", "d", false); err != nil {
		t.Fatal(err)
	}
	it, err := s.CreateIssue("alice", "demo", "alice", "fresh", "b")
	if err != nil {
		t.Fatal(err)
	}
	if it.Number != 1 {
		t.Fatalf("recreated repo first issue = %d, want 1", it.Number)
	}
}

// TestPRNumbersAreMonotonic PR 编号同样按仓库单调递增、删除后不复用。
func TestPRNumbersAreMonotonic(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateUser("alice", "alice-pass-123"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateRepo("alice", "demo", "d", false); err != nil {
		t.Fatal(err)
	}
	for i := int64(1); i <= 2; i++ {
		pr, err := s.CreatePull("alice", "demo", "alice", "t", "b", "feat", "main", "", "", false)
		if err != nil {
			t.Fatal(err)
		}
		if pr.Number != i {
			t.Fatalf("pr %d: got number %d", i, pr.Number)
		}
	}
	// 直接清空 PR 行（模拟级联删除），计数不应回退。
	if err := s.db.Where("owner = ? AND repo = ?", "alice", "demo").Delete(&pullRequestRow{}).Error; err != nil {
		t.Fatal(err)
	}
	pr, err := s.CreatePull("alice", "demo", "alice", "t3", "b", "feat3", "main", "", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if pr.Number != 3 {
		t.Fatalf("pr number reset after delete: got %d, want 3", pr.Number)
	}
}
