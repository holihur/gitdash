package tests

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gitdash/backend/internal/api"
	"gitdash/backend/internal/store"
)

func newStaticServer(t *testing.T, dir string) *httptest.Server {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	hs := httptest.NewServer(api.New(st, "test").Handler(dir))
	t.Cleanup(hs.Close)
	return hs
}

func TestStaticHandler(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html>SPA</html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "assets", "app.js"), []byte("console.log(1)"), 0o644); err != nil {
		t.Fatal(err)
	}
	hs := newStaticServer(t, dir)

	get := func(path string) (int, string) {
		resp, err := http.Get(hs.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		buf := new(strings.Builder)
		readInto(resp.Body, buf)
		return resp.StatusCode, buf.String()
	}

	// 首页
	code, body := get("/")
	if code != 200 || !strings.Contains(body, "SPA") {
		t.Fatalf("index = %d %q", code, body)
	}
	// 静态资源
	code, body = get("/assets/app.js")
	if code != 200 || !strings.Contains(body, "console.log") {
		t.Fatalf("asset = %d %q", code, body)
	}
	// SPA fallback（前端路由）
	code, body = get("/repo/alice/demo")
	if code != 200 || !strings.Contains(body, "SPA") {
		t.Fatalf("spa fallback = %d %q", code, body)
	}
	// API 未匹配 → JSON 404，不落到 SPA
	code, body = get("/api/unknown")
	if code != 404 || strings.Contains(body, "SPA") {
		t.Fatalf("api 404 = %d %q", code, body)
	}
	// 静态目录之外放置秘密文件，确认无法穿越读取
	secretPath := filepath.Join(filepath.Dir(dir), "secret.txt")
	if err := os.WriteFile(secretPath, []byte("TOP-SECRET"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(secretPath) })

	code, body = get("/../secret.txt")
	if strings.Contains(body, "TOP-SECRET") {
		t.Fatalf("path traversal served: %d %q", code, body)
	}
	code, body = get("/%2e%2e/secret.txt")
	if strings.Contains(body, "TOP-SECRET") {
		t.Fatalf("encoded traversal served: %d %q", code, body)
	}
}

func TestEmbeddedFallbackWhenNoAssets(t *testing.T) {
	// 本仓库默认只 embed .gitkeep（HasAssets=false）时，
	// Handler("") 不应注册任何根路径处理器 → / 返回 404
	st, err := store.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	hs := httptest.NewServer(api.New(st, "test").Handler(""))
	t.Cleanup(hs.Close)

	resp, err := http.Get(hs.URL + "/some/path")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if embeddedAssetsPresent() {
		// 构建时 embed 了真实前端 → 期望 SPA fallback 200
		if resp.StatusCode != 200 {
			t.Fatalf("embedded assets present: status = %d", resp.StatusCode)
		}
		return
	}
	if resp.StatusCode != 404 {
		t.Fatalf("no embedded assets: status = %d, want 404", resp.StatusCode)
	}
}
