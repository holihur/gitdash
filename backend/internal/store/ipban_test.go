package store

import (
	"errors"
	"path/filepath"
	"testing"
)

func openIPBanStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "ipban.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	return s
}

func TestNormalizeIPCIDR(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"1.2.3.4", "1.2.3.4/32", true},
		{"1.2.3.4/24", "1.2.3.0/24", true},
		{"10.0.0.1/8", "10.0.0.0/8", true},
		{"  2001:db8::1  ", "2001:db8::1/128", true},
		{"2001:db8::/32", "2001:db8::/32", true},
		{"::ffff:1.2.3.4", "1.2.3.4/32", true},
		{"", "", false},
		{"not-an-ip", "", false},
		{"1.2.3.4/33", "", false},
	}
	for _, c := range cases {
		got, ok := NormalizeIPCIDR(c.in)
		if ok != c.ok || got != c.want {
			t.Errorf("NormalizeIPCIDR(%q) = %q,%v; want %q,%v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestIPBanCRUDAndMatch(t *testing.T) {
	s := openIPBanStore(t)

	if s.IsIPBanned("203.0.113.5") {
		t.Fatal("nothing should be banned yet")
	}

	ban, err := s.AddIPBan("203.0.113.0/24", "abuse", "admin")
	if err != nil {
		t.Fatal(err)
	}
	if ban.CIDR != "203.0.113.0/24" || ban.CreatedBy != "admin" || ban.CreatedAt == "" {
		t.Fatalf("unexpected ban: %+v", ban)
	}
	if !s.IsIPBanned("203.0.113.5") || !s.IsIPBanned("203.0.113.255") {
		t.Fatal("addresses in CIDR should be banned")
	}
	if s.IsIPBanned("203.0.114.1") {
		t.Fatal("address outside CIDR should not be banned")
	}

	// 单 IP 规范化为 /32
	single, err := s.AddIPBan("198.51.100.7", "", "admin")
	if err != nil {
		t.Fatal(err)
	}
	if single.CIDR != "198.51.100.7/32" {
		t.Fatalf("single ip = %q, want /32", single.CIDR)
	}
	if !s.IsIPBanned("198.51.100.7") || s.IsIPBanned("198.51.100.8") {
		t.Fatal("single ip match wrong")
	}

	// 重复条目
	if _, err := s.AddIPBan("203.0.113.42/24", "", "admin"); !errors.Is(err, ErrExists) {
		t.Fatalf("duplicate = %v, want ErrExists", err)
	}
	// 非法输入
	if _, err := s.AddIPBan("bogus", "", "admin"); !errors.Is(err, ErrInvalidCIDR) {
		t.Fatalf("invalid = %v, want ErrInvalidCIDR", err)
	}

	list, err := s.ListIPBans()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("list = %d, want 2", len(list))
	}

	if err := s.DeleteIPBan(ban.ID); err != nil {
		t.Fatal(err)
	}
	if s.IsIPBanned("203.0.113.5") {
		t.Fatal("deleted CIDR should no longer match")
	}
	if err := s.DeleteIPBan(ban.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete again = %v, want ErrNotFound", err)
	}
}
