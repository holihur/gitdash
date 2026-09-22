package store

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestUserCoverCRUD(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateUser("alice", "alice-pass-123"); err != nil {
		t.Fatal(err)
	}

	if s.HasUserCover("alice") {
		t.Fatal("should not have cover yet")
	}
	if _, err := s.GetUserCover("alice"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}

	data := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}
	if err := s.SetUserCover("alice", "image/png", data); err != nil {
		t.Fatal(err)
	}
	if !s.HasUserCover("alice") {
		t.Fatal("should have cover")
	}
	cv, err := s.GetUserCover("alice")
	if err != nil || cv.ContentType != "image/png" || string(cv.Data) != string(data) {
		t.Fatalf("get cover = %+v, %v", cv, err)
	}

	// 覆盖更新
	if err := s.SetUserCover("alice", "image/jpeg", []byte{0xff, 0xd8}); err != nil {
		t.Fatal(err)
	}
	cv, _ = s.GetUserCover("alice")
	if cv.ContentType != "image/jpeg" {
		t.Fatalf("overwrite failed: %+v", cv)
	}

	if err := s.DeleteUserCover("alice"); err != nil {
		t.Fatal(err)
	}
	if s.HasUserCover("alice") {
		t.Fatal("cover should be deleted")
	}
	if err := s.DeleteUserCover("alice"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestOrgCoverCRUD(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateUser("bob", "bob-pass-12345"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateOrg("acme", "Acme", "bob"); err != nil {
		t.Fatal(err)
	}

	if s.HasOrgCover("acme") {
		t.Fatal("should not have cover yet")
	}
	if _, err := s.GetOrgCover("acme"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}

	data := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}
	if err := s.SetOrgCover("acme", "image/png", data); err != nil {
		t.Fatal(err)
	}
	if !s.HasOrgCover("acme") {
		t.Fatal("should have cover")
	}
	cv, err := s.GetOrgCover("acme")
	if err != nil || cv.ContentType != "image/png" || string(cv.Data) != string(data) {
		t.Fatalf("get cover = %+v, %v", cv, err)
	}

	if err := s.DeleteOrgCover("acme"); err != nil {
		t.Fatal(err)
	}
	if s.HasOrgCover("acme") {
		t.Fatal("cover should be deleted")
	}
	if err := s.DeleteOrgCover("acme"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}
