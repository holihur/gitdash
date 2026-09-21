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

// TestValidImportURLGitBlocksPrivateByDefault 覆盖黑盒报告 S-02：git:// 曾反向
// 放行回环/私网、拦截公网。默认应与 http(s)/ssh 一致。
func TestValidImportURLGitBlocksPrivateByDefault(t *testing.T) {
	t.Setenv("GITDASH_IMPORT_ALLOW_PRIVATE_GIT", "")
	for _, raw := range []string{
		"git://127.0.0.1/x.git",
		"git://10.0.0.1/x.git",
		"git://192.168.1.100/x.git",
		"git://169.254.169.254/x.git",
	} {
		if _, err := validImportURL(raw); err == nil {
			t.Fatalf("validImportURL(%q) = nil error, want blocked", raw)
		}
	}
	if got, err := validImportURL("git://1.1.1.1/x.git"); err != nil || got != "git://1.1.1.1/x.git" {
		t.Fatalf("public git:// should be allowed, got %q, %v", got, err)
	}
}

// TestValidImportURLGitAllowPrivateOptIn 自托管内网 git 服务器需显式开启，
// 且 link-local/元数据地址即使在开启后也始终拦截。
func TestValidImportURLGitAllowPrivateOptIn(t *testing.T) {
	t.Setenv("GITDASH_IMPORT_ALLOW_PRIVATE_GIT", "1")
	if _, err := validImportURL("git://127.0.0.1/x.git"); err != nil {
		t.Fatalf("loopback git:// should be allowed when opted in: %v", err)
	}
	if _, err := validImportURL("git://169.254.169.254/x.git"); err == nil {
		t.Fatal("metadata address must stay blocked even when opted in")
	}
}
