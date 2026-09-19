package gitsvc

import "testing"

func TestRemoteURLBlocked(t *testing.T) {
	blocked := []string{
		"",
		"http://127.0.0.1/evil.git",
		"https://10.0.0.1/evil.git",
		"http://192.168.1.1/evil.git",
		"https://172.16.0.5/evil.git",
		"http://169.254.169.254/latest/meta-data/",
		"http://100.100.100.200/x",
		"ssh://git@192.168.0.10/srv/evil.git",
		"git@127.0.0.1:foo/bar.git",
		"git@10.0.0.1:foo/bar.git",
		"git@[::1]:foo/bar.git",
		"file:///etc/passwd",
	}
	for _, u := range blocked {
		if !RemoteURLBlocked(u) {
			t.Fatalf("RemoteURLBlocked(%q) = false, want blocked", u)
		}
	}

	allowed := []string{
		"https://1.1.1.1/owner/repo.git", // 公网 IP 字面量（不触发 DNS）
		"git@1.1.1.1:owner/repo.git",
		"/tmp/local.git",
		"./local",
		"../local",
		"local",
	}
	for _, u := range allowed {
		if RemoteURLBlocked(u) {
			t.Fatalf("RemoteURLBlocked(%q) = true, want allowed", u)
		}
	}
}

func TestRemoteURLBlockedAllowPrivate(t *testing.T) {
	t.Setenv("GITDASH_SSRF_ALLOW_PRIVATE", "1")
	if RemoteURLBlocked("git@127.0.0.1:foo/bar.git") {
		t.Fatal("loopback scp-like should be allowed when GITDASH_SSRF_ALLOW_PRIVATE=1")
	}
}
