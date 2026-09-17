package pipeline

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestParseCacheBlock(t *testing.T) {
	src := "cache:\n  key: go-mod\n  paths:\n    - vendor\n    - .cache/go\nsteps:\n  - run: echo hi\n"
	cfg, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg.CacheKey != "go-mod" {
		t.Fatalf("key = %q", cfg.CacheKey)
	}
	if len(cfg.CachePaths) != 2 || cfg.CachePaths[0] != "vendor" || cfg.CachePaths[1] != ".cache/go" {
		t.Fatalf("paths = %v", cfg.CachePaths)
	}
}

func TestParseCacheDefaultsKey(t *testing.T) {
	cfg, err := Parse([]byte("cache:\n  paths:\n    - vendor\nsteps:\n  - run: x\n"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg.CacheKey != "default" {
		t.Fatalf("default key = %q", cfg.CacheKey)
	}
}

func TestParseCacheRejects(t *testing.T) {
	if _, err := Parse([]byte("cache:\n  key: k\nsteps:\n  - run: x\n")); err == nil {
		t.Fatal("cache without paths should fail")
	}
	if _, err := Parse([]byte("cache:\n  paths:\n    - /etc/passwd\nsteps:\n  - run: x\n")); err == nil {
		t.Fatal("absolute cache path should fail")
	}
	if _, err := Parse([]byte("cache:\n  paths:\n    - ../escape\nsteps:\n  - run: x\n")); err == nil {
		t.Fatal("traversal cache path should fail")
	}
}

func TestCacheRoundTrip(t *testing.T) {
	SetCacheDir(t.TempDir())
	t.Cleanup(func() { SetCacheDir("") })

	job := RunJob{Owner: "alice", Repo: "r", RunID: 1, Ref: "main"}
	cfg := &Config{CacheKey: "k", CachePaths: []string{"vendor"}}

	work := t.TempDir()
	if err := os.MkdirAll(filepath.Join(work, "vendor", "lib"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, "vendor", "lib", "a.txt"), []byte("cached"), 0o644); err != nil {
		t.Fatal(err)
	}

	var log bytes.Buffer
	cacheSave(cfg, job, work, &log)

	work2 := t.TempDir()
	cacheRestore(cfg, job, work2, &log)
	got, err := os.ReadFile(filepath.Join(work2, "vendor", "lib", "a.txt"))
	if err != nil {
		t.Fatalf("read restored: %v", err)
	}
	if string(got) != "cached" {
		t.Fatalf("restored = %q", got)
	}
}

func TestCacheKeyIsolation(t *testing.T) {
	SetCacheDir(t.TempDir())
	t.Cleanup(func() { SetCacheDir("") })

	job := RunJob{Owner: "a", Repo: "r", RunID: 1}
	work := t.TempDir()
	if err := os.MkdirAll(filepath.Join(work, "out"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, "out", "x"), []byte("A"), 0o644); err != nil {
		t.Fatal(err)
	}
	var log bytes.Buffer
	cacheSave(&Config{CacheKey: "a", CachePaths: []string{"out"}}, job, work, &log)

	work2 := t.TempDir()
	cacheRestore(&Config{CacheKey: "b", CachePaths: []string{"out"}}, job, work2, &log)
	if _, err := os.Stat(filepath.Join(work2, "out", "x")); !os.IsNotExist(err) {
		t.Fatal("cache key b should not see key a")
	}
}
