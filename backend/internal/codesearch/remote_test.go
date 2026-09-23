package codesearch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRemoteSearch(t *testing.T) {
	setupRepo(t)
	svc, err := NewService(ModeBleve, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = svc.Close() }()
	if err := svc.Index(context.Background(), "alice", "demo", testRepoID, "main"); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(NewSearchServer(svc, "secret").Handler())
	defer srv.Close()

	r := NewRemote(srv.URL, "secret", nil)
	hits, src, err := r.SearchSource(context.Background(), "alice", "demo", "package",
		Options{Ref: "main", Terms: []string{"package"}, RepoID: testRepoID})
	if err != nil {
		t.Fatal(err)
	}
	if src != SourceIndex {
		t.Fatalf("source = %q, want index", src)
	}
	if len(hits) != 2 {
		t.Fatalf("hits = %+v, want 2", hits)
	}
	// Search 便捷方法。
	hits, err = r.Search(context.Background(), "alice", "demo", "package",
		Options{Ref: "main", Terms: []string{"package"}, RepoID: testRepoID})
	if err != nil || len(hits) != 2 {
		t.Fatalf("Remote.Search = %+v, %v", hits, err)
	}
}

// 远程索引服务运行态代理。
func TestRemoteIndexStats(t *testing.T) {
	setupRepo(t)
	svc, err := NewService(ModeBleve, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = svc.Close() }()
	if err := svc.Index(context.Background(), "alice", "demo", testRepoID, "main"); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(NewSearchServer(svc, "secret").Handler())
	defer srv.Close()

	st, err := NewRemote(srv.URL, "secret", nil).IndexStats(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if st.Backend != ModeBleve || st.Repos != 1 || st.Documents == 0 {
		t.Fatalf("remote stats = %+v", st)
	}
	if _, err := NewRemote(srv.URL, "wrong", nil).IndexStats(context.Background()); err == nil {
		t.Fatal("expected auth error for wrong token")
	}
}

// 服务端非 200 且无 fallback 时返回错误。
func TestRemoteErrorNoFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()
	r := NewRemote(srv.URL, "", nil)
	_, _, err := r.SearchSource(context.Background(), "alice", "demo", "x", Options{})
	if err == nil {
		t.Fatal("expected error when server fails and no fallback")
	}
	if !strings.Contains(err.Error(), "500") && !strings.Contains(err.Error(), "Internal Server Error") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// token 不符时服务端 401，客户端回退到本机 grep。
func TestRemoteAuthFallback(t *testing.T) {
	setupRepo(t)
	svc, err := NewService(ModeBleve, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = svc.Close() }()
	_ = svc.Index(context.Background(), "alice", "demo", testRepoID, "main")
	srv := httptest.NewServer(NewSearchServer(svc, "secret").Handler())
	defer srv.Close()

	fg := &fakeSearcher{hits: []Hit{{Path: "fallback.go", Line: 1, Text: "x"}}}
	r := NewRemote(srv.URL, "wrong-token", fg)
	hits, src, err := r.SearchSource(context.Background(), "alice", "demo", "package",
		Options{Ref: "main", Terms: []string{"package"}, RepoID: testRepoID})
	if err != nil {
		t.Fatal(err)
	}
	if src != SourceGrep || !fg.called {
		t.Fatalf("expected grep fallback, src=%q called=%v", src, fg.called)
	}
	if len(hits) != 1 || hits[0].Path != "fallback.go" {
		t.Fatalf("hits = %+v", hits)
	}
}

func TestSearchServerRejectsInvalidInput(t *testing.T) {
	svc, err := NewService(ModeGrep, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(NewSearchServer(svc, "").Handler())
	defer srv.Close()

	post := func(body string) int {
		resp, err := http.Post(srv.URL+InternalSearchPath, "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		return resp.StatusCode
	}
	// 非法 owner（含 /）应被拒绝。
	if got := post(`{"owner":"a/b","name":"x","query":"q"}`); got != http.StatusBadRequest {
		t.Fatalf("invalid owner status = %d, want 400", got)
	}
	// 空 query 应被拒绝。
	if got := post(`{"owner":"a","name":"x","query":"  "}`); got != http.StatusBadRequest {
		t.Fatalf("empty query status = %d, want 400", got)
	}
	// 非法 JSON 应被拒绝。
	if got := post(`{not json`); got != http.StatusBadRequest {
		t.Fatalf("bad json status = %d, want 400", got)
	}
}
