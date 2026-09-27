package store

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestRepoPins(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateUser("alice", "alice-pass-123"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateUser("bob", "bob-pass-1234"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a", "b", "c", "d", "e", "f", "g"} {
		if _, err := s.CreateRepo("alice", name, "", true); err != nil {
			t.Fatalf("create repo %s: %v", name, err)
		}
	}

	if pins, err := s.ListRepoPins("alice"); err != nil || len(pins) != 0 {
		t.Fatalf("initial pins = %v, %v", pins, err)
	}

	// 依次置顶，position 递增
	for i, name := range []string{"a", "b", "c"} {
		pins, err := s.PinRepo("alice", "alice", name)
		if err != nil {
			t.Fatalf("pin %s: %v", name, err)
		}
		if len(pins) != i+1 || pins[i].Repo != name || pins[i].Position != i {
			t.Fatalf("pins after %s = %+v", name, pins)
		}
	}

	// 重复置顶幂等
	pins, err := s.PinRepo("alice", "alice", "a")
	if err != nil || len(pins) != 3 {
		t.Fatalf("idempotent pin = %+v, %v", pins, err)
	}

	// 不存在用户
	if _, err := s.ListRepoPins("ghost"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown user pins err = %v", err)
	}

	// 上限 6
	for _, name := range []string{"d", "e", "f"} {
		if _, err := s.PinRepo("alice", "alice", name); err != nil {
			t.Fatalf("pin %s: %v", name, err)
		}
	}
	if _, err := s.PinRepo("alice", "alice", "g"); !errors.Is(err, ErrPinLimit) {
		t.Fatalf("over-limit err = %v, want ErrPinLimit", err)
	}

	// unpin 后位置保留，再次置顶排到末尾
	if _, err := s.UnpinRepo("alice", "alice", "b"); err != nil {
		t.Fatalf("unpin: %v", err)
	}
	pins, err = s.PinRepo("alice", "alice", "g")
	if err != nil {
		t.Fatalf("re-pin after unpin: %v", err)
	}
	if len(pins) != 6 || pins[len(pins)-1].Repo != "g" {
		t.Fatalf("pins after re-pin = %+v", pins)
	}
	if n, err := s.CountRepoPins("alice"); err != nil || n != 6 {
		t.Fatalf("count = %d, %v", n, err)
	}

	// 他人的 pin 互不影响
	if pins, err := s.ListRepoPins("bob"); err != nil || len(pins) != 0 {
		t.Fatalf("bob pins = %+v, %v", pins, err)
	}
	// unpin 幂等（不存在也不报错）
	if _, err := s.UnpinRepo("alice", "alice", "zzz"); err != nil {
		t.Fatalf("unpin missing = %v", err)
	}
}

func TestRepoPinsCascadeOnRepoDelete(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateUser("alice", "alice-pass-123"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateRepo("alice", "demo", "", true); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PinRepo("alice", "alice", "demo"); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteRepo("alice", "demo"); err != nil {
		t.Fatalf("delete repo: %v", err)
	}
	var n int64
	if err := s.db.Model(&repoPinRow{}).Count(&n).Error; err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("pins after repo delete = %d", n)
	}
}

func TestRepoPinsCascadeOnUserDelete(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateUser("alice", "alice-pass-123"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateRepo("alice", "demo", "", true); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PinRepo("alice", "alice", "demo"); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteUserAccount("alice"); err != nil {
		t.Fatalf("delete account: %v", err)
	}
	var n int64
	if err := s.db.Model(&repoPinRow{}).Count(&n).Error; err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("pins after user delete = %d", n)
	}
}
