package api

import (
	"net/http/httptest"
	"testing"
)

func TestMatchGitHTTP(t *testing.T) {
	cases := []struct {
		method string
		target string
		ok     bool
		owner  string
		name   string
		svc    gitService
	}{
		{"GET", "/alice/demo.git/info/refs?service=git-upload-pack", true, "alice", "demo", gitUploadPack},
		{"GET", "/alice/demo.git/info/refs?service=git-receive-pack", true, "alice", "demo", gitReceivePack},
		{"POST", "/alice/demo.git/git-upload-pack", true, "alice", "demo", gitUploadPack},
		{"POST", "/alice/demo.git/git-receive-pack", true, "alice", "demo", gitReceivePack},
		// 不带 .git 后缀也接受
		{"GET", "/alice/demo/info/refs?service=git-upload-pack", true, "alice", "demo", gitUploadPack},
		// 缺 service / 未知 service
		{"GET", "/alice/demo.git/info/refs", false, "", "", ""},
		{"GET", "/alice/demo.git/info/refs?service=bogus", false, "", "", ""},
		// 保留前缀不应被误吞
		{"GET", "/api/alice/demo.git/info/refs?service=git-upload-pack", false, "", "", ""},
		{"GET", "/v2/alice/info/refs?service=git-upload-pack", false, "", "", ""},
		// 方法/形状不符
		{"DELETE", "/alice/demo.git/git-upload-pack", false, "", "", ""},
		{"POST", "/alice/demo.git/info/refs?service=git-upload-pack", false, "", "", ""},
	}
	for _, tc := range cases {
		r := httptest.NewRequest(tc.method, tc.target, nil)
		owner, name, svc, ok := matchGitHTTP(r)
		if ok != tc.ok || owner != tc.owner || name != tc.name || svc != tc.svc {
			t.Errorf("%s %s = (%q, %q, %q, %v), want (%q, %q, %q, %v)",
				tc.method, tc.target, owner, name, svc, ok, tc.owner, tc.name, tc.svc, tc.ok)
		}
	}
}
