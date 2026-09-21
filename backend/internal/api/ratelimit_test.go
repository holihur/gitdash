package api

import "testing"

func TestWriteRateLimiterBurst(t *testing.T) {
	l := newWriteRateLimiter(6) // 6/min，burst=6
	for i := 0; i < 6; i++ {
		if !l.allow("k") {
			t.Fatalf("request %d within burst should be allowed", i+1)
		}
	}
	if l.allow("k") {
		t.Fatal("request beyond burst should be limited")
	}
	// 不同 key 互不影响
	if !l.allow("other") {
		t.Fatal("different key should have its own bucket")
	}
}

func TestWriteRateLimiterDisabledKey(t *testing.T) {
	l := newWriteRateLimiter(0) // 回退默认 240
	if !l.allow("x") {
		t.Fatal("default limiter should allow first request")
	}
}
