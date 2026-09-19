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

// TestRegistrationDisabled 覆盖安全评审 §8.1：运维可通过
// GITDASH_DISABLE_REGISTRATION 关闭开放注册，收敛“人人可触发服务端 SSRF”的链条。
func TestRegistrationDisabled(t *testing.T) {
	t.Setenv("GITDASH_DISABLE_REGISTRATION", "")
	if registrationDisabled() {
		t.Fatal("registration should be enabled by default")
	}
	for _, v := range []string{"1", "true", "TRUE"} {
		t.Setenv("GITDASH_DISABLE_REGISTRATION", v)
		if !registrationDisabled() {
			t.Fatalf("registrationDisabled() = false for %q", v)
		}
	}
	t.Setenv("GITDASH_DISABLE_REGISTRATION", "0")
	if registrationDisabled() {
		t.Fatal("registrationDisabled() = true for \"0\"")
	}
}
