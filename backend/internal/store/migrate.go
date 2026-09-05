package store

import "database/sql"

func (s *Store) migrate() error {
	// 旧版（无 owner 字段）schema 直接重置，v0.2 起仓库归属用户
	if legacy := s.hasLegacyRepos(); legacy {
		if _, err := s.db.Exec(`DROP TABLE IF EXISTS repos; DROP TABLE IF EXISTS ssh_keys;`); err != nil {
			return err
		}
	}
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS users (
	id            INTEGER PRIMARY KEY AUTOINCREMENT,
	username      TEXT NOT NULL UNIQUE,
	password_hash TEXT NOT NULL,
	created_at    TEXT NOT NULL,
	mfa_secret    TEXT NOT NULL DEFAULT '',
	mfa_enabled   INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS sessions (
	token      TEXT PRIMARY KEY,
	user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	created_at TEXT NOT NULL,
	expires_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS repos (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	owner       TEXT NOT NULL,
	name        TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	private     INTEGER NOT NULL DEFAULT 1,
	created_at  TEXT NOT NULL,
	UNIQUE(owner, name)
);
CREATE TABLE IF NOT EXISTS ssh_keys (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	name        TEXT NOT NULL,
	public_key  TEXT NOT NULL,
	fingerprint TEXT NOT NULL UNIQUE,
	created_at  TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS issues (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	owner      TEXT NOT NULL,
	repo       TEXT NOT NULL,
	number     INTEGER NOT NULL,
	title      TEXT NOT NULL,
	body       TEXT NOT NULL DEFAULT '',
	state      TEXT NOT NULL DEFAULT 'open',
	author     TEXT NOT NULL,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	closed_at  TEXT,
	milestone_id INTEGER,
	UNIQUE(owner, repo, number)
);
CREATE TABLE IF NOT EXISTS repo_labels (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	owner      TEXT NOT NULL,
	repo       TEXT NOT NULL,
	name       TEXT NOT NULL,
	color      TEXT NOT NULL DEFAULT '0366d6',
	created_at TEXT NOT NULL,
	UNIQUE(owner, repo, name)
);
CREATE TABLE IF NOT EXISTS issue_labels (
	issue_id INTEGER NOT NULL REFERENCES issues(id) ON DELETE CASCADE,
	label_id INTEGER NOT NULL REFERENCES repo_labels(id) ON DELETE CASCADE,
	PRIMARY KEY (issue_id, label_id)
);
CREATE TABLE IF NOT EXISTS milestones (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	owner       TEXT NOT NULL,
	repo        TEXT NOT NULL,
	title       TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	state       TEXT NOT NULL DEFAULT 'open',
	created_at  TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS repo_collabs (
	owner      TEXT NOT NULL,
	repo       TEXT NOT NULL,
	username   TEXT NOT NULL,
	permission TEXT NOT NULL DEFAULT 'read',
	created_at TEXT NOT NULL,
	PRIMARY KEY (owner, repo, username)
);
CREATE TABLE IF NOT EXISTS webhooks (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	owner      TEXT NOT NULL,
	repo       TEXT NOT NULL,
	url        TEXT NOT NULL,
	secret     TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	UNIQUE(owner, repo, url)
);
CREATE TABLE IF NOT EXISTS admin_users (
	id            INTEGER PRIMARY KEY AUTOINCREMENT,
	username      TEXT NOT NULL UNIQUE,
	password_hash TEXT NOT NULL,
	created_at    TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS admin_sessions (
	token      TEXT PRIMARY KEY,
	admin_id   INTEGER NOT NULL REFERENCES admin_users(id) ON DELETE CASCADE,
	created_at TEXT NOT NULL,
	expires_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS settings (
	key   TEXT PRIMARY KEY,
	value TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS user_oauth (
	provider   TEXT NOT NULL,
	external_id TEXT NOT NULL,
	user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	created_at TEXT NOT NULL,
	PRIMARY KEY (provider, external_id)
);
CREATE TABLE IF NOT EXISTS orgs (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	name       TEXT NOT NULL UNIQUE,
	display    TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS org_members (
	org      TEXT NOT NULL,
	username TEXT NOT NULL,
	role     TEXT NOT NULL DEFAULT 'member',
	created_at TEXT NOT NULL,
	PRIMARY KEY (org, username)
);
CREATE TABLE IF NOT EXISTS repo_stars (
	username   TEXT NOT NULL,
	owner      TEXT NOT NULL,
	repo       TEXT NOT NULL,
	created_at TEXT NOT NULL,
	PRIMARY KEY (username, owner, repo)
);
CREATE TABLE IF NOT EXISTS repo_watches (
	username   TEXT NOT NULL,
	owner      TEXT NOT NULL,
	repo       TEXT NOT NULL,
	created_at TEXT NOT NULL,
	PRIMARY KEY (username, owner, repo)
);
CREATE TABLE IF NOT EXISTS notifications (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	username   TEXT NOT NULL,
	kind       TEXT NOT NULL,
	action     TEXT NOT NULL,
	owner      TEXT NOT NULL,
	repo       TEXT NOT NULL,
	number     INTEGER NOT NULL,
	title      TEXT NOT NULL DEFAULT '',
	actor      TEXT NOT NULL DEFAULT '',
	read       INTEGER NOT NULL DEFAULT 0,
	created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_notifications_user ON notifications(username, read);
CREATE TABLE IF NOT EXISTS repo_forks (
	owner        TEXT NOT NULL,
	repo         TEXT NOT NULL,
	source_owner TEXT NOT NULL,
	source_repo  TEXT NOT NULL,
	created_at   TEXT NOT NULL,
	PRIMARY KEY (owner, repo)
);
CREATE TABLE IF NOT EXISTS repo_imports (
	owner      TEXT NOT NULL,
	repo       TEXT NOT NULL,
	source_url TEXT NOT NULL,
	created_at TEXT NOT NULL,
	PRIMARY KEY (owner, repo)
);
CREATE TABLE IF NOT EXISTS repo_mirrors (
	owner       TEXT NOT NULL,
	repo        TEXT NOT NULL,
	url         TEXT NOT NULL,
	private_key TEXT NOT NULL DEFAULT '',
	created_at  TEXT NOT NULL,
	PRIMARY KEY (owner, repo)
);
CREATE TABLE IF NOT EXISTS gpg_keys (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	fingerprint TEXT NOT NULL UNIQUE,
	armor       TEXT NOT NULL,
	created_at  TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS pull_requests (
	id            INTEGER PRIMARY KEY AUTOINCREMENT,
	owner         TEXT NOT NULL,
	repo          TEXT NOT NULL,
	number        INTEGER NOT NULL,
	title         TEXT NOT NULL,
	body          TEXT NOT NULL DEFAULT '',
	source_branch TEXT NOT NULL,
	target_branch TEXT NOT NULL,
	base_sha      TEXT NOT NULL DEFAULT '',
	head_sha      TEXT NOT NULL DEFAULT '',
	state         TEXT NOT NULL DEFAULT 'open',
	author        TEXT NOT NULL,
	created_at    TEXT NOT NULL,
	updated_at    TEXT NOT NULL,
	merged_at     TEXT,
	merged_by     TEXT NOT NULL DEFAULT '',
	UNIQUE(owner, repo, number)
);
CREATE TABLE IF NOT EXISTS repo_pipelines (
	owner      TEXT NOT NULL,
	repo       TEXT NOT NULL,
	enabled    INTEGER NOT NULL DEFAULT 0,
	created_at TEXT NOT NULL,
	PRIMARY KEY (owner, repo)
);
CREATE TABLE IF NOT EXISTS pipeline_runs (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	owner       TEXT NOT NULL,
	repo        TEXT NOT NULL,
	sha         TEXT NOT NULL DEFAULT '',
	ref         TEXT NOT NULL DEFAULT '',
	trigger_by  TEXT NOT NULL DEFAULT '',
	status      TEXT NOT NULL DEFAULT 'pending',
	steps_total INTEGER NOT NULL DEFAULT 0,
	steps_done  INTEGER NOT NULL DEFAULT 0,
	error       TEXT NOT NULL DEFAULT '',
	created_at  TEXT NOT NULL,
	finished_at TEXT
);
CREATE INDEX IF NOT EXISTS idx_pipeline_runs_repo ON pipeline_runs(owner, repo, id);
CREATE INDEX IF NOT EXISTS idx_repo_stars_owner_repo ON repo_stars(owner, repo);
CREATE INDEX IF NOT EXISTS idx_repo_watches_owner_repo ON repo_watches(owner, repo);
CREATE INDEX IF NOT EXISTS idx_issues_owner_repo ON issues(owner, repo, state, number);
CREATE INDEX IF NOT EXISTS idx_pulls_owner_repo ON pull_requests(owner, repo, state, number);`)
	if err != nil {
		return err
	}
	// 存量库增量迁移：为 users 补 MFA 列
	if err := ensureColumn(s.db, "users", "mfa_secret", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := ensureColumn(s.db, "users", "mfa_enabled", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := ensureColumn(s.db, "issues", "milestone_id", "INTEGER"); err != nil {
		return err
	}
	if err := ensureColumn(s.db, "webhooks", "secret", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := ensureColumn(s.db, "repos", "private", "INTEGER NOT NULL DEFAULT 1"); err != nil {
		return err
	}
	return nil
}

// ensureColumn 若表缺少指定列则 ALTER TABLE 添加（SQLite 不支持 IF NOT EXISTS on ADD COLUMN）。
func ensureColumn(db *sql.DB, table, column, ddl string) error {
	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var cid, notNull, pk int
		var name, typ string
		var dflt any
		if err := rows.Scan(&cid, &name, &typ, &notNull, &dflt, &pk); err != nil {
			return err
		}
		if name == column {
			return nil
		}
	}
	_, err = db.Exec(`ALTER TABLE ` + table + ` ADD COLUMN ` + column + ` ` + ddl)
	return err
}

func (s *Store) hasLegacyRepos() bool {
	rows, err := s.db.Query(`PRAGMA table_info(repos)`)
	if err != nil {
		return false
	}
	defer func() { _ = rows.Close() }()
	var names []string
	for rows.Next() {
		var cid, notNull, pk int
		var name, typ string
		var dflt any
		if err := rows.Scan(&cid, &name, &typ, &notNull, &dflt, &pk); err != nil {
			continue
		}
		names = append(names, name)
	}
	return len(names) > 0 && !hasStr(names, "owner")
}

func hasStr(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
