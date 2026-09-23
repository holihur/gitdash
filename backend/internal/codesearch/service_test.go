package codesearch

import (
	"context"
	"errors"
	"testing"
)

type fakeSearcher struct {
	called bool
	hits   []Hit
	err    error
}

func (f *fakeSearcher) Search(_ context.Context, _, _, _ string, _ Options) ([]Hit, error) {
	f.called = true
	return f.hits, f.err
}

func TestNewServiceModes(t *testing.T) {
	grepOnly, err := NewService("grep", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if grepOnly.IndexingEnabled() {
		t.Fatal("grep mode must not enable indexing")
	}

	ix, err := NewService("bleve", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ix.Close() }()
	if !ix.IndexingEnabled() {
		t.Fatal("bleve mode must enable indexing")
	}

	if _, err := NewService("nope", t.TempDir()); err == nil {
		t.Fatal("unknown mode should error")
	}
}

// grep-only 模式下所有索引相关方法都应是安全的 no-op。
func TestServiceGrepOnlyNoops(t *testing.T) {
	svc, err := NewService(ModeGrep, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if svc.IndexingEnabled() {
		t.Fatal("grep-only must not enable indexing")
	}
	if err := svc.Index(ctx, "a", "b", 1, "main"); err != nil {
		t.Fatal(err)
	}
	svc.Merge(ctx)
	svc.Forget("a", "b")
	if svc.NeedsIndex("a", "b", 1, "main") {
		t.Fatal("NeedsIndex must be false without an index")
	}
	if got := svc.KnownRepos(); got != nil {
		t.Fatalf("KnownRepos = %v, want nil", got)
	}
	if err := svc.Close(); err != nil {
		t.Fatal(err)
	}
}

// 显式 grep 模式用 grep；bleve 模式未就绪返回 ErrIndexing，绝不回退 grep。
func TestServiceSearchBackends(t *testing.T) {
	// 显式 grep 模式。
	fg := &fakeSearcher{hits: []Hit{{Path: "x", Line: 1, Text: "y"}}}
	s := &Service{grep: fg}
	hits, src, err := s.SearchSource(context.Background(), "a", "b", "q", Options{})
	if err != nil || len(hits) != 1 || src != SourceGrep || !fg.called {
		t.Fatalf("grep-only search = %+v, %q, %v", hits, src, err)
	}

	// bleve 模式未就绪：ErrIndexing，grep 不被调用。
	b, err := OpenBleve(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = b.Close() }()
	fg2 := &fakeSearcher{}
	s2 := &Service{bleve: b, grep: fg2}
	if _, _, err := s2.SearchSource(context.Background(), "a", "b", "q", Options{}); !errors.Is(err, ErrIndexing) {
		t.Fatalf("not-ready bleve err = %v, want ErrIndexing", err)
	}
	if fg2.called {
		t.Fatal("grep must not be used as a fallback in bleve mode")
	}
}
