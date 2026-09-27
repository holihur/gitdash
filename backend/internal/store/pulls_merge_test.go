package store

import (
	"errors"
	"path/filepath"
	"testing"
)

// TestMarkPullMergedStateGuard 覆盖审计 F-03：MarkPullMerged 带 state='open'
// 守卫，重复合并只能落库一次。
func TestMarkPullMergedStateGuard(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateUser("alice", "hash"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateRepo("alice", "r", "", false); err != nil {
		t.Fatal(err)
	}
	pr, err := s.CreatePull("alice", "r", "alice", "t", "b", "feat", "main", "basesha", "headsha", false)
	if err != nil {
		t.Fatal(err)
	}
	merged, err := s.MarkPullMerged("alice", "r", pr.Number, "mergesha", "alice")
	if err != nil || merged.State != "merged" {
		t.Fatalf("first merge = %+v err=%v", merged, err)
	}
	if _, err := s.MarkPullMerged("alice", "r", pr.Number, "other", "bob"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second merge err = %v, want ErrNotFound", err)
	}
	// 合并结果落库，且 state 仍为 merged。
	got, err := s.GetPull("alice", "r", pr.Number)
	if err != nil || got.State != "merged" || got.MergedBy != "alice" {
		t.Fatalf("after merge = %+v err=%v", got, err)
	}
}
