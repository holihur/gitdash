package api

import (
	"path/filepath"
	"testing"

	"gitdash/backend/internal/store"
)

// TestCanReadPackageFailClosedWhenLinkedRepoMissing 覆盖安全审计 F-24：包关联的
// 仓库已删除/不可读时，即使是 anonymous 可见性也必须按最严格一档处理。
func TestCanReadPackageFailClosedWhenLinkedRepoMissing(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	if _, err := st.CreateUser("alice", "hash"); err != nil {
		t.Fatal(err)
	}
	pkg := &store.Package{
		Owner:      "alice",
		Repo:       "ghost", // 该仓库并不存在
		Type:       "npm",
		Name:       "pkg",
		Version:    "1.0.0",
		Filename:   "pkg-1.0.0.tgz",
		Visibility: "anonymous",
	}
	if err := st.CreatePackage(pkg, []byte("x")); err != nil {
		t.Fatal(err)
	}
	a := New(st, "test")
	if a.canReadPackage("alice", "npm", "pkg", "") {
		t.Fatal("anonymous read must be denied when the linked repo is missing")
	}
	if !a.canReadPackage("alice", "npm", "pkg", "alice") {
		t.Fatal("owner should still read the package")
	}
}
