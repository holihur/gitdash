package store

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestUserAvatarCRUD(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateUser("alice", "alice-pass-123"); err != nil {
		t.Fatal(err)
	}

	if s.HasUserAvatar("alice") {
		t.Fatal("should not have avatar yet")
	}
	if _, err := s.GetUserAvatar("alice"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}

	data := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}
	if err := s.SetUserAvatar("alice", "image/png", data); err != nil {
		t.Fatal(err)
	}
	if !s.HasUserAvatar("alice") {
		t.Fatal("should have avatar")
	}
	av, err := s.GetUserAvatar("alice")
	if err != nil || av.ContentType != "image/png" || string(av.Data) != string(data) {
		t.Fatalf("get avatar = %+v, %v", av, err)
	}

	// 覆盖更新
	if err := s.SetUserAvatar("alice", "image/jpeg", []byte{0xff, 0xd8}); err != nil {
		t.Fatal(err)
	}
	av, _ = s.GetUserAvatar("alice")
	if av.ContentType != "image/jpeg" {
		t.Fatalf("overwrite failed: %+v", av)
	}

	if err := s.DeleteUserAvatar("alice"); err != nil {
		t.Fatal(err)
	}
	if s.HasUserAvatar("alice") {
		t.Fatal("avatar should be deleted")
	}
	if err := s.DeleteUserAvatar("alice"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}
