package store

import (
	"path/filepath"
	"testing"
)

func newOAuthTestStore(t *testing.T, username string) (*Store, int64) {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateUser(username, username+"-pass-123"); err != nil {
		t.Fatal(err)
	}
	uid, err := s.UserID(username)
	if err != nil {
		t.Fatal(err)
	}
	return s, uid
}

func TestOAuthAppLifecycle(t *testing.T) {
	s, uid := newOAuthTestStore(t, "alice")

	app, err := s.CreateOAuthApp(uid, "My App", "https://example.com", "", "https://example.com/cb")
	if err != nil {
		t.Fatal(err)
	}
	if app.ClientID == "" || app.ClientSecret == "" {
		t.Fatal("expected client_id and client_secret")
	}
	if app.ID == 0 {
		t.Fatal("expected app id")
	}

	apps, err := s.ListOAuthApps(uid)
	if err != nil || len(apps) != 1 {
		t.Fatalf("list = %d, err %v", len(apps), err)
	}
	if apps[0].ClientID != app.ClientID {
		t.Fatal("list must return matching client_id")
	}

	got, err := s.GetOAuthAppByClientID(app.ClientID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != app.ID || !s.VerifyOAuthAppSecret(got, app.ClientSecret) {
		t.Fatal("client lookup / secret verify failed")
	}
	if s.VerifyOAuthAppSecret(got, "wrong") {
		t.Fatal("wrong secret must fail verification")
	}

	newSecret, err := s.ResetOAuthAppSecret(uid, app.ID)
	if err != nil {
		t.Fatal(err)
	}
	got2, _ := s.GetOAuthAppByClientID(app.ClientID)
	if s.VerifyOAuthAppSecret(got2, app.ClientSecret) {
		t.Fatal("old secret must be invalid after reset")
	}
	if !s.VerifyOAuthAppSecret(got2, newSecret) {
		t.Fatal("new secret must verify")
	}
}

func TestOAuthGrantOneTimeAndExpiry(t *testing.T) {
	s, uid := newOAuthTestStore(t, "bob")
	app, err := s.CreateOAuthApp(uid, "A", "", "", "https://cb.example")
	if err != nil {
		t.Fatal(err)
	}

	code, err := s.CreateOAuthGrant(app.ID, uid, "repo,inbox", "https://cb.example")
	if err != nil {
		t.Fatal(err)
	}
	grant, err := s.ConsumeOAuthGrant(code)
	if err != nil {
		t.Fatal(err)
	}
	if grant.AppID != app.ID || grant.UserID != uid || grant.Scopes != "repo,inbox" {
		t.Fatalf("unexpected grant: %+v", grant)
	}
	if _, err := s.ConsumeOAuthGrant(code); err == nil {
		t.Fatal("code must be single-use")
	}

	// 过期 code：直接造一行已过期 grant 验证被拒。
	code2, err := s.CreateOAuthGrant(app.ID, uid, "repo", "https://cb.example")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.db.Model(&oauthGrantRow{}).Where("code_hash = ?", oauthCodeHash(code2)).
		Update("expires_at", "2000-01-01T00:00:00Z").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := s.ConsumeOAuthGrant(code2); err == nil {
		t.Fatal("expired code must be rejected")
	}
}

func TestOAuthTokenIssueListRevoke(t *testing.T) {
	s, uid := newOAuthTestStore(t, "carol")
	app, err := s.CreateOAuthApp(uid, "App", "", "", "https://cb.example")
	if err != nil {
		t.Fatal(err)
	}

	token, pat, err := s.CreateOAuthPAT(uid, app.ID, "OAuth: App", "repo")
	if err != nil {
		t.Fatal(err)
	}
	if token == "" || pat.ID == 0 {
		t.Fatal("expected token")
	}
	// access token 必须能被现有 PAT 校验通过（resolveUser 复用）。
	user, scopes, err := s.ValidatePAT(token, "")
	if err != nil {
		t.Fatal(err)
	}
	if user != "carol" || len(scopes) != 1 || scopes[0] != "repo" {
		t.Fatalf("validate = %q %v", user, scopes)
	}

	auths, err := s.ListOAuthAuthorizations(uid)
	if err != nil || len(auths) != 1 {
		t.Fatalf("authorizations = %d, err %v", len(auths), err)
	}
	if auths[0].AppID != app.ID || auths[0].AppName != "App" {
		t.Fatalf("unexpected auth: %+v", auths[0])
	}

	if err := s.RevokeOAuthAuthorization(uid, auths[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.ValidatePAT(token, ""); err == nil {
		t.Fatal("revoked token must fail validation")
	}
}

func TestOAuthDeleteAppCascades(t *testing.T) {
	s, uid := newOAuthTestStore(t, "dave")
	app, err := s.CreateOAuthApp(uid, "App", "", "", "https://cb.example")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.CreateOAuthPAT(uid, app.ID, "OAuth: App", "repo"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateOAuthGrant(app.ID, uid, "repo", "https://cb.example"); err != nil {
		t.Fatal(err)
	}

	if err := s.DeleteOAuthApp(uid, app.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetOAuthAppByClientID(app.ClientID); err == nil {
		t.Fatal("app must be gone")
	}
	auths, _ := s.ListOAuthAuthorizations(uid)
	if len(auths) != 0 {
		t.Fatal("tokens must be cascaded away")
	}
}
