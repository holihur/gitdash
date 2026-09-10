package main

import (
	"os"
	"path/filepath"
	"testing"

	"gitdash/backend/internal/store"
)

func TestBackupRestoreRoundTrip(t *testing.T) {
	t.Setenv("GITDASH_DB", "")
	src := t.TempDir()
	if err := os.MkdirAll(filepath.Join(src, "repos", "alice"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "repos", "alice", "demo.git"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if st, err := store.Open(filepath.Join(src, "gitdash.db")); err != nil {
		t.Fatal(err)
	} else {
		_ = st
	}
	out := filepath.Join(t.TempDir(), "backup.tar.gz")
	if err := writeBackup(src, out); err != nil {
		t.Fatalf("backup: %v", err)
	}

	dst := t.TempDir()
	if err := restoreArchive(out, dst, false); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, "gitdash.db")); err != nil {
		t.Fatalf("db missing after restore: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, "repos", "alice", "demo.git")); err != nil {
		t.Fatalf("repo missing after restore: %v", err)
	}

	// 路径穿越与绝对路径必须被拒绝
	if _, err := safeArchivePath(dst, "../evil"); err == nil {
		t.Fatal("expected traversal rejection")
	}
	if _, err := safeArchivePath(dst, "/abs"); err == nil {
		t.Fatal("expected absolute path rejection")
	}
}
