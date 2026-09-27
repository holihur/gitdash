package store

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gitdash/backend/internal/totp"
)

// TestSensitiveSettingsEncryptedAtRest 覆盖安全审计 M2.7：OAuth client secret /
// SMTP 密码 / feedback token 静态加密落库，读取时透明解密。
func TestSensitiveSettingsEncryptedAtRest(t *testing.T) {
	t.Setenv("GITDASH_SECRET_KEY", strings.Repeat("0", 64))
	resetSecretKey()
	defer resetSecretKey()
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"smtp_pass", "feedback_token", "github_client_secret"} {
		if err := s.SetSetting(key, "hunter2-"+key); err != nil {
			t.Fatal(err)
		}
		var row settingRow
		if err := s.db.Where("\"key\" = ?", key).First(&row).Error; err != nil {
			t.Fatal(err)
		}
		if row.Value == "hunter2-"+key || !strings.HasPrefix(row.Value, "v1:") {
			t.Fatalf("%s not encrypted at rest: %q", key, row.Value)
		}
		if got := s.GetSetting(key); got != "hunter2-"+key {
			t.Fatalf("%s roundtrip = %q", key, got)
		}
	}
	// 非敏感键保持明文。
	if err := s.SetSetting("site_title", "hello"); err != nil {
		t.Fatal(err)
	}
	var plain settingRow
	if err := s.db.Where("\"key\" = ?", "site_title").First(&plain).Error; err != nil {
		t.Fatal(err)
	}
	if plain.Value != "hello" {
		t.Fatalf("non-sensitive setting altered: %q", plain.Value)
	}
}

// TestAcceptTOTPRejectsReplay 覆盖安全审计 M2.4：同一 TOTP 码不可重放。
func TestAcceptTOTPRejectsReplay(t *testing.T) {
	s := openBanStore(t)
	if _, err := s.CreateUser("alice", "hash"); err != nil {
		t.Fatal(err)
	}
	secret, err := totp.GenerateSecret()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetMFASecret("alice", secret, true); err != nil {
		t.Fatal(err)
	}
	code, err := totp.Code(secret, time.Now().Add(-2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if !s.AcceptTOTP("alice", secret, code, 1) {
		t.Fatal("first use of a valid code was rejected")
	}
	if s.AcceptTOTP("alice", secret, code, 1) {
		t.Fatal("replayed code was accepted")
	}
	next, err := totp.Code(secret, time.Now().Add(28*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if !s.AcceptTOTP("alice", secret, next, 1) {
		t.Fatal("next time step was rejected")
	}
}
