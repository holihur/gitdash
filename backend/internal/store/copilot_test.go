package store

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestByokKeysCRUD(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateUser("alice", "alice-pass-123"); err != nil {
		t.Fatal(err)
	}

	key, err := s.CreateByokKey("alice", "openai", "openai", "sk-test-1", "https://api.openai.com/v1", "gpt-4o-mini")
	if err != nil {
		t.Fatal(err)
	}
	if key.ID == 0 || !key.KeySet {
		t.Fatalf("unexpected dto: %+v", key)
	}
	// 明文永远不通过 DTO 回传
	if key.BaseURL != "https://api.openai.com/v1" || key.Model != "gpt-4o-mini" {
		t.Fatalf("dto missing fields: %+v", key)
	}

	secret, err := s.GetByokSecret("alice", key.ID)
	if err != nil {
		t.Fatal(err)
	}
	if secret.APIKey != "sk-test-1" || secret.Provider != "openai" {
		t.Fatalf("secret mismatch: %+v", secret)
	}

	// 更新时留空 api_key 应保留原密钥
	key, err = s.UpdateByokKey("alice", key.ID, "openai-prod", "openai", "", "", "gpt-4o")
	if err != nil {
		t.Fatal(err)
	}
	secret, _ = s.GetByokSecret("alice", key.ID)
	if secret.APIKey != "sk-test-1" {
		t.Fatalf("api key should be preserved, got %q", secret.APIKey)
	}
	if key.Name != "openai-prod" || key.Model != "gpt-4o" {
		t.Fatalf("update mismatch: %+v", key)
	}

	list, err := s.ListByokKeys("alice")
	if err != nil || len(list) != 1 {
		t.Fatalf("list = %d, %v", len(list), err)
	}
	if err := s.DeleteByokKey("alice", key.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetByokKey("alice", key.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestByokKeyScopedToUser(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	_, _ = s.CreateUser("alice", "alice-pass-123")
	_, _ = s.CreateUser("bob", "bob-pass-1234")

	key, _ := s.CreateByokKey("alice", "mine", "openai", "sk-a", "", "")
	if _, err := s.GetByokSecret("bob", key.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("bob should not access alice's key, got %v", err)
	}
	if err := s.DeleteByokKey("bob", key.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("bob should not delete alice's key, got %v", err)
	}
}

func TestCopilotSessionsCRUD(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	_, _ = s.CreateUser("alice", "alice-pass-123")
	key, _ := s.CreateByokKey("alice", "k", "openai", "sk-a", "", "")

	cs, err := s.CreateCopilotSession("alice", "repo1", "alice", key.ID, "fix the bug")
	if err != nil {
		t.Fatal(err)
	}
	if cs.Status != "idle" || cs.ByokID != key.ID || cs.CreatedBy != "alice" {
		t.Fatalf("unexpected session: %+v", cs)
	}

	if err := s.SetCopilotSessionGit("alice", "repo1", cs.ID, "copilot/session-1", "deadbeef"); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetCopilotSession("alice", "repo1", cs.ID)
	if err != nil || got.Branch != "copilot/session-1" || got.HeadSHA != "deadbeef" {
		t.Fatalf("get = %+v, %v", got, err)
	}

	if err := s.SetCopilotSessionStatus("alice", "repo1", cs.ID, "running", ""); err != nil {
		t.Fatal(err)
	}
	got, err = s.GetCopilotSession("alice", "repo1", cs.ID)
	if err != nil || got.Status != "running" {
		t.Fatalf("get = %+v, %v", got, err)
	}

	list, err := s.ListCopilotSessions("alice", "repo1")
	if err != nil || len(list) != 1 {
		t.Fatalf("list = %d, %v", len(list), err)
	}

	ids, err := s.CopilotSessionsByByok("alice", key.ID)
	if err != nil || len(ids) != 1 || ids[0] != cs.ID {
		t.Fatalf("sessions by byok = %v, %v", ids, err)
	}

	if err := s.DeleteCopilotSession("alice", "repo1", cs.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetCopilotSession("alice", "repo1", cs.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}
