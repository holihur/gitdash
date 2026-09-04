package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

var (
	ErrNotFound = errors.New("not found")
	ErrExists   = errors.New("already exists")
)

type User struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	CreatedAt string `json:"created_at"`
}

type UserAuth struct {
	ID           int64
	Username     string
	PasswordHash string
	CreatedAt    string
	MFASecret    string // 空 = 未启用/无待激活 secret
	MFAEnabled   bool
}

type Repo struct {
	ID          int64  `json:"id"`
	Owner       string `json:"owner"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Private     bool   `json:"private"`
	CreatedAt   string `json:"created_at"`
	// Role 仅用于“可访问仓库列表”（owner / read / write），普通查询为空
	Role string `json:"role,omitempty"`
	// 展示字段（由 API 层填充，store 查询不扫描）
	Stars     int    `json:"stars"`
	Starred   bool   `json:"starred"`
	ForkOwner string `json:"fork_owner,omitempty"`
	ForkRepo  string `json:"fork_repo,omitempty"`
}

type SSHKey struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	PublicKey   string `json:"public_key"`
	Fingerprint string `json:"fingerprint"`
	CreatedAt   string `json:"created_at"`
}

type Issue struct {
	ID        int64   `json:"id"`
	Owner     string  `json:"-"`
	Repo      string  `json:"-"`
	Number    int64   `json:"number"`
	Title     string  `json:"title"`
	Body      string  `json:"body"`
	State     string  `json:"state"` // "open" | "closed"
	Author    string  `json:"author"`
	CreatedAt string  `json:"created_at"`
	UpdatedAt string  `json:"updated_at"`
	ClosedAt  *string `json:"closed_at"`
}

// Collab 仓库协作者（read=可克隆/浏览，write=可 push）
type Collab struct {
	Owner      string `json:"owner"`
	Repo       string `json:"repo"`
	Username   string `json:"username"`
	Permission string `json:"permission"` // "read" | "write"
	CreatedAt  string `json:"created_at"`
}

// Webhook push 事件回调配置

// PullRequest 仓库内的拉取请求（同仓库分支合并，MVP：仅支持 fast-forward 合并）
type PullRequest struct {
	ID           int64   `json:"id"`
	Owner        string  `json:"-"`
	Repo         string  `json:"-"`
	Number       int64   `json:"number"`
	Title        string  `json:"title"`
	Body         string  `json:"body"`
	SourceBranch string  `json:"source_branch"`
	TargetBranch string  `json:"target_branch"`
	BaseSHA      string  `json:"base_sha"`
	HeadSHA      string  `json:"head_sha"`
	State        string  `json:"state"` // open | merged | closed
	Author       string  `json:"author"`
	CreatedAt    string  `json:"created_at"`
	UpdatedAt    string  `json:"updated_at"`
	MergedAt     *string `json:"merged_at"`
	MergedBy     string  `json:"merged_by"`
}

// Org 组织（命名空间）：成员可把仓库 owner 设为组织名。
type Org struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Display   string `json:"display"`
	CreatedAt string `json:"created_at"`
}

// OrgMember 组织成员（owner 拥有全部管理权，member 可写组织仓库）
type OrgMember struct {
	Org      string `json:"org"`
	Username string `json:"username"`
	Role     string `json:"role"`
}

// Label 仓库 issue 标签
type Label struct {
	ID        int64  `json:"id"`
	Owner     string `json:"owner"`
	Repo      string `json:"repo"`
	Name      string `json:"name"`
	Color     string `json:"color"`
	CreatedAt string `json:"created_at"`
}

// Milestone 仓库里程碑
type Milestone struct {
	ID           int64  `json:"id"`
	Owner        string `json:"owner"`
	Repo         string `json:"repo"`
	Title        string `json:"title"`
	Description  string `json:"description"`
	State        string `json:"state"` // open | closed
	OpenIssues   int    `json:"open_issues"`
	ClosedIssues int    `json:"closed_issues"`
	CreatedAt    string `json:"created_at"`
}

type GPGKey struct {
	ID          int64  `json:"id"`
	Fingerprint string `json:"fingerprint"`
	CreatedAt   string `json:"created_at"`
}

type GPGKeyAuth struct {
	Username    string
	Fingerprint string
	Armor       string
}

type Webhook struct {
	ID        int64  `json:"id"`
	Owner     string `json:"owner"`
	Repo      string `json:"repo"`
	URL       string `json:"url"`
	Secret    string `json:"-"` // 签名密钥，不回传
	CreatedAt string `json:"created_at"`
}

// PublicKeyAuth 用于 SSH 鉴权：公钥行 + 所属用户
type PublicKeyAuth struct {
	UserID   int64
	Username string
	Line     string
}

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	return s, nil
}

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
CREATE TABLE IF NOT EXISTS repo_forks (
	owner        TEXT NOT NULL,
	repo         TEXT NOT NULL,
	source_owner TEXT NOT NULL,
	source_repo  TEXT NOT NULL,
	created_at   TEXT NOT NULL,
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
);`)
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
	defer rows.Close()
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
	defer rows.Close()
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

func now() string { return time.Now().UTC().Format(time.RFC3339) }

func isUniqueErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}

// ---- users & sessions ----

func (s *Store) CreateUser(username, passwordHash string) (User, error) {
	u := User{Username: username, CreatedAt: now()}
	res, err := s.db.Exec(`INSERT INTO users (username, password_hash, created_at) VALUES (?, ?, ?)`,
		u.Username, passwordHash, u.CreatedAt)
	if err != nil {
		if isUniqueErr(err) {
			return u, ErrExists
		}
		return u, err
	}
	u.ID, _ = res.LastInsertId()
	return u, nil
}

