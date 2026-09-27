package updater

import (
	"net/http"
	"testing"
)

// TestValidReleaseURL 覆盖安全评审 §3.6：只信任 https 的 GitHub 下载地址。
func TestValidReleaseURL(t *testing.T) {
	ok := []string{
		"https://github.com/holihur/gitdash/releases/download/v1/x.tar.gz",
		"https://objects.githubusercontent.com/x",
		"https://github-releases.githubusercontent.com/x",
	}
	for _, u := range ok {
		if !validReleaseURL(u) {
			t.Fatalf("validReleaseURL(%q) = false", u)
		}
	}
	bad := []string{
		"http://github.com/x",
		"https://evil.example.com/x",
		"https://github.com.evil.example/x",
		"file:///etc/passwd",
		"",
	}
	for _, u := range bad {
		if validReleaseURL(u) {
			t.Fatalf("validReleaseURL(%q) = true, want false", u)
		}
	}
}

// TestRedirectRevalidation 覆盖安全审计 A4：下载重定向必须逐跳重新校验。
func TestRedirectRevalidation(t *testing.T) {
	mk := func(raw string) *http.Request {
		req, err := http.NewRequest("GET", raw, nil)
		if err != nil {
			t.Fatal(err)
		}
		return req
	}
	if err := httpClient.CheckRedirect(mk("https://github.com/x"), nil); err != nil {
		t.Fatalf("trusted redirect rejected: %v", err)
	}
	if err := httpClient.CheckRedirect(mk("https://evil.example.com/x"), nil); err == nil {
		t.Fatal("redirect to untrusted host accepted")
	}
	via := make([]*http.Request, 10)
	if err := httpClient.CheckRedirect(mk("https://github.com/x"), via); err == nil {
		t.Fatal("redirect chain longer than 10 accepted")
	}
}
