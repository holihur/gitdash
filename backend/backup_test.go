package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

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

	// dry-run 校验：归档可解、路径安全、能识别 db/repos
	m, err := verifyArchive(out)
	if err != nil {
		t.Fatalf("verifyArchive: %v", err)
	}
	if !m.HasDB || m.Files == 0 {
		t.Fatalf("manifest missing db/files: %+v", m)
	}
}

func TestPruneBackupsKeepsNewest(t *testing.T) {
	dir := t.TempDir()
	base := time.Now().Add(-time.Hour)
	names := []string{
		"gitdash-backup-20260101-000000.tar.gz",
		"gitdash-backup-20260102-000000.tar.gz",
		"gitdash-backup-20260103-000000.tar.gz",
		"unrelated.tar.gz",
	}
	for i, n := range names {
		p := filepath.Join(dir, n)
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		// 让文件名序号与 mtime 顺序一致
		if err := os.Chtimes(p, base.Add(time.Duration(i)*time.Minute), base.Add(time.Duration(i)*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	removed, err := pruneBackups(dir, 2)
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if removed != 1 {
		t.Fatalf("removed=%d, want 1", removed)
	}
	files, err := backupFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("kept %d backups, want 2", len(files))
	}
	// 最新的两份应保留，最旧的被删；无关文件不受影响
	if _, err := os.Stat(filepath.Join(dir, names[0])); !os.IsNotExist(err) {
		t.Fatalf("oldest backup should be removed, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, names[2])); err != nil {
		t.Fatalf("newest backup should remain: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, names[3])); err != nil {
		t.Fatalf("unrelated file should remain: %v", err)
	}
}

func TestVerifyArchiveRejectsGarbage(t *testing.T) {
	p := filepath.Join(t.TempDir(), "bad.tar.gz")
	if err := os.WriteFile(p, []byte("not gzip"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := verifyArchive(p); err == nil {
		t.Fatal("expected error for non-gzip archive")
	}
}
