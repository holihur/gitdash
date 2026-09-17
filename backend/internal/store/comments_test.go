package store

import (
	"path/filepath"
	"testing"
)

func TestCreateCommentMetaAndLookup(t *testing.T) {
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
	it, err := s.CreateIssue("alice", "demo", "alice", "t", "b")
	if err != nil {
		t.Fatal(err)
	}

	c, err := s.CreateCommentMeta("alice", "demo", "issue", it.Number, "alice", "hi", nil,
		"<mid-1@example.com>", "<parent@example.com>")
	if err != nil {
		t.Fatal(err)
	}
	if c.MessageID != "<mid-1@example.com>" || c.InReplyTo != "<parent@example.com>" {
		t.Fatalf("comment meta = %+v", c)
	}

	got, ok := s.CommentByMessageID("<mid-1@example.com>")
	if !ok || got.ID != c.ID {
		t.Fatalf("lookup = %+v ok=%v", got, ok)
	}
	if _, ok := s.CommentByMessageID("<nope@example.com>"); ok {
		t.Fatal("unexpected hit for unknown message id")
	}
	if _, ok := s.CommentByMessageID(""); ok {
		t.Fatal("empty message id should not match")
	}

	// 列表读回同样携带线程字段
	list, err := s.ListComments("alice", "demo", "issue", it.Number, 0, 0)
	if err != nil || len(list) != 1 {
		t.Fatalf("list = %+v err=%v", list, err)
	}
	if list[0].MessageID != "<mid-1@example.com>" || list[0].InReplyTo != "<parent@example.com>" {
		t.Fatalf("listed meta = %+v", list[0])
	}
}

func TestGetByEmail(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateUser("alice", "alice-pass-123"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetUserEmail("alice", "Alice@Example.com"); err != nil {
		t.Fatal(err)
	}
	u, err := s.GetByEmail("alice@example.com")
	if err != nil || u.Username != "alice" {
		t.Fatalf("get by email = %+v err=%v", u, err)
	}
	if _, err := s.GetByEmail("nobody@example.com"); err == nil {
		t.Fatal("expected not found")
	}
}
