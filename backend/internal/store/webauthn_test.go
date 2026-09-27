package store

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestWebAuthnCredentialStore(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateUser("alice", "alice-pass-123"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateUser("bob", "bob-pass-1234"); err != nil {
		t.Fatal(err)
	}

	if recs, err := s.ListWebAuthnCredentials("alice"); err != nil || len(recs) != 0 {
		t.Fatalf("initial list = %v, %v", recs, err)
	}

	id, err := s.AddWebAuthnCredential("alice", "Laptop", "cred-a", []byte(`{"id":"a"}`))
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if _, err := s.AddWebAuthnCredential("bob", "Phone", "cred-b", []byte(`{"id":"b"}`)); err != nil {
		t.Fatalf("add bob: %v", err)
	}
	// 凭据 ID 全局唯一
	if _, err := s.AddWebAuthnCredential("alice", "Dup", "cred-a", []byte(`{}`)); !errors.Is(err, ErrExists) {
		t.Fatalf("dup add err = %v, want ErrExists", err)
	}
	// 未知用户
	if _, err := s.AddWebAuthnCredential("ghost", "x", "cred-x", []byte(`{}`)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown user err = %v, want ErrNotFound", err)
	}

	recs, err := s.ListWebAuthnCredentials("alice")
	if err != nil || len(recs) != 1 || recs[0].Name != "Laptop" {
		t.Fatalf("list = %v, %v", recs, err)
	}
	if n, err := s.CountWebAuthnCredentials("alice"); err != nil || n != 1 {
		t.Fatalf("count = %d, %v", n, err)
	}

	rec, err := s.WebAuthnCredentialByCredentialID("cred-a")
	if err != nil || rec.Username != "alice" || rec.UserID != recs[0].UserID {
		t.Fatalf("by credential id = %v, %v", rec, err)
	}
	if _, err := s.WebAuthnCredentialByCredentialID("missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing credential err = %v", err)
	}

	if err := s.UpdateWebAuthnCredential("cred-a", []byte(`{"id":"a","signCount":5}`)); err != nil {
		t.Fatalf("update: %v", err)
	}
	rec, _ = s.WebAuthnCredentialByCredentialID("cred-a")
	if string(rec.Data) != `{"id":"a","signCount":5}` || rec.LastUsedAt == "" {
		t.Fatalf("after update = %+v", rec)
	}

	// 不能跨用户删除
	if err := s.DeleteWebAuthnCredential("bob", id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-user delete err = %v", err)
	}
	if err := s.DeleteWebAuthnCredential("alice", id); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if n, _ := s.CountWebAuthnCredentials("alice"); n != 0 {
		t.Fatalf("count after delete = %d", n)
	}
}

func TestWebAuthnSessionStore(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	now := "2030-01-01T00:00:00Z"
	if err := s.PutWebAuthnSession("tok1", []byte(`{"challenge":"abc"}`), now); err != nil {
		t.Fatal(err)
	}
	data, err := s.TakeWebAuthnSession("tok1", "2029-01-01T00:00:00Z")
	if err != nil || string(data) != `{"challenge":"abc"}` {
		t.Fatalf("take = %s, %v", data, err)
	}
	// 一次性
	if _, err := s.TakeWebAuthnSession("tok1", "2029-01-01T00:00:00Z"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second take err = %v", err)
	}

	// 过期后不可用
	if err := s.PutWebAuthnSession("tok2", []byte(`{}`), now); err != nil {
		t.Fatal(err)
	}
	if _, err := s.TakeWebAuthnSession("tok2", "2031-01-01T00:00:00Z"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expired take err = %v", err)
	}

	// prune 清理过期会话
	if err := s.PutWebAuthnSession("tok3", []byte(`{}`), now); err != nil {
		t.Fatal(err)
	}
	if err := s.PutWebAuthnSession("tok4", []byte(`{}`), "2035-01-01T00:00:00Z"); err != nil {
		t.Fatal(err)
	}
	if n, err := s.PruneWebAuthnSessions("2032-01-01T00:00:00Z"); err != nil || n != 1 {
		t.Fatalf("prune = %d, %v", n, err)
	}
	if _, err := s.TakeWebAuthnSession("tok4", "2032-01-01T00:00:00Z"); err != nil {
		t.Fatalf("future session should survive prune: %v", err)
	}
}

func TestDeleteUserAccountRemovesWebAuthn(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateUser("alice", "alice-pass-123"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddWebAuthnCredential("alice", "Laptop", "cred-a", []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteUserAccount("alice"); err != nil {
		t.Fatalf("delete account: %v", err)
	}
	var n int64
	if err := s.db.Model(&webauthnCredentialRow{}).Count(&n).Error; err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("webauthn credentials after account delete = %d", n)
	}
}
