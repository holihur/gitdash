package api

import "testing"

// TestValidImportURLBlocksScpLikePrivateHost 覆盖安全评审 §2.3：scp-like 地址
// 曾完全绕过 importHostBlocked，导致 worker 拨号内网。
func TestValidImportURLBlocksScpLikePrivateHost(t *testing.T) {
	for _, raw := range []string{
		"git@127.0.0.1:foo/bar.git",
		"git@10.0.0.1:foo/bar.git",
		"git@192.168.1.1:foo/bar.git",
		"git@169.254.169.254:foo/bar.git",
		"git@[::1]:foo/bar.git",
	} {
		if _, err := validImportURL(raw); err == nil {
			t.Fatalf("validImportURL(%q) = nil error, want blocked", raw)
		}
	}
}

// TestValidImportURLAllowsScpLikePublicHost 使用公网 IP 字面量，避免依赖 DNS。
func TestValidImportURLAllowsScpLikePublicHost(t *testing.T) {
	raw := "git@1.1.1.1:owner/repo.git"
	if got, err := validImportURL(raw); err != nil || got != raw {
		t.Fatalf("validImportURL(%q) = %q, %v; want unchanged, nil", raw, got, err)
	}
}
