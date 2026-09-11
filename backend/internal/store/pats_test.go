package store

import (
	"path/filepath"
	"testing"
	"time"
)

func TestNormalizePATCIDRs(t *testing.T) {
	got, ok := NormalizePATCIDRs([]string{" 192.0.2.0/24 ", "192.0.2.0/24", "2001:db8::1", "10.0.0.5"})
	if !ok {
		t.Fatal("should accept valid cidrs")
	}
	want := "192.0.2.0/24,2001:db8::1/128,10.0.0.5/32"
	if got != want {
		t.Fatalf("normalize = %q, want %q", got, want)
	}
	if _, ok := NormalizePATCIDRs([]string{"not-an-ip"}); ok {
		t.Fatal("invalid cidr should fail")
	}
	if _, ok := NormalizePATCIDRs([]string{}); !ok {
		t.Fatal("empty list should be allowed")
	}
}

func TestValidatePATRestrictions(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateUser("alice", "alice-pass-123"); err != nil {
		t.Fatal(err)
	}
	uid, err := s.UserID("alice")
	if err != nil {
		t.Fatal(err)
	}
	noRestrict, _, err := s.CreatePAT(uid, "any", "repo", "", "")
	if err != nil {
		t.Fatal(err)
	}
	cidrTok, _, err := s.CreatePAT(uid, "cidr", "repo", "192.0.2.0/24", "")
	if err != nil {
		t.Fatal(err)
	}
	expiredTok, _, err := s.CreatePAT(uid, "expired", "repo", "",
		time.Now().Add(-time.Hour).UTC().Format(time.RFC3339))
	if err != nil {
		t.Fatal(err)
	}

	if _, _, err := s.ValidatePAT(noRestrict, "10.1.2.3"); err != nil {
		t.Fatalf("unrestricted pat should pass any ip: %v", err)
	}
	if _, _, err := s.ValidatePAT(cidrTok, "192.0.2.9"); err != nil {
		t.Fatalf("cidr pat allowed ip failed: %v", err)
	}
	if _, _, err := s.ValidatePAT(cidrTok, "10.1.2.3"); err != ErrPATIPDenied {
		t.Fatalf("cidr pat denied ip = %v, want ErrPATIPDenied", err)
	}
	// 空 ip（admin 检测路径）应跳过 IP 限制
	if _, _, err := s.ValidatePAT(cidrTok, ""); err != nil {
		t.Fatalf("cidr pat with empty ip should pass: %v", err)
	}
	if _, _, err := s.ValidatePAT(expiredTok, ""); err != ErrPATExpired {
		t.Fatalf("expired pat = %v, want ErrPATExpired", err)
	}
	if _, _, err := s.ValidatePAT("bogus", ""); err != ErrNotFound {
		t.Fatalf("bogus token = %v, want ErrNotFound", err)
	}
}