func (s *Store) GetByUsername(username string) (UserAuth, error) {
	var ua UserAuth
	var mfa int
	err := s.db.QueryRow(`SELECT id, username, password_hash, created_at, COALESCE(mfa_secret,''), mfa_enabled
		FROM users WHERE username = ?`, username).
		Scan(&ua.ID, &ua.Username, &ua.PasswordHash, &ua.CreatedAt, &ua.MFASecret, &mfa)
	ua.MFAEnabled = mfa != 0
	if errors.Is(err, sql.ErrNoRows) {
		return ua, ErrNotFound
	}
	return ua, err
}

const SessionTTL = 7 * 24 * time.Hour

func (s *Store) CreateSession(token string, userID int64) error {
	_, err := s.db.Exec(`INSERT INTO sessions (token, user_id, created_at, expires_at) VALUES (?, ?, ?, ?)`,
		token, userID, now(), time.Now().Add(SessionTTL).UTC().Format(time.RFC3339))
	return err
}

func (s *Store) GetSession(token string) (string, error) {
	var username string
	err := s.db.QueryRow(
		`SELECT u.username FROM sessions s JOIN users u ON u.id = s.user_id
		 WHERE s.token = ? AND s.expires_at > ?`, token, now()).
		Scan(&username)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return username, err
}

func (s *Store) DeleteSession(token string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE token = ?`, token)
	return err
}

// DeleteSessionsExcept 撤销用户除 keepToken 外的全部会话（改密/安全操作后调用）。
func (s *Store) DeleteSessionsExcept(username, keepToken string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE user_id = (SELECT id FROM users WHERE username = ?) AND token <> ?`,
		username, keepToken)
	return err
}

// ---- user profile & mfa ----

