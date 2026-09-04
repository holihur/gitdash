package tests

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"gitdash/backend/internal/store"

	_ "modernc.org/sqlite"
)

func TestFreshSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.db")
	st, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}

	// 用户 + 会话
	u, err := st.CreateUser("alice", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.CreateSession("tok", u.ID); err != nil {
		t.Fatal(err)
	}
	username, err := st.GetSession("tok")
	if err != nil || username != "alice" {
		t.Fatalf("session = %q, %v", username, err)
	}
	if _, err := st.GetSession("bad"); err == nil {
		t.Fatal("unknown token accepted")
	}
	_ = st.DeleteSession("tok")
	if _, err := st.GetSession("tok"); err == nil {
		t.Fatal("deleted token accepted")
	}

	// 重复用户名
	if _, err := st.CreateUser("alice", "h2"); err != store.ErrExists {
		t.Fatalf("dup user = %v", err)
	}
}

func TestLegacySchemaReset(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	// 模拟 v0.1 旧 schema（无 owner / user_id）
	if _, err := db.Exec(`CREATE TABLE repos (
		id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL UNIQUE,
		description TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL);
		CREATE TABLE ssh_keys (
		id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL,
		public_key TEXT NOT NULL, fingerprint TEXT NOT NULL UNIQUE, created_at TEXT NOT NULL);
		INSERT INTO repos (name, created_at) VALUES ('old', 'now');`); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	// 打开时自动重置旧表并创建新 schema
	if _, err := store.Open(path); err != nil {
		t.Fatal(err)
	}
	db2, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db2.Close()

	if !tableHasColumn(t, db2, "repos", "owner") {
		t.Fatal("repos missing owner column")
	}
	if !tableHasColumn(t, db2, "ssh_keys", "user_id") {
		t.Fatal("ssh_keys missing user_id column")
	}
	var n int
	if err := db2.QueryRow(`SELECT count(*) FROM repos`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("legacy rows not reset: n=%d err=%v", n, err)
	}
	// 新 schema 可写
	if _, err := db2.Exec(`INSERT INTO repos (owner, name, created_at) VALUES ('a', 'x', 'now')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db2.Exec(`INSERT INTO users (username, password_hash, created_at) VALUES ('a', 'h', 'now')`); err != nil {
		t.Fatal(err)
	}
}

func tableHasColumn(t *testing.T, db *sql.DB, table, column string) bool {
	t.Helper()
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notNull, pk int
		var name, typ string
		var dflt any
		if err := rows.Scan(&cid, &name, &typ, &notNull, &dflt, &pk); err != nil {
			continue
		}
		if name == column {
			return true
		}
	}
	return false
}

func TestStoreDataIsolation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.db")
	st, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	st.CreateUser("alice", "h")
	st.CreateUser("bob", "h")

	st.CreateRepo("alice", "demo", "")
	st.CreateRepo("bob", "demo", "")

	if _, err := st.CreateRepo("alice", "demo", ""); err != store.ErrExists {
		t.Fatalf("dup repo = %v", err)
	}

	repos, err := st.ListRepos("alice")
	if err != nil || len(repos) != 1 || repos[0].Owner != "alice" {
		t.Fatalf("alice repos = %+v, %v", repos, err)
	}
	if _, err := st.GetRepo("bob", "demo"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetRepo("carol", "demo"); err != store.ErrNotFound {
		t.Fatalf("unknown owner = %v", err)
	}

	// key 按用户隔离
	st.CreateKey("alice", "k", "pub", "fp1")
	st.CreateKey("bob", "k2", "pub2", "fp2")
	if keys, _ := st.ListKeys("alice"); len(keys) != 1 {
		t.Fatal("alice should have 1 key")
	}
	pubKeys, err := st.PublicKeys()
	if err != nil || len(pubKeys) != 2 {
		t.Fatalf("public keys = %+v, %v", pubKeys, err)
	}
	bobKeyID := mustKeyID(t, st, "bob")
	if err := st.DeleteKey("bob", bobKeyID); err != nil {
		t.Fatal(err)
	}
	// alice 无法删除 bob 的 key（已删，且 owner 不符时也返回 ErrNotFound）
	if err := st.DeleteKey("alice", bobKeyID); err != store.ErrNotFound {
		t.Fatalf("alice deleted foreign key: %v", err)
	}
	_ = os.Remove(path)
}

func mustKeyID(t *testing.T, st *store.Store, username string) int64 {
	t.Helper()
	keys, err := st.ListKeys(username)
	if err != nil || len(keys) == 0 {
		t.Fatalf("no keys for %s", username)
	}
	return keys[0].ID
}
