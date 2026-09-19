package updater

import "testing"

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
