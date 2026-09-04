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
	CreatedAt   string `json:"created_at"`
	// Role 仅用于“可访问仓库列表”（owner / read / write），普通查询为空
	Role string `json:"role,omitempty"`
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

type Webhook struct {
	ID        int64  `json:"id"`
	Owner     string `json:"owner"`
	Repo      string `json:"repo"`
	URL       string `json:"url"`
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
	UNIQUE(owner, repo, number)
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
	created_at TEXT NOT NULL,
	UNIQUE(owner, repo, url)
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

func (s *Store) CreateRepo(owner, name, description string) (Repo, error) {
	r := Repo{Owner: owner, Name: name, Description: description, CreatedAt: now()}
	res, err := s.db.Exec(`INSERT INTO repos (owner, name, description, created_at) VALUES (?, ?, ?, ?)`,
		r.Owner, r.Name, r.Description, r.CreatedAt)
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
	rows, err := s.db.Query(`SELECT id, owner, name, description, created_at FROM repos WHERE owner = ? ORDER BY name`, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	repos := []Repo{}
	for rows.Next() {
		var r Repo
		if err := rows.Scan(&r.ID, &r.Owner, &r.Name, &r.Description, &r.CreatedAt); err != nil {
			return nil, err
		}
		repos = append(repos, r)
	}
	return repos, rows.Err()
}

func (s *Store) GetRepo(owner, name string) (Repo, error) {
	var r Repo
	err := s.db.QueryRow(`SELECT id, owner, name, description, created_at FROM repos WHERE owner = ? AND name = ?`, owner, name).
		Scan(&r.ID, &r.Owner, &r.Name, &r.Description, &r.CreatedAt)
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
	if _, err := tx.Exec(`DELETE FROM repo_collabs WHERE owner = ? AND repo = ?`, owner, name); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM webhooks WHERE owner = ? AND repo = ?`, owner, name); err != nil {
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
	var one int
	err := s.db.QueryRow(`SELECT 1 FROM repo_collabs WHERE owner = ? AND repo = ? AND username = ?`, owner, repo, username).Scan(&one)
	return err == nil
}

func (s *Store) CanWrite(owner, repo, username string) bool {
	if owner == username {
		return true
	}
	var one int
	err := s.db.QueryRow(`SELECT 1 FROM repo_collabs WHERE owner = ? AND repo = ? AND username = ? AND permission = 'write'`, owner, repo, username).Scan(&one)
	return err == nil
}

// AccessibleRepos 返回用户自己拥有的仓库 + 作为协作者可访问的仓库（带 role）。
func (s *Store) AccessibleRepos(username string) ([]Repo, error) {
	rows, err := s.db.Query(`
SELECT r.id, r.owner, r.name, r.description, r.created_at, 'owner' AS role
  FROM repos r WHERE r.owner = ?
UNION ALL
SELECT r.id, r.owner, r.name, r.description, r.created_at, c.permission AS role
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
		if err := rows.Scan(&r.ID, &r.Owner, &r.Name, &r.Description, &r.CreatedAt, &r.Role); err != nil {
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

func (s *Store) CreateWebhook(owner, repo, url string) (Webhook, error) {
	w := Webhook{Owner: owner, Repo: repo, URL: url, CreatedAt: now()}
	res, err := s.db.Exec(`INSERT INTO webhooks (owner, repo, url, created_at) VALUES (?, ?, ?, ?)`,
		owner, repo, url, w.CreatedAt)
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
	rows, err := s.db.Query(`SELECT id, owner, repo, url, created_at
		FROM webhooks WHERE owner = ? AND repo = ? ORDER BY id`, owner, repo)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ws := []Webhook{}
	for rows.Next() {
		var w Webhook
		if err := rows.Scan(&w.ID, &w.Owner, &w.Repo, &w.URL, &w.CreatedAt); err != nil {
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
