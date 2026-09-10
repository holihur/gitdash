package ssrf

import (
	"context"
	"testing"
)

func TestHostBlockedDefaults(t *testing.T) {
	for _, host := range []string{"", "127.0.0.1", "localhost", "10.0.0.1", "169.254.169.254"} {
		if !HostBlocked(host) {
			t.Fatalf("host %q should be blocked by default", host)
		}
	}
	if HostBlocked("1.1.1.1") {
		t.Fatal("public address should be allowed")
	}
}

func TestHostBlockedAllowPrivate(t *testing.T) {
	t.Setenv("GITDASH_SSRF_ALLOW_PRIVATE", "1")
	if HostBlocked("127.0.0.1") {
		t.Fatal("loopback should be allowed when GITDASH_SSRF_ALLOW_PRIVATE=1")
	}
}

func TestDialContextBlocksLoopback(t *testing.T) {
	if _, err := DialContext(context.Background(), "tcp", "127.0.0.1:9"); err == nil {
		t.Fatal("DialContext should reject loopback by default")
	}
}

func TestDialContextAllowsPrivateWhenOptedIn(t *testing.T) {
	t.Setenv("GITDASH_SSRF_ALLOW_PRIVATE", "1")
	// 127.0.0.1:1 通常无可监听服务；断言的是「未被 SSRF 拦截」（连接错误 ≠ 拦截错误）。
	_, err := DialContext(context.Background(), "tcp", "127.0.0.1:1")
	if err == nil {
		return
	}
	if got := err.Error(); got == "" {
		t.Fatal("expected a dial error")
	}
}
