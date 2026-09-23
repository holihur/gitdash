package store

import (
	"path/filepath"
	"testing"
)

func TestAllReposAfter(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "codeindex.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateUser("alice", "alice-pass-123"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a", "b", "c"} {
		if _, err := s.CreateRepo("alice", name, "", false); err != nil {
			t.Fatal(err)
		}
	}

	first, err := s.AllReposAfter(0, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 2 {
		t.Fatalf("first page = %d rows, want 2", len(first))
	}
	if first[0].DefaultBranch == "" {
		t.Fatal("DefaultBranch should default to main")
	}
	second, err := s.AllReposAfter(first[len(first)-1].ID, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != 1 {
		t.Fatalf("second page = %d rows, want 1", len(second))
	}
	if second[0].ID <= first[len(first)-1].ID {
		t.Fatal("cursor should advance")
	}
}
