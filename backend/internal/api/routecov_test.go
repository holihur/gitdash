package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestHandlerRegistersRoutes guards the inventory that feeds black-box
// endpoint-coverage reporting: Handler must expose every registered pattern.
func TestHandlerRegistersRoutes(t *testing.T) {
	a := New(nil, "test")
	if h := a.Handler(""); h == nil {
		t.Fatal("Handler returned nil")
	}
	routes := a.Routes()
	if len(routes) < 100 {
		t.Fatalf("route inventory looks truncated: %d patterns", len(routes))
	}
	want := map[string]bool{
		"GET /api/health":       false,
		"GET /api/repos":        false,
		"POST /api/repos":       false,
		"GET /metrics":          false,
		"GET /api/openapi.json": false,
	}
	for _, p := range routes {
		if _, ok := want[p]; ok {
			want[p] = true
		}
	}
	for p, found := range want {
		if !found {
			t.Errorf("route %q missing from inventory", p)
		}
	}
}

// TestRouteRecorderRecordsHits verifies the recorder observes the pattern
// matched by http.ServeMux (Request.Pattern) and persists inventory + hits.
func TestRouteRecorderRecordsHits(t *testing.T) {
	file := filepath.Join(t.TempDir(), "cov.txt")
	mux := newRouteMux()
	mux.HandleFunc("GET /api/ping", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("GET /api/nope", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	srv := httptest.NewServer(newRouteRecorder(mux, file, mux.Routes()))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/ping")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()

	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	for _, want := range []string{"route\tGET /api/ping", "hit\tGET /api/ping"} {
		if !strings.Contains(got, want) {
			t.Fatalf("coverage file missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "hit\tGET /api/nope") {
		t.Fatalf("unvisited route recorded as hit:\n%s", got)
	}
}
