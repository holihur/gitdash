package api

import (
	"net/http/httptest"
	"testing"
)

func TestSetStaticCache(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"assets/index-abc123.js", "public, max-age=31536000, immutable"},
		{"/assets/index-abc123.css", "public, max-age=31536000, immutable"},
		{"index.html", "no-cache, must-revalidate"},
		{"admin.html", "no-cache, must-revalidate"},
		{"favicon.ico", "no-cache, must-revalidate"},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		setStaticCache(rec, tc.path)
		if got := rec.Header().Get("Cache-Control"); got != tc.want {
			t.Errorf("setStaticCache(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}
