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

type Repo struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	CreatedAt   string `json:"created_at"`
}

type SSHKey struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	PublicKey   string `json:"public_key"`
	Fingerprint string `json:"fingerprint"`
	CreatedAt   string `json:"created_at"`
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
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS repos (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	name        TEXT NOT NULL UNIQUE,
	description TEXT NOT NULL DEFAULT '',
	created_at  TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS ssh_keys (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	name        TEXT NOT NULL,
	public_key  TEXT NOT NULL,
	fingerprint TEXT NOT NULL UNIQUE,
	created_at  TEXT NOT NULL
);`)
	return err
}

func now() string { return time.Now().UTC().Format(time.RFC3339) }

func (s *Store) CreateRepo(name, description string) (Repo, error) {
	r := Repo{Name: name, Description: description, CreatedAt: now()}
	res, err := s.db.Exec(`INSERT INTO repos (name, description, created_at) VALUES (?, ?, ?)`,
		r.Name, r.Description, r.CreatedAt)
	if err != nil {
		if isUniqueErr(err) {
			return r, ErrExists
		}
		return r, err
	}
	r.ID, _ = res.LastInsertId()
	return r, nil
}

func (s *Store) ListRepos() ([]Repo, error) {
	rows, err := s.db.Query(`SELECT id, name, description, created_at FROM repos ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	repos := []Repo{}
	for rows.Next() {
		var r Repo
		if err := rows.Scan(&r.ID, &r.Name, &r.Description, &r.CreatedAt); err != nil {
			return nil, err
		}
		repos = append(repos, r)
	}
	return repos, rows.Err()
}

func (s *Store) GetRepo(name string) (Repo, error) {
	var r Repo
	err := s.db.QueryRow(`SELECT id, name, description, created_at FROM repos WHERE name = ?`, name).
		Scan(&r.ID, &r.Name, &r.Description, &r.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return r, ErrNotFound
	}
	return r, err
}

func (s *Store) DeleteRepo(name string) error {
	res, err := s.db.Exec(`DELETE FROM repos WHERE name = ?`, name)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) CreateKey(name, publicKey, fingerprint string) (SSHKey, error) {
	k := SSHKey{Name: name, PublicKey: publicKey, Fingerprint: fingerprint, CreatedAt: now()}
	res, err := s.db.Exec(`INSERT INTO ssh_keys (name, public_key, fingerprint, created_at) VALUES (?, ?, ?, ?)`,
		k.Name, k.PublicKey, k.Fingerprint, k.CreatedAt)
	if err != nil {
		if isUniqueErr(err) {
			return k, ErrExists
		}
		return k, err
	}
	k.ID, _ = res.LastInsertId()
	return k, nil
}

func (s *Store) ListKeys() ([]SSHKey, error) {
	rows, err := s.db.Query(`SELECT id, name, public_key, fingerprint, created_at FROM ssh_keys ORDER BY id DESC`)
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

func (s *Store) PublicKeys() ([]string, error) {
	rows, err := s.db.Query(`SELECT public_key FROM ssh_keys`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var keys []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

func (s *Store) DeleteKey(id int64) error {
	res, err := s.db.Exec(`DELETE FROM ssh_keys WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func isUniqueErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}
