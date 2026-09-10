package api

import (
	"net/http/httptest"
	"testing"
)

func TestClientIPTrustedProxies(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set("X-Forwarded-For", "1.2.3.4")

	// 默认：只信任回环反代，采用 XFF
	t.Setenv("GITDASH_TRUSTED_PROXIES", "")
	if got := clientIP(req); got != "1.2.3.4" {
		t.Fatalf("loopback proxy XFF = %q, want 1.2.3.4", got)
	}

	// 配置了受信代理但不含回环 → 忽略 XFF
	t.Setenv("GITDASH_TRUSTED_PROXIES", "10.0.0.1")
	if got := clientIP(req); got != "127.0.0.1" {
		t.Fatalf("untrusted proxy must ignore XFF, got %q", got)
	}

	// CIDR 包含回环 → 采用 XFF
	t.Setenv("GITDASH_TRUSTED_PROXIES", "127.0.0.0/8")
	if got := clientIP(req); got != "1.2.3.4" {
		t.Fatalf("trusted CIDR XFF = %q, want 1.2.3.4", got)
	}
}
