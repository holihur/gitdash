package store

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestSessionTokenHashedAtRest 覆盖安全评审 §3.3：会话 token 以 sha256 落库，
// 且旧版明文行仍可登录并会被就地升级。
func TestSessionTokenHashedAtRest(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateUser("alice", "alice-pass-123"); err != nil {
		t.Fatal(err)
	}
	uid, _ := s.UserID("alice")

	token := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	if err := s.CreateSession(token, uid); err != nil {
		t.Fatal(err)
	}
	var row sessionRow
	if err := s.db.First(&row, "user_id = ?", uid).Error; err != nil {
		t.Fatal(err)
	}
	if row.Token == token || row.Token != patHash(token) {
		t.Fatalf("stored session token = %q, want sha256 hash", row.Token)
	}
	if name, err := s.GetSession(token); err != nil || name != "alice" {
		t.Fatalf("GetSession = %q, %v", name, err)
	}

	// 旧版明文行：查找成功并升级为哈希。
	legacy := "legacy-plaintext-session-token"
	if err := s.db.Create(&sessionRow{Token: legacy, UserID: uid, CreatedAt: now(), ExpiresAt: time.Now().Add(time.Hour).UTC().Format(time.RFC3339)}).Error; err != nil {
		t.Fatal(err)
	}
	if name, err := s.GetSession(legacy); err != nil || name != "alice" {
		t.Fatalf("legacy GetSession = %q, %v", name, err)
	}
	var upgraded sessionRow
	if err := s.db.First(&upgraded, "user_id = ? AND token = ?", uid, patHash(legacy)).Error; err != nil {
		t.Fatalf("legacy row not upgraded: %v", err)
	}
}

// TestOAuthTokenDefaultExpiry 覆盖安全评审 §3.3：OAuth/device token 默认有期限。
func TestOAuthTokenDefaultExpiry(t *testing.T) {
	s, uid := newOAuthTestStore(t, "carol")
	app, err := s.CreateOAuthApp(uid, "App", "", "", "https://cb.example")
	if err != nil {
		t.Fatal(err)
	}

	_, pat, err := s.CreateOAuthPAT(uid, app.ID, "OAuth: App", "repo")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(pat.ExpiresAt) == "" {
		t.Fatal("oauth token should expire by default")
	}

	t.Setenv("GITDASH_OAUTH_TOKEN_TTL", "0")
	_, pat2, err := s.CreateOAuthPAT(uid, app.ID, "OAuth: App 2", "repo")
	if err != nil {
		t.Fatal(err)
	}
	if pat2.ExpiresAt != "" {
		t.Fatalf("GITDASH_OAUTH_TOKEN_TTL=0 should disable expiry, got %q", pat2.ExpiresAt)
	}
}
