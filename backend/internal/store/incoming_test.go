package store

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestIncomingWebhooksMultiple(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateUser("alice", "alice-pass-123"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateRepo("alice", "demo", "", false); err != nil {
		t.Fatal(err)
	}

	tok1, h1, err := s.CreateIncomingWebhook("alice", "demo", "ci")
	if err != nil {
		t.Fatalf("create 1: %v", err)
	}
	tok2, _, err := s.CreateIncomingWebhook("alice", "demo", "alerts")
	if err != nil {
		t.Fatalf("create 2: %v", err)
	}
	if tok1 == tok2 || !h1.Enabled {
		t.Fatalf("tokens/state = %q %q %+v", tok1, tok2, h1)
	}

	hooks, err := s.ListIncomingWebhooks("alice", "demo")
	if err != nil || len(hooks) != 2 {
		t.Fatalf("list = %+v, %v", hooks, err)
	}

	if ok, _ := s.ResolveIncomingWebhook("alice", "demo", tok1); !ok {
		t.Fatal("tok1 should resolve")
	}
	// 禁用后失效，启用后恢复
	if err := s.SetIncomingWebhookEnabled("alice", "demo", h1.ID, false); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if ok, _ := s.ResolveIncomingWebhook("alice", "demo", tok1); ok {
		t.Fatal("disabled token should not resolve")
	}
	if ok, _ := s.ResolveIncomingWebhook("alice", "demo", tok2); !ok {
		t.Fatal("other token should still resolve")
	}
	if err := s.SetIncomingWebhookEnabled("alice", "demo", h1.ID, true); err != nil {
		t.Fatalf("enable: %v", err)
	}
	if ok, _ := s.ResolveIncomingWebhook("alice", "demo", tok1); !ok {
		t.Fatal("re-enabled token should resolve")
	}
	// 未知 id
	if err := s.SetIncomingWebhookEnabled("alice", "demo", 99999, false); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown id err = %v", err)
	}

	// 删除其一不影响另一个
	if err := s.DeleteIncomingWebhook("alice", "demo", h1.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if ok, _ := s.ResolveIncomingWebhook("alice", "demo", tok1); ok {
		t.Fatal("deleted token should not resolve")
	}
	if ok, _ := s.ResolveIncomingWebhook("alice", "demo", tok2); !ok {
		t.Fatal("remaining token should still resolve")
	}
	if err := s.DeleteIncomingWebhook("alice", "demo", h1.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("double delete err = %v", err)
	}
	// 跨仓库删除
	if err := s.DeleteIncomingWebhook("alice", "other", h1.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-repo delete err = %v", err)
	}
}