func (s *Store) UpdatePassword(username, passwordHash string) error {
	res, err := s.db.Exec(`UPDATE users SET password_hash = ? WHERE username = ?`, passwordHash, username)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetMFASecret 写入（或覆盖）MFA secret；enable=false 时保留 secret 但标记未激活。
func (s *Store) SetMFASecret(username, secret string, enabled bool) error {
	en := 0
	if enabled {
		en = 1
	}
	res, err := s.db.Exec(`UPDATE users SET mfa_secret = ?, mfa_enabled = ? WHERE username = ?`, secret, en, username)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ClearMFA(username string) error {
	_, err := s.db.Exec(`UPDATE users SET mfa_secret = '', mfa_enabled = 0 WHERE username = ?`, username)
	return err
}

// ---- repos ----

func (s *Store) CreateRepo(owner, name, description string, private bool) (Repo, error) {
	r := Repo{Owner: owner, Name: name, Description: description, Private: private, CreatedAt: now()}
	pv := 1
	if !private {
		pv = 0
	}
	res, err := s.db.Exec(`INSERT INTO repos (owner, name, description, private, created_at) VALUES (?, ?, ?, ?, ?)`,
		r.Owner, r.Name, r.Description, pv, r.CreatedAt)
	if err != nil {
		if isUniqueErr(err) {
			return r, ErrExists
		}
		return r, err
	}
	r.ID, _ = res.LastInsertId()
	return r, nil
}

func (s *Store) ListRepos(owner string) ([]Repo, error) {
	rows, err := s.db.Query(`SELECT id, owner, name, description, private, created_at FROM repos WHERE owner = ? ORDER BY name`, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	repos := []Repo{}
	for rows.Next() {
		var r Repo
		if err := rows.Scan(&r.ID, &r.Owner, &r.Name, &r.Description, &r.Private, &r.CreatedAt); err != nil {
			return nil, err
		}
		repos = append(repos, r)
	}
	return repos, rows.Err()
}

// ExploreRepos 公开仓库（供发现页使用）。
func (s *Store) ExploreRepos() ([]Repo, error) {
	rows, err := s.db.Query(`SELECT id, owner, name, description, private, created_at
		FROM repos WHERE private = 0 ORDER BY id DESC LIMIT 100`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	repos := []Repo{}
	for rows.Next() {
		var r Repo
		if err := rows.Scan(&r.ID, &r.Owner, &r.Name, &r.Description, &r.Private, &r.CreatedAt); err != nil {
			return nil, err
		}
		repos = append(repos, r)
	}
	return repos, rows.Err()
}

// SetRepoPrivate 切换可见性（仅 owner 调用）。
func (s *Store) SetRepoPrivate(owner, name string, private bool) error {
	pv := 1
	if !private {
		pv = 0
	}
	res, err := s.db.Exec(`UPDATE repos SET private = ? WHERE owner = ? AND name = ?`, pv, owner, name)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) GetRepo(owner, name string) (Repo, error) {
	var r Repo
	err := s.db.QueryRow(`SELECT id, owner, name, description, private, created_at FROM repos WHERE owner = ? AND name = ?`, owner, name).
		Scan(&r.ID, &r.Owner, &r.Name, &r.Description, &r.Private, &r.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return r, ErrNotFound
	}
	return r, err
}

func (s *Store) DeleteRepo(owner, name string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM issues WHERE owner = ? AND repo = ?`, owner, name); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM repo_labels WHERE owner = ? AND repo = ?`, owner, name); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM milestones WHERE owner = ? AND repo = ?`, owner, name); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM repo_stars WHERE owner = ? AND repo = ?`, owner, name); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM repo_forks WHERE owner = ? AND repo = ?`, owner, name); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM repo_forks WHERE source_owner = ? AND source_repo = ?`, owner, name); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM repo_collabs WHERE owner = ? AND repo = ?`, owner, name); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM webhooks WHERE owner = ? AND repo = ?`, owner, name); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM pull_requests WHERE owner = ? AND repo = ?`, owner, name); err != nil {
		return err
	}
	res, err := tx.Exec(`DELETE FROM repos WHERE owner = ? AND name = ?`, owner, name)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return tx.Commit()
}

// ---- issues ----

func (s *Store) getIssue(owner, repo string, number int64) (Issue, error) {
	var it Issue
	var ca sql.NullString
	err := s.db.QueryRow(`SELECT id, owner, repo, number, title, body, state, author, created_at, updated_at, closed_at
		FROM issues WHERE owner = ? AND repo = ? AND number = ?`, owner, repo, number).
		Scan(&it.ID, &it.Owner, &it.Repo, &it.Number, &it.Title, &it.Body, &it.State, &it.Author,
			&it.CreatedAt, &it.UpdatedAt, &ca)
	if errors.Is(err, sql.ErrNoRows) {
		return it, ErrNotFound
	}
	if ca.Valid {
		v := ca.String
		it.ClosedAt = &v
	}
	return it, err
}

func (s *Store) CreateIssue(owner, repo, author, title, body string) (Issue, error) {
	var it Issue
	var err error
	now := now()
	// 号码在同一仓库内递增；并发冲突时重试（UNIQUE(owner, repo, number)）
	for attempt := 0; attempt < 5; attempt++ {
		res, e := s.db.Exec(`INSERT INTO issues (owner, repo, number, title, body, state, author, created_at, updated_at)
			VALUES (?, ?, (SELECT COALESCE(MAX(number), 0) + 1 FROM issues WHERE owner = ? AND repo = ?), ?, ?, 'open', ?, ?, ?)`,
			owner, repo, owner, repo, title, body, author, now, now)
		if e == nil {
			it.ID, _ = res.LastInsertId()
			it = Issue{ID: it.ID, Owner: owner, Repo: repo, Title: title, Body: body,
				State: "open", Author: author, CreatedAt: now, UpdatedAt: now}
			if err := s.db.QueryRow(`SELECT number FROM issues WHERE id = ?`, it.ID).Scan(&it.Number); err != nil {
				return it, err
			}
			return it, nil
		}
		if !isUniqueErr(e) {
			return it, e
		}
		err = e
	}
	return it, err
}

func (s *Store) ListIssues(owner, repo string) ([]Issue, error) {
	rows, err := s.db.Query(`SELECT id, owner, repo, number, title, body, state, author, created_at, updated_at, closed_at
		FROM issues WHERE owner = ? AND repo = ? ORDER BY (state = 'open') DESC, number DESC`, owner, repo)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	issues := []Issue{}
	for rows.Next() {
		var it Issue
		var ca sql.NullString
		if err := rows.Scan(&it.ID, &it.Owner, &it.Repo, &it.Number, &it.Title, &it.Body, &it.State,
			&it.Author, &it.CreatedAt, &it.UpdatedAt, &ca); err != nil {
			return nil, err
		}
		if ca.Valid {
			v := ca.String
			it.ClosedAt = &v
		}
		issues = append(issues, it)
	}
	return issues, rows.Err()
}

func (s *Store) SetIssueState(owner, repo string, number int64, state string) (Issue, error) {
	if state != "open" && state != "closed" {
		return Issue{}, errors.New("invalid state")
	}
	now := now()
	var closedAt any
	if state == "closed" {
		closedAt = now
	}
	res, err := s.db.Exec(`UPDATE issues SET state = ?, updated_at = ?, closed_at = ?
		WHERE owner = ? AND repo = ? AND number = ?`, state, now, closedAt, owner, repo, number)
	if err != nil {
		return Issue{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Issue{}, ErrNotFound
	}
	return s.getIssue(owner, repo, number)
}

// ---- collaborators ----

func (s *Store) OwnedByName(username, name string) (string, error) {
	var owner string
	err := s.db.QueryRow(`SELECT owner FROM repos WHERE owner = ? AND name = ?`, username, name).Scan(&owner)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return owner, err
}

// SharedByName 返回用户以协作者身份可访问的、指定名称的仓库 owner（同名多仓库取其一）。
func (s *Store) SharedByName(username, name string) (string, error) {
	var owner string
	err := s.db.QueryRow(`SELECT owner FROM repo_collabs WHERE username = ? AND repo = ? ORDER BY owner LIMIT 1`, username, name).Scan(&owner)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return owner, err
}

func (s *Store) CanRead(owner, repo, username string) bool {
	if owner == username {
		return true
	}
	if s.IsOrg(owner) {
		if s.OrgRole(owner, username) != "" {
			return true
		}
		// 组织仓库公开时也放行（读）
		if r, err := s.GetRepo(owner, repo); err == nil && !r.Private {
			return true
		}
	}
	var one int
	err := s.db.QueryRow(`SELECT 1 FROM repo_collabs WHERE owner = ? AND repo = ? AND username = ?`, owner, repo, username).Scan(&one)
	return err == nil
}

func (s *Store) CanWrite(owner, repo, username string) bool {
	if owner == username {
		return true
	}
	if s.IsOrg(owner) {
		role := s.OrgRole(owner, username)
		if role == "owner" || role == "member" {
			return true
		}
	}
	var one int
	err := s.db.QueryRow(`SELECT 1 FROM repo_collabs WHERE owner = ? AND repo = ? AND username = ? AND permission = 'write'`, owner, repo, username).Scan(&one)
	return err == nil
}

// IsRepoOwner owner 语义：用户本人，或该用户是仓库所属组织的 owner。
func (s *Store) IsRepoOwner(owner, username string) bool {
	if owner == username {
		return true
	}
	if s.IsOrg(owner) {
		return s.OrgRole(owner, username) == "owner"
	}
	return false
}

// QueryOrgRepos 组织的全部仓库。
func (s *Store) QueryOrgRepos(org string) ([]Repo, error) {
	rows, err := s.db.Query(`SELECT id, owner, name, description, private, created_at
		FROM repos WHERE owner = ? ORDER BY name`, org)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Repo{}
	for rows.Next() {
		var r Repo
		if err := rows.Scan(&r.ID, &r.Owner, &r.Name, &r.Description, &r.Private, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// AccessibleRepos 返回用户自己拥有的仓库 + 作为协作者可访问的仓库（带 role）。
func (s *Store) AccessibleRepos(username string) ([]Repo, error) {
	rows, err := s.db.Query(`
SELECT r.id, r.owner, r.name, r.description, r.private, r.created_at, 'owner' AS role
  FROM repos r WHERE r.owner = ?
UNION ALL
SELECT r.id, r.owner, r.name, r.description, r.private, r.created_at, c.permission AS role
  FROM repo_collabs c JOIN repos r ON r.owner = c.owner AND r.name = c.repo
 WHERE c.username = ?
ORDER BY owner, name`, username, username)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	repos := []Repo{}
	for rows.Next() {
		var r Repo
		if err := rows.Scan(&r.ID, &r.Owner, &r.Name, &r.Description, &r.Private, &r.CreatedAt, &r.Role); err != nil {
			return nil, err
		}
		repos = append(repos, r)
	}
	return repos, rows.Err()
}

func (s *Store) UpsertCollab(owner, repo, username, permission string) error {
	_, err := s.db.Exec(`INSERT INTO repo_collabs (owner, repo, username, permission, created_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(owner, repo, username) DO UPDATE SET permission = excluded.permission`,
		owner, repo, username, permission, now())
	return err
}

func (s *Store) ListCollabs(owner, repo string) ([]Collab, error) {
	rows, err := s.db.Query(`SELECT owner, repo, username, permission, created_at
		FROM repo_collabs WHERE owner = ? AND repo = ? ORDER BY username`, owner, repo)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	collabs := []Collab{}
	for rows.Next() {
		var c Collab
		if err := rows.Scan(&c.Owner, &c.Repo, &c.Username, &c.Permission, &c.CreatedAt); err != nil {
			return nil, err
		}
		collabs = append(collabs, c)
	}
	return collabs, rows.Err()
}

func (s *Store) RemoveCollab(owner, repo, username string) error {
	res, err := s.db.Exec(`DELETE FROM repo_collabs WHERE owner = ? AND repo = ? AND username = ?`, owner, repo, username)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- webhooks ----

func (s *Store) CreateWebhook(owner, repo, url, secret string) (Webhook, error) {
	w := Webhook{Owner: owner, Repo: repo, URL: url, Secret: secret, CreatedAt: now()}
	res, err := s.db.Exec(`INSERT INTO webhooks (owner, repo, url, secret, created_at) VALUES (?, ?, ?, ?, ?)`,
		owner, repo, url, secret, w.CreatedAt)
	if err != nil {
		if isUniqueErr(err) {
			return w, ErrExists
		}
		return w, err
	}
	w.ID, _ = res.LastInsertId()
	return w, nil
}

func (s *Store) ListWebhooks(owner, repo string) ([]Webhook, error) {
	rows, err := s.db.Query(`SELECT id, owner, repo, url, secret, created_at
		FROM webhooks WHERE owner = ? AND repo = ? ORDER BY id`, owner, repo)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ws := []Webhook{}
	for rows.Next() {
		var w Webhook
		if err := rows.Scan(&w.ID, &w.Owner, &w.Repo, &w.URL, &w.Secret, &w.CreatedAt); err != nil {
			return nil, err
		}
		ws = append(ws, w)
	}
	return ws, rows.Err()
}

func (s *Store) DeleteWebhook(owner, repo string, id int64) error {
	res, err := s.db.Exec(`DELETE FROM webhooks WHERE owner = ? AND repo = ? AND id = ?`, owner, repo, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- ssh keys ----

func (s *Store) UserID(username string) (int64, error) {
	var id int64
	err := s.db.QueryRow(`SELECT id FROM users WHERE username = ?`, username).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	return id, err
}

func (s *Store) CreateKey(username, name, publicKey, fingerprint string) (SSHKey, error) {
	k := SSHKey{Name: name, PublicKey: publicKey, Fingerprint: fingerprint, CreatedAt: now()}
	res, err := s.db.Exec(`INSERT INTO ssh_keys (user_id, name, public_key, fingerprint, created_at)
		VALUES ((SELECT id FROM users WHERE username = ?), ?, ?, ?, ?)`,
		username, k.Name, k.PublicKey, k.Fingerprint, k.CreatedAt)
	if err != nil {
		if isUniqueErr(err) {
			return k, ErrExists
		}
		return k, err
	}
	k.ID, _ = res.LastInsertId()
	return k, nil
}

func (s *Store) ListKeys(username string) ([]SSHKey, error) {
	rows, err := s.db.Query(`SELECT k.id, k.name, k.public_key, k.fingerprint, k.created_at
		FROM ssh_keys k JOIN users u ON u.id = k.user_id WHERE u.username = ? ORDER BY k.id DESC`, username)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	keys := []SSHKey{}
	for rows.Next() {
		var k SSHKey
		if err := rows.Scan(&k.ID, &k.Name, &k.PublicKey, &k.Fingerprint, &k.CreatedAt); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

func (s *Store) PublicKeys() ([]PublicKeyAuth, error) {
	rows, err := s.db.Query(`SELECT k.user_id, u.username, k.public_key FROM ssh_keys k JOIN users u ON u.id = k.user_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var keys []PublicKeyAuth
	for rows.Next() {
		var k PublicKeyAuth
		if err := rows.Scan(&k.UserID, &k.Username, &k.Line); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

func (s *Store) DeleteKey(username string, id int64) error {
	res, err := s.db.Exec(`DELETE FROM ssh_keys WHERE id = ? AND user_id = (SELECT id FROM users WHERE username = ?)`, id, username)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- gpg keys ----

func (s *Store) AddGPGKey(username, fingerprint, armor string) (GPGKey, error) {
	k := GPGKey{Fingerprint: fingerprint, CreatedAt: now()}
	res, err := s.db.Exec(`INSERT INTO gpg_keys (user_id, fingerprint, armor, created_at)
		VALUES ((SELECT id FROM users WHERE username = ?), ?, ?, ?)`,
		username, k.Fingerprint, armor, k.CreatedAt)
	if err != nil {
		if isUniqueErr(err) {
			return k, ErrExists
		}
		return k, err
	}
	k.ID, _ = res.LastInsertId()
	return k, nil
}

func (s *Store) ListGPGKeys(username string) ([]GPGKey, error) {
	rows, err := s.db.Query(`SELECT k.id, k.fingerprint, k.created_at
		FROM gpg_keys k JOIN users u ON u.id = k.user_id WHERE u.username = ? ORDER BY k.id`, username)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []GPGKey{}
	for rows.Next() {
		var k GPGKey
		if err := rows.Scan(&k.ID, &k.Fingerprint, &k.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

func (s *Store) DeleteGPGKey(username string, id int64) error {
	res, err := s.db.Exec(`DELETE FROM gpg_keys WHERE id = ? AND user_id = (SELECT id FROM users WHERE username = ?)`, id, username)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// AllGPGKeys 返回全部用户注册的公钥（供提交签名校验使用）。
func (s *Store) AllGPGKeys() ([]GPGKeyAuth, error) {
	rows, err := s.db.Query(`SELECT u.username, k.fingerprint, k.armor FROM gpg_keys k JOIN users u ON u.id = k.user_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []GPGKeyAuth{}
	for rows.Next() {
		var k GPGKeyAuth
		if err := rows.Scan(&k.Username, &k.Fingerprint, &k.Armor); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// ---- pull requests ----

func (s *Store) getPull(owner, repo string, number int64) (PullRequest, error) {
	var pr PullRequest
	var ma sql.NullString
	err := s.db.QueryRow(`SELECT id, owner, repo, number, title, body, source_branch, target_branch,
		base_sha, head_sha, state, author, created_at, updated_at, merged_at, merged_by
		FROM pull_requests WHERE owner = ? AND repo = ? AND number = ?`, owner, repo, number).
		Scan(&pr.ID, &pr.Owner, &pr.Repo, &pr.Number, &pr.Title, &pr.Body, &pr.SourceBranch,
			&pr.TargetBranch, &pr.BaseSHA, &pr.HeadSHA, &pr.State, &pr.Author, &pr.CreatedAt,
			&pr.UpdatedAt, &ma, &pr.MergedBy)
	if errors.Is(err, sql.ErrNoRows) {
		return pr, ErrNotFound
	}
	if ma.Valid {
		v := ma.String
		pr.MergedAt = &v
	}
	return pr, err
}

func (s *Store) CreatePull(owner, repo, author, title, body, source, target, baseSHA, headSHA string) (PullRequest, error) {
	var pr PullRequest
	now := now()
	var err error
	for attempt := 0; attempt < 5; attempt++ {
		res, e := s.db.Exec(`INSERT INTO pull_requests (owner, repo, number, title, body, source_branch,
			target_branch, base_sha, head_sha, state, author, created_at, updated_at)
			VALUES (?, ?, (SELECT COALESCE(MAX(number),0)+1 FROM pull_requests WHERE owner=? AND repo=?),
			?, ?, ?, ?, ?, ?, 'open', ?, ?, ?)`,
			owner, repo, owner, repo, title, body, source, target, baseSHA, headSHA, author, now, now)
		if e == nil {
			id, _ := res.LastInsertId()
			pr = PullRequest{ID: id, Owner: owner, Repo: repo, Title: title, Body: body,
				SourceBranch: source, TargetBranch: target, BaseSHA: baseSHA, HeadSHA: headSHA,
				State: "open", Author: author, CreatedAt: now, UpdatedAt: now}
			if err := s.db.QueryRow(`SELECT number FROM pull_requests WHERE id = ?`, id).Scan(&pr.Number); err != nil {
				return pr, err
			}
			return pr, nil
		}
		if !isUniqueErr(e) {
			return pr, e
		}
		err = e
	}
	return pr, err
}

// GetPullIssue 按仓库内序号取 issue（供 label/milestone 更新后回读）。
func (s *Store) GetPullIssue(owner, repo string, number int64) (Issue, error) {
	return s.getIssue(owner, repo, number)
}

func (s *Store) GetPull(owner, repo string, number int64) (PullRequest, error) {
	return s.getPull(owner, repo, number)
}

func (s *Store) ListPulls(owner, repo, state string) ([]PullRequest, error) {
	q := `SELECT id, owner, repo, number, title, body, source_branch, target_branch,
		base_sha, head_sha, state, author, created_at, updated_at, merged_at, merged_by
		FROM pull_requests WHERE owner = ? AND repo = ?`
	args := []any{owner, repo}
	if state != "" {
		q += ` AND state = ?`
		args = append(args, state)
	}
	q += ` ORDER BY (state = 'open') DESC, number DESC`
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PullRequest{}
	for rows.Next() {
		var pr PullRequest
		var ma sql.NullString
		if err := rows.Scan(&pr.ID, &pr.Owner, &pr.Repo, &pr.Number, &pr.Title, &pr.Body,
			&pr.SourceBranch, &pr.TargetBranch, &pr.BaseSHA, &pr.HeadSHA, &pr.State, &pr.Author,
			&pr.CreatedAt, &pr.UpdatedAt, &ma, &pr.MergedBy); err != nil {
			return nil, err
		}
		if ma.Valid {
			v := ma.String
			pr.MergedAt = &v
		}
		out = append(out, pr)
	}
	return out, rows.Err()
}

// SetPullState 关闭/重开（不改变 merged 状态）。
func (s *Store) SetPullState(owner, repo string, number int64, state string) (PullRequest, error) {
	res, err := s.db.Exec(`UPDATE pull_requests SET state = ?, updated_at = ? WHERE owner = ? AND repo = ? AND number = ?`,
		state, now(), owner, repo, number)
	if err != nil {
		return PullRequest{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return PullRequest{}, ErrNotFound
	}
	return s.getPull(owner, repo, number)
}

// MarkPullMerged 记录合并结果（fast-forward 后 target 指向 headSHA）。
func (s *Store) MarkPullMerged(owner, repo string, number int64, headSHA, mergedBy string) (PullRequest, error) {
	res, err := s.db.Exec(`UPDATE pull_requests SET state = 'merged', head_sha = ?, merged_by = ?, merged_at = ?, updated_at = ?
		WHERE owner = ? AND repo = ? AND number = ?`, headSHA, mergedBy, now(), now(), owner, repo, number)
	if err != nil {
		return PullRequest{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return PullRequest{}, ErrNotFound
	}
	return s.getPull(owner, repo, number)
}

// ---- issue labels & milestones ----

func (s *Store) CreateLabel(owner, repo, name, color string) (Label, error) {
	l := Label{Owner: owner, Repo: repo, Name: name, Color: color, CreatedAt: now()}
	res, err := s.db.Exec(`INSERT INTO repo_labels (owner, repo, name, color, created_at) VALUES (?,?,?,?,?)`,
		owner, repo, name, color, l.CreatedAt)
	if err != nil {
		if isUniqueErr(err) {
			return l, ErrExists
		}
		return l, err
	}
	l.ID, _ = res.LastInsertId()
	return l, nil
}

func (s *Store) ListLabels(owner, repo string) ([]Label, error) {
	rows, err := s.db.Query(`SELECT id, owner, repo, name, color, created_at
		FROM repo_labels WHERE owner = ? AND repo = ? ORDER BY name`, owner, repo)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Label{}
	for rows.Next() {
		var l Label
		if err := rows.Scan(&l.ID, &l.Owner, &l.Repo, &l.Name, &l.Color, &l.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (s *Store) UpdateLabel(owner, repo string, id int64, name, color string) (Label, error) {
	res, err := s.db.Exec(`UPDATE repo_labels SET name = ?, color = ? WHERE id = ? AND owner = ? AND repo = ?`,
		name, color, id, owner, repo)
	if err != nil {
		return Label{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Label{}, ErrNotFound
	}
	rows, err := s.db.Query(`SELECT id, owner, repo, name, color, created_at FROM repo_labels WHERE id = ?`, id)
	if err != nil {
		return Label{}, err
	}
	defer rows.Close()
	var l Label
	if rows.Next() {
		_ = rows.Scan(&l.ID, &l.Owner, &l.Repo, &l.Name, &l.Color, &l.CreatedAt)
	}
	return l, rows.Err()
}

func (s *Store) DeleteLabel(owner, repo string, id int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM issue_labels WHERE label_id = ? AND issue_id IN
		(SELECT id FROM issues WHERE owner = ? AND repo = ?)`, id, owner, repo); err != nil {
		return err
	}
	res, err := tx.Exec(`DELETE FROM repo_labels WHERE id = ? AND owner = ? AND repo = ?`, id, owner, repo)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return tx.Commit()
}

// SetIssueLabels 全量替换 issue 标签（校验标签属于该仓库）。
func (s *Store) SetIssueLabels(owner, repo string, number int64, labelIDs []int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var issueID int64
	err = tx.QueryRow(`SELECT id FROM issues WHERE owner = ? AND repo = ? AND number = ?`, owner, repo, number).Scan(&issueID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	for _, id := range labelIDs {
		var one int
		if err := tx.QueryRow(`SELECT 1 FROM repo_labels WHERE id = ? AND owner = ? AND repo = ?`, id, owner, repo).Scan(&one); err != nil {
			return fmt.Errorf("label %d does not belong to this repository", id)
		}
	}
	if _, err := tx.Exec(`DELETE FROM issue_labels WHERE issue_id = ?`, issueID); err != nil {
		return err
	}
	for _, id := range labelIDs {
		if _, err := tx.Exec(`INSERT INTO issue_labels (issue_id, label_id) VALUES (?, ?)`, issueID, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// IssueLabels 返回若干 issue（number）的标签映射。
func (s *Store) IssueLabels(owner, repo string, numbers []int64) (map[int64][]Label, error) {
	out := map[int64][]Label{}
	if len(numbers) == 0 {
		return out, nil
	}
	rows, err := s.db.Query(`SELECT i.number, l.id, l.name, l.color FROM issue_labels il
		JOIN issues i ON i.id = il.issue_id
		JOIN repo_labels l ON l.id = il.label_id
		WHERE i.owner = ? AND i.repo = ?`, owner, repo)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var num, id int64
		var name, color string
		if err := rows.Scan(&num, &id, &name, &color); err != nil {
			return nil, err
		}
		out[num] = append(out[num], Label{ID: id, Name: name, Color: color})
	}
	return out, rows.Err()
}

func (s *Store) CreateMilestone(owner, repo, title, description string) (Milestone, error) {
	m := Milestone{Owner: owner, Repo: repo, Title: title, Description: description, State: "open", CreatedAt: now()}
	res, err := s.db.Exec(`INSERT INTO milestones (owner, repo, title, description, state, created_at) VALUES (?,?,?,?,'open',?)`,
		owner, repo, title, description, m.CreatedAt)
	if err != nil {
		return m, err
	}
	m.ID, _ = res.LastInsertId()
	return m, nil
}

func (s *Store) ListMilestones(owner, repo string) ([]Milestone, error) {
	rows, err := s.db.Query(`SELECT m.id, m.owner, m.repo, m.title, m.description, m.state, m.created_at,
		COALESCE(SUM(CASE WHEN i.state = 'open' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN i.state = 'closed' THEN 1 ELSE 0 END), 0)
		FROM milestones m LEFT JOIN issues i ON i.milestone_id = m.id AND i.owner = m.owner AND i.repo = m.repo
		WHERE m.owner = ? AND m.repo = ? GROUP BY m.id ORDER BY m.title`, owner, repo)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Milestone{}
	for rows.Next() {
		var m Milestone
		if err := rows.Scan(&m.ID, &m.Owner, &m.Repo, &m.Title, &m.Description, &m.State, &m.CreatedAt,
			&m.OpenIssues, &m.ClosedIssues); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) UpdateMilestone(owner, repo string, id int64, title, description, state string) (Milestone, error) {
	if title != "" {
		if _, err := s.db.Exec(`UPDATE milestones SET title = ? WHERE id = ? AND owner = ? AND repo = ?`,
			title, id, owner, repo); err != nil {
			return Milestone{}, err
		}
	}
	if description != "" {
		if _, err := s.db.Exec(`UPDATE milestones SET description = ? WHERE id = ? AND owner = ? AND repo = ?`,
			description, id, owner, repo); err != nil {
			return Milestone{}, err
		}
	}
	if state != "" {
		if _, err := s.db.Exec(`UPDATE milestones SET state = ? WHERE id = ? AND owner = ? AND repo = ?`,
			state, id, owner, repo); err != nil {
			return Milestone{}, err
		}
	}
	res, err := s.db.Exec(`UPDATE milestones SET title = title WHERE id = ? AND owner = ? AND repo = ?`, id, owner, repo)
	if err != nil {
		return Milestone{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 && title == "" && description == "" && state == "" {
		return Milestone{}, ErrNotFound
	}
	// 重新读取
	list, err := s.ListMilestones(owner, repo)
	if err != nil {
		return Milestone{}, err
	}
	for _, m := range list {
		if m.ID == id {
			return m, nil
		}
	}
	return Milestone{}, ErrNotFound
}

func (s *Store) DeleteMilestone(owner, repo string, id int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE issues SET milestone_id = NULL WHERE owner = ? AND repo = ? AND milestone_id = ?`,
		owner, repo, id); err != nil {
		return err
	}
	res, err := tx.Exec(`DELETE FROM milestones WHERE id = ? AND owner = ? AND repo = ?`, id, owner, repo)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return tx.Commit()
}

// SetIssueMilestone 设置/清除 issue 里程碑（返回是否属于仓库校验）。
func (s *Store) SetIssueMilestone(owner, repo string, number, milestoneID int64) error {
	var issueID int64
	err := s.db.QueryRow(`SELECT id FROM issues WHERE owner = ? AND repo = ? AND number = ?`, owner, repo, number).Scan(&issueID)
	if err != nil {
		return ErrNotFound
	}
	if milestoneID == 0 {
		_, err := s.db.Exec(`UPDATE issues SET milestone_id = NULL WHERE id = ?`, issueID)
		return err
	}
	var one int
	if err := s.db.QueryRow(`SELECT 1 FROM milestones WHERE id = ? AND owner = ? AND repo = ?`, milestoneID, owner, repo).Scan(&one); err != nil {
		return fmt.Errorf("milestone does not belong to this repository")
	}
	_, err = s.db.Exec(`UPDATE issues SET milestone_id = ? WHERE id = ?`, milestoneID, issueID)
	return err
}

// IssueMilestones 返回 issue number -> 所属里程碑（精简字段）。
func (s *Store) IssueMilestones(owner, repo string, numbers []int64) (map[int64]Milestone, error) {
	out := map[int64]Milestone{}
	if len(numbers) == 0 {
		return out, nil
	}
	rows, err := s.db.Query(`SELECT i.number, m.id, m.title, m.state FROM issues i
		JOIN milestones m ON m.id = i.milestone_id
		WHERE i.owner = ? AND i.repo = ? AND i.milestone_id IS NOT NULL`, owner, repo)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var num, id int64
		var title, state string
		if err := rows.Scan(&num, &id, &title, &state); err != nil {
			return nil, err
		}
		out[num] = Milestone{ID: id, Title: title, State: state}
	}
	return out, rows.Err()
}

// ---- admin & settings & oauth ----

func (s *Store) AdminCount() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM admin_users`).Scan(&n)
	return n, err
}

func (s *Store) CreateAdminUser(username, passwordHash string) error {
	_, err := s.db.Exec(`INSERT INTO admin_users (username, password_hash, created_at) VALUES (?, ?, ?)`,
		username, passwordHash, now())
	if err != nil && isUniqueErr(err) {
		return ErrExists
	}
	return err
}

func (s *Store) AdminAuth(username string) (int64, string, error) {
	var id int64
	var hash string
	err := s.db.QueryRow(`SELECT id, password_hash FROM admin_users WHERE username = ?`, username).Scan(&id, &hash)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", ErrNotFound
	}
	return id, hash, err
}

func (s *Store) UpdateAdminPassword(username, passwordHash string) error {
	res, err := s.db.Exec(`UPDATE admin_users SET password_hash = ? WHERE username = ?`, passwordHash, username)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) CreateAdminSession(token string, adminID int64) error {
	_, err := s.db.Exec(`INSERT INTO admin_sessions (token, admin_id, created_at, expires_at) VALUES (?, ?, ?, ?)`,
		token, adminID, now(), time.Now().Add(12*time.Hour).UTC().Format(time.RFC3339))
	return err
}

func (s *Store) GetAdminSession(token string) (int64, string, error) {
	var id int64
	var username string
	err := s.db.QueryRow(`SELECT a.id, a.username FROM admin_sessions s
		JOIN admin_users a ON a.id = s.admin_id
		WHERE s.token = ? AND s.expires_at > ?`, token, now()).Scan(&id, &username)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", ErrNotFound
	}
	return id, username, err
}

func (s *Store) DeleteAdminSession(token string) error {
	_, err := s.db.Exec(`DELETE FROM admin_sessions WHERE token = ?`, token)
	return err
}

func (s *Store) GetSetting(key string) string {
	var v string
	_ = s.db.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	return v
}

func (s *Store) SetSetting(key, value string) error {
	_, err := s.db.Exec(`INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

func (s *Store) LinkOAuth(provider, externalID string, userID int64) error {
	_, err := s.db.Exec(`INSERT INTO user_oauth (provider, external_id, user_id, created_at) VALUES (?, ?, ?, ?)`,
		provider, externalID, userID, now())
	if err != nil && isUniqueErr(err) {
		return ErrExists
	}
	return err
}

// OAuthUser 返回 (userID, username) 或 ErrNotFound。
func (s *Store) OAuthUser(provider, externalID string) (int64, string, error) {
	var id int64
	var username string
	err := s.db.QueryRow(`SELECT u.id, u.username FROM user_oauth o JOIN users u ON u.id = o.user_id
		WHERE o.provider = ? AND o.external_id = ?`, provider, externalID).Scan(&id, &username)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", ErrNotFound
	}
	return id, username, err
}

// ---- stars ----

func (s *Store) StarRepo(username, owner, repo string) error {
	_, err := s.db.Exec(`INSERT INTO repo_stars (username, owner, repo, created_at) VALUES (?, ?, ?, ?)`,
		username, owner, repo, now())
	if err != nil && isUniqueErr(err) {
		return ErrExists
	}
	return err
}

func (s *Store) UnstarRepo(username, owner, repo string) error {
	_, err := s.db.Exec(`DELETE FROM repo_stars WHERE username = ? AND owner = ? AND repo = ?`, username, owner, repo)
	return err
}

func (s *Store) IsStarred(username, owner, repo string) bool {
	var one int
	err := s.db.QueryRow(`SELECT 1 FROM repo_stars WHERE username = ? AND owner = ? AND repo = ?`,
		username, owner, repo).Scan(&one)
	return err == nil
}

// StarCounts 返回若干 (owner,repo) 的 star 数。
func (s *Store) StarCounts(pairs [][2]string) map[[2]string]int {
	out := map[[2]string]int{}
	for _, p := range pairs {
		var n int
		_ = s.db.QueryRow(`SELECT COUNT(*) FROM repo_stars WHERE owner = ? AND repo = ?`, p[0], p[1]).Scan(&n)
		out[p] = n
	}
	return out
}

// StarredRepos 我 star 过的公开/可访问仓库。
func (s *Store) StarredRepos(username string) ([]Repo, error) {
	rows, err := s.db.Query(`SELECT r.id, r.owner, r.name, r.description, r.private, r.created_at
		FROM repo_stars st JOIN repos r ON r.owner = st.owner AND r.name = st.repo
		WHERE st.username = ? ORDER BY st.created_at DESC`, username)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Repo{}
	for rows.Next() {
		var r Repo
		if err := rows.Scan(&r.ID, &r.Owner, &r.Name, &r.Description, &r.Private, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) DeleteRepoStars(owner, repo string) error {
	_, err := s.db.Exec(`DELETE FROM repo_stars WHERE owner = ? AND repo = ?`, owner, repo)
	return err
}

// ---- forks ----

// SetForkSource 记录仓库的 fork 来源。
func (s *Store) SetForkSource(owner, repo, sourceOwner, sourceRepo string) error {
	_, err := s.db.Exec(`INSERT INTO repo_forks (owner, repo, source_owner, source_repo, created_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(owner, repo) DO UPDATE SET source_owner = excluded.source_owner, source_repo = excluded.source_repo`,
		owner, repo, sourceOwner, sourceRepo, now())
	return err
}

// ForkSource 返回 fork 来源；非 fork 仓库返回空串。
func (s *Store) ForkSource(owner, repo string) (string, string, error) {
	var so, sr string
	err := s.db.QueryRow(`SELECT source_owner, source_repo FROM repo_forks WHERE owner = ? AND repo = ?`, owner, repo).
		Scan(&so, &sr)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", nil
	}
	return so, sr, err
}

// ---- orgs (namespace) ----

func (s *Store) IsOrg(name string) bool {
	var one int
	err := s.db.QueryRow(`SELECT 1 FROM orgs WHERE name = ?`, name).Scan(&one)
	return err == nil
}

func (s *Store) CreateOrg(name, display, creator string) (Org, error) {
	if _, err := s.GetByUsername(name); err == nil {
		return Org{}, ErrExists // 用户名占用
	}
	o := Org{Name: name, Display: display, CreatedAt: now()}
	tx, err := s.db.Begin()
	if err != nil {
		return o, err
	}
	defer tx.Rollback()
	res, err := tx.Exec(`INSERT INTO orgs (name, display, created_at) VALUES (?, ?, ?)`, name, display, o.CreatedAt)
	if err != nil {
		if isUniqueErr(err) {
			return o, ErrExists
		}
		return o, err
	}
	o.ID, _ = res.LastInsertId()
	if _, err := tx.Exec(`INSERT INTO org_members (org, username, role, created_at) VALUES (?, ?, 'owner', ?)`,
		name, creator, now()); err != nil {
		return o, err
	}
	return o, tx.Commit()
}

func (s *Store) ListMyOrgs(username string) ([]Org, error) {
	rows, err := s.db.Query(`SELECT o.id, o.name, o.display, o.created_at FROM orgs o
		JOIN org_members m ON m.org = o.name WHERE m.username = ? ORDER BY o.name`, username)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Org{}
	for rows.Next() {
		var o Org
		if err := rows.Scan(&o.ID, &o.Name, &o.Display, &o.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func (s *Store) OrgRole(org, username string) string {
	var role string
	err := s.db.QueryRow(`SELECT role FROM org_members WHERE org = ? AND username = ?`, org, username).Scan(&role)
	if err != nil {
		return ""
	}
	return role
}

func (s *Store) OrgMembers(org string) ([]OrgMember, error) {
	rows, err := s.db.Query(`SELECT org, username, role FROM org_members WHERE org = ? ORDER BY username`, org)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []OrgMember{}
	for rows.Next() {
		var m OrgMember
		if err := rows.Scan(&m.Org, &m.Username, &m.Role); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) AddOrgMember(org, username, role string) error {
	_, err := s.db.Exec(`INSERT INTO org_members (org, username, role, created_at) VALUES (?, ?, ?, ?)
		ON CONFLICT(org, username) DO UPDATE SET role = excluded.role`, org, username, role, now())
	return err
}

func (s *Store) RemoveOrgMember(org, username string) error {
	res, err := s.db.Exec(`DELETE FROM org_members WHERE org = ? AND username = ?`, org, username)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteOrg(org string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var cnt int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM repos WHERE owner = ?`, org).Scan(&cnt); err != nil {
		return err
	}
	if cnt > 0 {
		return errors.New("org not empty")
	}
	if _, err := tx.Exec(`DELETE FROM org_members WHERE org = ?`, org); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM orgs WHERE name = ?`, org); err != nil {
		return err
	}
	return tx.Commit()
}
