package store

import (
	"testing"
	"time"
)

func TestRateLimitWindowAndReset(t *testing.T) {
	s := openQuotaStore(t)
	const key = "alice|192.0.2.10"

	if blocked, err := s.RateBlocked(key, 3); err != nil || blocked {
		t.Fatalf("no record: blocked=%v err=%v, want false", blocked, err)
	}

	if err := s.RateFail(key, time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := s.RateFail(key, time.Minute); err != nil {
		t.Fatal(err)
	}
	if blocked, _ := s.RateBlocked(key, 3); blocked {
		t.Fatal("2 fails should not block with max=3")
	}
	if err := s.RateFail(key, time.Minute); err != nil {
		t.Fatal(err)
	}
	if blocked, _ := s.RateBlocked(key, 3); !blocked {
		t.Fatal("3 fails must block with max=3")
	}

	// 成功登录后清零
	if err := s.RateReset(key); err != nil {
		t.Fatal(err)
	}
	if blocked, _ := s.RateBlocked(key, 3); blocked {
		t.Fatal("reset must clear the counter")
	}

	// 窗口过期：RateBlocked 惰性清理已过期的记录
	if err := s.RateFail(key, -time.Second); err != nil {
		t.Fatal(err)
	}
	if blocked, _ := s.RateBlocked(key, 1); blocked {
		t.Fatal("expired window must not block")
	}

	// 窗口过期后 RateFail 重新计数（不会累加到旧窗口）
	if err := s.RateFail(key, -time.Second); err != nil {
		t.Fatal(err)
	}
	if err := s.RateFail(key, time.Minute); err != nil {
		t.Fatal(err)
	}
	if blocked, _ := s.RateBlocked(key, 2); blocked {
		t.Fatal("window should have restarted, count should be 1")
	}
}

func TestCleanupLoginFails(t *testing.T) {
	s := openQuotaStore(t)
	if err := s.RateFail("old|9.9.9.9", -time.Hour); err != nil {
		t.Fatal(err)
	}
	if err := s.RateFail("new|9.9.9.9", time.Hour); err != nil {
		t.Fatal(err)
	}
	n, err := s.CleanupLoginFails(time.Minute)
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if n != 1 {
		t.Fatalf("cleanup removed %d rows, want 1", n)
	}
	if blocked, _ := s.RateBlocked("new|9.9.9.9", 5); blocked {
		t.Fatal("fresh key should survive cleanup")
	}
}
