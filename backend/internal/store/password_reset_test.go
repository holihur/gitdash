package store

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

// TestPasswordResetLifecycle 覆盖密码重置令牌的写入 / 一次性消费 / 过期 / 清理。
func TestPasswordResetLifecycle(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateUser("alice", "alice-pass-123"); err != nil {
		t.Fatal(err)
	}

	token := "unit-test-pwreset-token" //gitleaks:allow
	expires := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	if err := s.PutPasswordReset(token, "alice", expires); err != nil {
		t.Fatal(err)
	}
	// 明文 token 不落库（settings key 为 sha256）。
	if got := s.GetSetting(pwResetKeyPrefix + token); got != "" {
		t.Fatalf("plaintext token stored in settings: %q", got)
	}
	username, err := s.TakePasswordReset(token)
	if err != nil || username != "alice" {
		t.Fatalf("TakePasswordReset = %q, %v", username, err)
	}
	// 一次性：再次消费失败。
	if _, err := s.TakePasswordReset(token); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second TakePasswordReset err = %v, want ErrNotFound", err)
	}

	// 过期令牌不可用。
	expired := "unit-test-expired-token" //gitleaks:allow
	past := time.Now().Add(-time.Minute).UTC().Format(time.RFC3339)
	if err := s.PutPasswordReset(expired, "alice", past); err != nil {
		t.Fatal(err)
	}
	if _, err := s.TakePasswordReset(expired); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expired TakePasswordReset err = %v, want ErrNotFound", err)
	}

	// ClearPasswordResets 只清理目标用户的令牌。
	if err := s.PutPasswordReset("token-a", "alice", expires); err != nil {
		t.Fatal(err)
	}
	if err := s.PutPasswordReset("token-b", "bob", expires); err != nil {
		t.Fatal(err)
	}
	if err := s.ClearPasswordResets("alice"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.TakePasswordReset("token-a"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("alice token survived clear: %v", err)
	}
	if _, err := s.TakePasswordReset("token-b"); err != nil {
		t.Fatalf("bob token should survive alice clear: %v", err)
	}
}
