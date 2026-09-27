package store

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestReservedNames(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateUser("alice", "alice-pass-123"); err != nil {
		t.Fatal(err)
	}

	if s.IsReservedName("admin") {
		t.Fatal("admin should not be reserved initially")
	}
	if err := s.AddReservedName("Admin", "root"); err != nil { // 大小写归一化
		t.Fatalf("add: %v", err)
	}
	if err := s.AddReservedName("admin", "root"); !errors.Is(err, ErrExists) {
		t.Fatalf("dup add err = %v, want ErrExists", err)
	}
	if !s.IsReservedName("ADMIN") || !s.IsReservedName(" admin ") {
		t.Fatal("IsReservedName should be case/space insensitive")
	}
	names, err := s.ListReservedNames()
	if err != nil || len(names) != 1 || names[0] != "admin" {
		t.Fatalf("list = %v, %v", names, err)
	}

	if err := s.DeleteReservedName("admin"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := s.DeleteReservedName("admin"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("double delete err = %v", err)
	}
	if s.IsReservedName("admin") {
		t.Fatal("admin should no longer be reserved")
	}
}
