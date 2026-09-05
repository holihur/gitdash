package store

import (
	"database/sql"
	"errors"
)

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

// ---- imports ----

// SetImportSource 记录仓库的导入来源 URL。
func (s *Store) SetImportSource(owner, repo, url string) error {
	_, err := s.db.Exec(`INSERT INTO repo_imports (owner, repo, source_url, created_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(owner, repo) DO UPDATE SET source_url = excluded.source_url`,
		owner, repo, url, now())
	return err
}

// ImportSource 返回导入来源 URL；非导入仓库返回空串。
func (s *Store) ImportSource(owner, repo string) (string, error) {
	var u string
	err := s.db.QueryRow(`SELECT source_url FROM repo_imports WHERE owner = ? AND repo = ?`, owner, repo).Scan(&u)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return u, err
}

// ---- mirrors ----

// SetMirror 配置仓库的 push 镜像目标（覆盖式）。
func (s *Store) SetMirror(owner, repo, url, privateKey string) error {
	_, err := s.db.Exec(`INSERT INTO repo_mirrors (owner, repo, url, private_key, created_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(owner, repo) DO UPDATE SET url = excluded.url, private_key = excluded.private_key`,
		owner, repo, url, privateKey, now())
	return err
}

// GetMirror 返回镜像配置；未配置返回空 URL。
func (s *Store) GetMirror(owner, repo string) (Mirror, error) {
	var m Mirror
	err := s.db.QueryRow(`SELECT owner, repo, url, private_key, created_at FROM repo_mirrors WHERE owner = ? AND repo = ?`, owner, repo).
		Scan(&m.Owner, &m.Repo, &m.URL, &m.PrivateKey, &m.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Mirror{Owner: owner, Repo: repo}, nil
	}
	return m, err
}

// DeleteMirror 移除镜像配置。
func (s *Store) DeleteMirror(owner, repo string) error {
	_, err := s.db.Exec(`DELETE FROM repo_mirrors WHERE owner = ? AND repo = ?`, owner, repo)
	return err
}

// ---- orgs (namespace) ----
