package store

import (
	"fmt"
	"path/filepath"
	"testing"
)

func newBadgeStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateUser("alice", "alice-pass-123456"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateRepo("alice", "demo", "d", false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateOrg("acme", "ACME", "alice"); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestBadgeCRUDAndImage(t *testing.T) {
	s := newBadgeStore(t)
	b, err := s.CreateBadge("verified", "Verified", "verified account")
	if err != nil {
		t.Fatal(err)
	}
	if b.HasImage {
		t.Fatal("new badge should have no image")
	}
	if _, err := s.CreateBadge("verified", "Dup", ""); err != ErrExists {
		t.Fatalf("want ErrExists, got %v", err)
	}
	if err := s.SetBadgeImage(b.ID, "image/png", []byte("png-bytes")); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetBadge(b.ID)
	if err != nil || !got.HasImage || got.ImageUpdatedAt == "" {
		t.Fatalf("badge with image = %+v, %v", got, err)
	}
	ct, data, err := s.GetBadgeImage(b.ID)
	if err != nil || ct != "image/png" || string(data) != "png-bytes" {
		t.Fatalf("image = %q %q %v", ct, data, err)
	}
	label := "Verified Pro"
	if _, err := s.UpdateBadge(b.ID, BadgeUpdate{Label: &label}); err != nil {
		t.Fatal(err)
	}
	list, _ := s.ListBadges()
	if len(list) != 1 || list[0].Label != "Verified Pro" || !list[0].HasImage {
		t.Fatalf("list = %+v", list)
	}
}

func TestBadgeGrantDisplayRevoke(t *testing.T) {
	s := newBadgeStore(t)
	ids := make([]int64, 0, 5)
	for i := 0; i < 5; i++ {
		b, err := s.CreateBadge("b"+string(rune('a'+i)), "B", "")
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, b.ID)
	}
	// 任意徽章可发给用户 / 仓库 / 组织。
	for _, k := range []struct{ kind, owner, repo string }{
		{"user", "alice", ""},
		{"repo", "alice", "demo"},
		{"org", "acme", ""},
	} {
		if !s.BadgeTargetExists(k.kind, k.owner, k.repo) {
			t.Fatalf("target %v should exist", k)
		}
	}
	if s.BadgeTargetExists("user", "nobody", "") || s.BadgeTargetExists("repo", "alice", "nope") {
		t.Fatal("missing targets should not exist")
	}

	// 授予 5 个给 user：全部获得，但默认只挂前 3 个。
	for _, id := range ids {
		if err := s.GrantBadge(id, "user", "alice", ""); err != nil {
			t.Fatal(err)
		}
	}
	granted, _ := s.GrantedBadges("user", "alice", "")
	if len(granted) != 5 {
		t.Fatalf("granted = %d", len(granted))
	}
	disp, _ := s.DisplayedBadges("user", "alice", "")
	if len(disp) != MaxDisplayedBadges {
		t.Fatalf("auto display = %d, want %d", len(disp), MaxDisplayedBadges)
	}
	// 重复授予
	if err := s.GrantBadge(ids[0], "user", "alice", ""); err != ErrExists {
		t.Fatalf("want ErrExists, got %v", err)
	}

	// 只接受已授予的，去重，截断到 3。
	if err := s.SetDisplayedBadges("user", "alice", "", []int64{ids[4], ids[3], ids[2], 9999, ids[4]}); err != nil {
		t.Fatal(err)
	}
	disp, _ = s.DisplayedBadges("user", "alice", "")
	if len(disp) != 3 || disp[0].ID != ids[4] || disp[1].ID != ids[3] || disp[2].ID != ids[2] {
		t.Fatalf("display order = %+v", disp)
	}

	// 撤销后从展示中移除。
	if err := s.RevokeBadge(ids[4], "user", "alice", ""); err != nil {
		t.Fatal(err)
	}
	granted, _ = s.GrantedBadges("user", "alice", "")
	if len(granted) != 4 {
		t.Fatalf("granted after revoke = %d", len(granted))
	}
	disp, _ = s.DisplayedBadges("user", "alice", "")
	for _, b := range disp {
		if b.ID == ids[4] {
			t.Fatal("revoked badge still displayed")
		}
	}
	if err := s.RevokeBadge(ids[4], "user", "alice", ""); err != ErrNotFound {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestDisplayedBadgesBatch(t *testing.T) {
	s := newBadgeStore(t)
	b1, _ := s.CreateBadge("x1", "X1", "")
	b2, _ := s.CreateBadge("x2", "X2", "")
	if err := s.GrantBadge(b1.ID, "user", "alice", ""); err != nil {
		t.Fatal(err)
	}
	if err := s.GrantBadge(b2.ID, "repo", "alice", "demo"); err != nil {
		t.Fatal(err)
	}

	m, err := s.DisplayedBadgesBatch("user", [][2]string{{"alice", ""}})
	if err != nil {
		t.Fatal(err)
	}
	if got := m["alice/"]; len(got) != 1 || got[0].ID != b1.ID {
		t.Fatalf("user batch = %+v", m)
	}

	// 60 个 target 跨越两个 chunk，仍能命中目标。
	targets := make([][2]string, 0, 60)
	targets = append(targets, [2]string{"alice", "demo"})
	for i := 0; i < 59; i++ {
		targets = append(targets, [2]string{"alice", fmt.Sprintf("nope%d", i)})
	}
	m2, err := s.DisplayedBadgesBatch("repo", targets)
	if err != nil {
		t.Fatal(err)
	}
	if got := m2["alice/demo"]; len(got) != 1 || got[0].ID != b2.ID {
		t.Fatalf("repo batch = %+v", m2)
	}
}

func TestBadgeDeleteCascades(t *testing.T) {
	s := newBadgeStore(t)
	b, _ := s.CreateBadge("x", "X", "")
	if err := s.SetBadgeImage(b.ID, "image/png", []byte("x")); err != nil {
		t.Fatal(err)
	}
	if err := s.GrantBadge(b.ID, "repo", "alice", "demo"); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteBadge(b.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetBadge(b.ID); err != ErrNotFound {
		t.Fatalf("badge should be gone, got %v", err)
	}
	if _, _, err := s.GetBadgeImage(b.ID); err != ErrNotFound {
		t.Fatalf("image should be gone, got %v", err)
	}
	grants, _ := s.ListBadgeGrants(b.ID)
	if len(grants) != 0 {
		t.Fatalf("grants should be gone, got %d", len(grants))
	}
	if err := s.DeleteBadge(b.ID); err != ErrNotFound {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}
