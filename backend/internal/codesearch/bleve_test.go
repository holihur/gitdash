package codesearch

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gitdash/backend/internal/gitsvc"
)

// TestMain 拦截 post-receive hook 回调（WriteCommit 的 push 会触发；hook 以
// `测试二进制 post-receive owner repo` 执行本二进制）。测试不关心 push 事件，直接成功退出。
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && (os.Args[1] == "post-receive" || os.Args[1] == "pre-receive") {
		_, _ = io.Copy(io.Discard, os.Stdin)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func seedRepo(t *testing.T, owner, name string, files map[string]string) {
	t.Helper()
	if err := gitsvc.CreateBare(owner, name); err != nil {
		t.Fatal(err)
	}
	changes := make([]gitsvc.FileChange, 0, len(files))
	for p, c := range files {
		changes = append(changes, gitsvc.FileChange{Path: p, Action: "create", Content: c})
	}
	if _, err := gitsvc.WriteCommit(owner, name, "main", "init", "tester", changes); err != nil {
		t.Fatal(err)
	}
}

func setupRepo(t *testing.T) {
	t.Helper()
	if err := gitsvc.Init(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	seedRepo(t, "alice", "demo", map[string]string{
		"main.go":        "package main\n\nfunc main() {\n\tprintln(\"hello world\")\n}\n",
		"docs/readme.md": "Hello World\nanother line\n",
		"src/util.go":    "package util\n\nfunc Add(a, b int) int { return a + b }\n",
	})
}

// 测试中 alice/demo 的稳定数据库 ID。
const testRepoID int64 = 7

func newIndexedBleve(t *testing.T) *Bleve {
	t.Helper()
	b, err := OpenBleve(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Close() })
	if err := b.Index(context.Background(), "alice", "demo", testRepoID, "main"); err != nil {
		t.Fatal(err)
	}
	return b
}

func TestBleveIndexAndSearch(t *testing.T) {
	setupRepo(t)
	b := newIndexedBleve(t)
	if !b.Ready("alice", "demo", "main", testRepoID) {
		t.Fatal("expected repo to be ready after indexing")
	}

	// 小写查询大小写不敏感：main.go 与 readme.md（Hello World）都应命中。
	hits, err := b.Search(context.Background(), "alice", "demo", "hello", Options{Ref: "main", Max: 10, Terms: []string{"hello"}})
	if err != nil {
		t.Fatal(err)
	}
	gotPaths := map[string]bool{}
	for _, h := range hits {
		gotPaths[h.Path] = true
	}
	if len(hits) != 2 || !gotPaths["main.go"] || !gotPaths["docs/readme.md"] {
		t.Fatalf("hello hits = %+v", hits)
	}

	// 多关键词 AND：两行都同时含 hello 与 world。
	hits, err = b.Search(context.Background(), "alice", "demo", "hello world", Options{Ref: "main", Max: 10, Terms: []string{"hello", "world"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 2 {
		t.Fatalf("hello+world hits = %+v, want 2", hits)
	}

	// 路径过滤：只应返回 *.go。
	hits, err = b.Search(context.Background(), "alice", "demo", "package", Options{Ref: "main", Max: 10, Pathspec: []string{"*.go"}, Terms: []string{"package"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 2 {
		t.Fatalf("package hits = %+v, want 2", hits)
	}
	for _, h := range hits {
		if !strings.HasSuffix(h.Path, ".go") {
			t.Fatalf("pathspec filter leaked %q", h.Path)
		}
	}

	// 无命中。
	hits, err = b.Search(context.Background(), "alice", "demo", "zzzznotfound", Options{Ref: "main", Terms: []string{"zzzznotfound"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 0 {
		t.Fatalf("expected no hits, got %+v", hits)
	}

	// 其它仓库不应命中（repo 过滤）。
	hits, err = b.Search(context.Background(), "bob", "demo", "package", Options{Ref: "main", Terms: []string{"package"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 0 {
		t.Fatalf("cross-repo leak: %+v", hits)
	}
}

// 代码标识符体验：拆分 camelCase/下划线/点号 + 智能大小写 + 前缀匹配。
func TestBleveIdentifierSearch(t *testing.T) {
	if err := gitsvc.Init(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	seedRepo(t, "alice", "ident", map[string]string{
		"a.go": "package a\nvar uniqueAlphaToken = 1\nvar foo_bar = 2\nfunc fmtPrintln() {}\nXMLParser := 3\n",
	})
	b, err := OpenBleve(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = b.Close() }()
	if err := b.Index(context.Background(), "alice", "ident", 5, "main"); err != nil {
		t.Fatal(err)
	}
	found := func(q string) bool {
		hits, err := b.Search(context.Background(), "alice", "ident", q, Options{Ref: "main", Terms: []string{q}, Max: 10})
		if err != nil {
			t.Fatal(err)
		}
		return len(hits) > 0
	}
	for _, q := range []string{"alpha", "Alpha", "unique", "Token", "foo", "bar", "println", "Parser", "Parse", "parse", "XML"} {
		if !found(q) {
			t.Errorf("identifier query %q should match", q)
		}
	}
	// 智能大小写：全大写词按区分大小写匹配，raw text 里没有 "ALPHA"。
	if found("ALPHA") {
		t.Error("ALPHA should not match uniqueAlphaToken (smart case)")
	}
}

// CJK 支持：中文/日文/韓文可搜，且保持短语（连续子串）语义。
func TestBleveCJKSearch(t *testing.T) {
	if err := gitsvc.Init(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	seedRepo(t, "alice", "cjk", map[string]string{
		"zh.go": "package zh\n// 这是一个中文代码搜索功能\nvar 混合abc中文def = 1\n",
		"ja.go": "package ja\n// 日本語のコード検索\n",
		"ko.go": "package ko\n// 한국어 코드 검색\n",
	})
	b, err := OpenBleve(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = b.Close() }()
	if err := b.Index(context.Background(), "alice", "cjk", 11, "main"); err != nil {
		t.Fatal(err)
	}
	find := func(q string) []Hit {
		hits, err := b.Search(context.Background(), "alice", "cjk", q, Options{Ref: "main", Terms: []string{q}, Max: 20})
		if err != nil {
			t.Fatal(err)
		}
		return hits
	}
	for _, q := range []string{"中文", "搜索", "代码搜索", "这是一个", "混合", "検索", "コード", "검색", "한국어"} {
		if len(find(q)) == 0 {
			t.Errorf("CJK query %q should match", q)
		}
	}
	// 非连续字符不应命中（短语语义，避免单字并集误报）。
	if got := find("搜功"); len(got) != 0 {
		t.Errorf("non-contiguous 搜功 should not match: %+v", got)
	}
}

func TestBleveNeedsIndexAndForget(t *testing.T) {
	setupRepo(t)
	b := newIndexedBleve(t)

	if b.NeedsIndex("alice", "demo", testRepoID, "main") {
		t.Fatal("index should be up to date")
	}
	if !b.NeedsIndex("alice", "other", testRepoID, "main") {
		t.Fatal("unknown repo should need an index")
	}
	// 同名但 ID 不同（删除后重建）必须视为需要重建，且不可就绪。
	if !b.NeedsIndex("alice", "demo", testRepoID+1, "main") {
		t.Fatal("recreated repo (new id) should need an index")
	}
	if b.Ready("alice", "demo", "main", testRepoID+1) {
		t.Fatal("stale index must not be ready for a different repo id")
	}

	b.Forget("alice", "demo")
	if b.Ready("alice", "demo", "main", testRepoID) {
		t.Fatal("should not be ready after Forget")
	}
	if !b.NeedsIndex("alice", "demo", testRepoID, "main") {
		t.Fatal("should need index after Forget")
	}
	hits, err := b.Search(context.Background(), "alice", "demo", "package", Options{Terms: []string{"package"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 0 {
		t.Fatalf("docs should be gone after Forget: %+v", hits)
	}
}

func TestBleveMetaPersistsAcrossReopen(t *testing.T) {
	setupRepo(t)
	dir := t.TempDir()
	b, err := OpenBleve(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Index(context.Background(), "alice", "demo", testRepoID, "main"); err != nil {
		t.Fatal(err)
	}
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}

	b2, err := OpenBleve(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = b2.Close() }()
	if !b2.Ready("alice", "demo", "main", testRepoID) {
		t.Fatal("ready state should persist across reopen")
	}
	hits, err := b2.Search(context.Background(), "alice", "demo", "package", Options{Ref: "main", Terms: []string{"package"}})
	if err != nil || len(hits) == 0 {
		t.Fatalf("search after reopen = %+v, %v", hits, err)
	}
}

// 增量重建：只重建变更/新增文件，删除移除文件，未变更文件保留。
func TestBleveIncrementalUpdate(t *testing.T) {
	if err := gitsvc.Init(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if err := gitsvc.CreateBare("alice", "inc"); err != nil {
		t.Fatal(err)
	}
	if _, err := gitsvc.WriteCommit("alice", "inc", "main", "init", "tester", []gitsvc.FileChange{
		{Path: "keep.go", Action: "create", Content: "package keep\n// KEEPTOKEN\n"},
		{Path: "change.go", Action: "create", Content: "package change\n// OLDTOKEN\n"},
		{Path: "remove.go", Action: "create", Content: "package remove\n// REMOVETOKEN\n"},
	}); err != nil {
		t.Fatal(err)
	}
	b, err := OpenBleve(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = b.Close() }()
	ctx := context.Background()
	const id int64 = 42
	if err := b.Index(ctx, "alice", "inc", id, "main"); err != nil {
		t.Fatal(err)
	}
	for _, tok := range []string{"KEEPTOKEN", "OLDTOKEN", "REMOVETOKEN"} {
		if got := searchTokens(t, b, tok); len(got) == 0 {
			t.Fatalf("baseline: %s not found", tok)
		}
	}

	// 第二次提交：改 change.go、删 remove.go、加 added.go。
	if _, err := gitsvc.WriteCommit("alice", "inc", "main", "change", "tester", []gitsvc.FileChange{
		{Path: "change.go", Action: "update", Content: "package change\n// NEWTOKEN\n"},
		{Path: "remove.go", Action: "delete"},
		{Path: "added.go", Action: "create", Content: "package added\n// ADDEDTOKEN\n"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := b.Index(ctx, "alice", "inc", id, "main"); err != nil {
		t.Fatal(err)
	}

	if got := searchTokens(t, b, "KEEPTOKEN"); len(got) == 0 {
		t.Fatal("unchanged file lost after incremental reindex")
	}
	if got := searchTokens(t, b, "NEWTOKEN"); len(got) == 0 {
		t.Fatal("updated content not indexed")
	}
	if got := searchTokens(t, b, "ADDEDTOKEN"); len(got) == 0 {
		t.Fatal("added file not indexed")
	}
	if got := searchTokens(t, b, "OLDTOKEN"); len(got) != 0 {
		t.Fatalf("stale content still indexed: %+v", got)
	}
	if got := searchTokens(t, b, "REMOVETOKEN"); len(got) != 0 {
		t.Fatalf("deleted file still indexed: %+v", got)
	}
}

// Service 的 Indexer/Searcher 生命周期：Index/NeedsIndex/KnownRepos/Merge/Forget/Close。
func TestServiceIndexerLifecycle(t *testing.T) {
	setupRepo(t)
	svc, err := NewService(ModeBleve, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := svc.Index(ctx, "alice", "demo", testRepoID, "main"); err != nil {
		t.Fatal(err)
	}
	if !svc.IndexingEnabled() {
		t.Fatal("indexing should be enabled")
	}
	if got := svc.KnownRepos(); len(got) != 1 || got[0] != "alice/demo" {
		t.Fatalf("KnownRepos = %v", got)
	}
	if svc.NeedsIndex("alice", "demo", testRepoID, "main") {
		t.Fatal("should not need index after Index")
	}
	svc.Merge(ctx)
	hits, err := svc.Search(ctx, "alice", "demo", "package", Options{Ref: "main", Terms: []string{"package"}, RepoID: testRepoID})
	if err != nil || len(hits) != 2 {
		t.Fatalf("Service.Search = %v, %v", hits, err)
	}
	svc.Forget("alice", "demo")
	if len(svc.KnownRepos()) != 0 {
		t.Fatal("KnownRepos should be empty after Forget")
	}
	if err := svc.Close(); err != nil {
		t.Fatal(err)
	}
}

// 索引运行态快照：仓库/文档规模、待重建数、grep 模式。
func TestServiceIndexStats(t *testing.T) {
	setupRepo(t)
	svc, err := NewService(ModeBleve, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = svc.Close() }()
	if err := svc.Index(context.Background(), "alice", "demo", testRepoID, "main"); err != nil {
		t.Fatal(err)
	}
	st, err := svc.IndexStats(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if st.Backend != ModeBleve || st.Repos != 1 || st.Documents == 0 || st.Dirty != 0 {
		t.Fatalf("stats = %+v", st)
	}

	// 写入待重建标记后：dirty 计数 +1，且不再就绪。
	b := svc.bleve
	if err := os.MkdirAll(filepath.Dir(b.dirtyMarker("alice", "demo")), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b.dirtyMarker("alice", "demo"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if st, _ := svc.IndexStats(context.Background()); st.Dirty != 1 {
		t.Fatalf("dirty stats = %+v", st)
	}
	if b.Ready("alice", "demo", "main", testRepoID) {
		t.Fatal("dirty repo must not be ready")
	}

	// grep 模式。
	gs, err := NewService(ModeGrep, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if st, _ := gs.IndexStats(context.Background()); st.Backend != ModeGrep {
		t.Fatalf("grep stats = %+v", st)
	}
}

// Index 完成后，若 HEAD 未推进则清除「待重建」标记并转为就绪。
func TestBleveIndexClearsDirty(t *testing.T) {
	setupRepo(t)
	b, err := OpenBleve(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = b.Close() }()
	// 模拟 post-receive hook 写入的待重建标记。
	if err := os.MkdirAll(filepath.Dir(b.dirtyMarker("alice", "demo")), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b.dirtyMarker("alice", "demo"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if b.Ready("alice", "demo", "main", testRepoID) {
		t.Fatal("dirty repo must not be ready")
	}
	if err := b.Index(context.Background(), "alice", "demo", testRepoID, "main"); err != nil {
		t.Fatal(err)
	}
	if b.isDirty("alice", "demo") {
		t.Fatal("Index must clear the dirty marker when HEAD is unchanged")
	}
	if !b.Ready("alice", "demo", "main", testRepoID) {
		t.Fatal("repo should be ready after Index")
	}
}

// 指定 ref 不存在时回退到 HEAD 索引。
func TestBleveIndexFallsBackToHead(t *testing.T) {
	setupRepo(t)
	b, err := OpenBleve(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = b.Close() }()
	if err := b.Index(context.Background(), "alice", "demo", testRepoID, "does-not-exist"); err != nil {
		t.Fatal(err)
	}
	if b.NeedsIndex("alice", "demo", testRepoID, "main") {
		t.Fatal("index should have fallen back to HEAD (main)")
	}
}

// Word 匹配与超长行截断。
func TestBleveWordAndTruncate(t *testing.T) {
	if err := gitsvc.Init(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	longLine := "LONGTOKEN " + strings.Repeat("x", 1500)
	seedRepo(t, "alice", "wt", map[string]string{
		"a.go":     "package a\nfunc main() {}\n",
		"b.go":     "package b\nvar foo_main int\n",
		"long.txt": longLine + "\n",
	})
	b, err := OpenBleve(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = b.Close() }()
	if err := b.Index(context.Background(), "alice", "wt", 9, "main"); err != nil {
		t.Fatal(err)
	}

	// word 匹配：main 只应命中 a.go 的 `func main()`，排除 b.go 的 `foo_main`。
	hits, err := b.Search(context.Background(), "alice", "wt", "main", Options{Ref: "main", Word: true, Terms: []string{"main"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range hits {
		if h.Path != "a.go" {
			t.Fatalf("word match leaked %q: %+v", h.Path, h)
		}
	}
	if len(hits) == 0 {
		t.Fatal("word search for main returned nothing")
	}

	// 超长行截断后仍可检索，且存储文本不超过上限。
	hits, err = b.Search(context.Background(), "alice", "wt", "LONGTOKEN", Options{Ref: "main", Terms: []string{"LONGTOKEN"}})
	if err != nil || len(hits) != 1 {
		t.Fatalf("long token hits = %+v, %v", hits, err)
	}
	if len(hits[0].Text) > maxIndexLineBytes {
		t.Fatalf("stored text not truncated: %d bytes", len(hits[0].Text))
	}
}

func TestFieldInt(t *testing.T) {
	cases := []struct {
		in   interface{}
		want int
	}{
		{float64(5), 5},
		{int64(6), 6},
		{7, 7},
		{"x", 0},
		{nil, 0},
	}
	for _, c := range cases {
		if got := fieldInt(c.in); got != c.want {
			t.Errorf("fieldInt(%v) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestSkipIndexPath(t *testing.T) {
	skip := []string{"node_modules/x.js", "vendor/a/b.go", "pkg/package-lock.json", "a.min.js", ".git/config", "dist/app.js"}
	for _, p := range skip {
		if !skipIndexPath(p) {
			t.Errorf("skipIndexPath(%q) = false, want true", p)
		}
	}
	keep := []string{"src/main.go", "README.md", "a/b/c.ts"}
	for _, p := range keep {
		if skipIndexPath(p) {
			t.Errorf("skipIndexPath(%q) = true, want false", p)
		}
	}
}

func searchTokens(t *testing.T, b *Bleve, tok string) []Hit {
	t.Helper()
	hits, err := b.Search(context.Background(), "alice", "inc", tok, Options{Ref: "main", Terms: []string{tok}, Max: 50})
	if err != nil {
		t.Fatal(err)
	}
	return hits
}

// 强制段合并后索引仍可正常检索。
func TestBleveMerge(t *testing.T) {
	setupRepo(t)
	b := newIndexedBleve(t)
	b.Merge(context.Background())
	hits, err := b.Search(context.Background(), "alice", "demo", "package", Options{Ref: "main", Terms: []string{"package"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 2 {
		t.Fatalf("after merge hits = %+v, want 2", hits)
	}
}

// 仓库 ID 不符（同名重建）/ 未就绪 / 请求非默认分支时，Service 返回错误而**不回退 grep**。
func TestServiceIndexOnlyNoGrepFallback(t *testing.T) {
	setupRepo(t)
	b := newIndexedBleve(t)
	fg := &fakeSearcher{}
	svc := &Service{grep: fg, bleve: b}

	// 正确 ID：走索引，且不触碰 grep。
	if _, src, err := svc.SearchSource(context.Background(), "alice", "demo", "package", Options{Ref: "main", Terms: []string{"package"}, RepoID: testRepoID}); err != nil || src != SourceIndex {
		t.Fatalf("index search = %q, %v", src, err)
	}
	if fg.called {
		t.Fatal("grep must not be used in bleve mode")
	}

	// ID 不符（仓库被删除后同名重建）：ErrIndexing，不回退 grep。
	if _, _, err := svc.SearchSource(context.Background(), "alice", "demo", "package", Options{Ref: "main", Terms: []string{"package"}, RepoID: testRepoID + 1}); !errors.Is(err, ErrIndexing) {
		t.Fatalf("stale repo id err = %v, want ErrIndexing", err)
	}

	// RepoID 未知（0）：同样 ErrIndexing。
	if _, _, err := svc.SearchSource(context.Background(), "alice", "demo", "package", Options{Ref: "main", Terms: []string{"package"}}); !errors.Is(err, ErrIndexing) {
		t.Fatalf("zero repo id err = %v, want ErrIndexing", err)
	}

	// 非默认分支：ErrRefNotIndexed。
	if _, _, err := svc.SearchSource(context.Background(), "alice", "demo", "package", Options{Ref: "feature", Terms: []string{"package"}, RepoID: testRepoID}); !errors.Is(err, ErrRefNotIndexed) {
		t.Fatalf("non-default ref err = %v, want ErrRefNotIndexed", err)
	}

	if fg.called {
		t.Fatal("grep must never be used in bleve mode")
	}
}

// 索引是 token 级的：子串查询（grep 能命中）在索引里可能召回不到。
// 这是 Bleve 全文检索与固定字符串 grep 的语义差异，Service 会在索引未就绪时回退。
func TestBleveIsTokenBased(t *testing.T) {
	setupRepo(t)
	b := newIndexedBleve(t)
	hits, err := b.Search(context.Background(), "alice", "demo", "ello", Options{Ref: "main", Terms: []string{"ello"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 0 {
		t.Fatalf("token index unexpectedly matched substring: %+v", hits)
	}
	// 同样的子串，实时 grep 能命中。
	gh, err := NewGrep().Search(context.Background(), "alice", "demo", "ello", Options{Ref: "main", Terms: []string{"ello"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(gh) == 0 {
		t.Fatal("grep should match the substring")
	}
}
