package store

import (
	"database/sql"
	"errors"
)

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
	defer func() { _ = rows.Close() }()
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
	defer func() { _ = rows.Close() }()
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
	defer func() { _ = tx.Rollback() }()
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
	if _, err := tx.Exec(`DELETE FROM repo_watches WHERE owner = ? AND repo = ?`, owner, name); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM notifications WHERE owner = ? AND repo = ?`, owner, name); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM repo_forks WHERE owner = ? AND repo = ?`, owner, name); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM repo_forks WHERE source_owner = ? AND source_repo = ?`, owner, name); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM repo_imports WHERE owner = ? AND repo = ?`, owner, name); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM repo_mirrors WHERE owner = ? AND repo = ?`, owner, name); err != nil {
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
	if _, err := tx.Exec(`DELETE FROM repo_pipelines WHERE owner = ? AND repo = ?`, owner, name); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM pipeline_runs WHERE owner = ? AND repo = ?`, owner, name); err != nil {
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
	defer func() { _ = rows.Close() }()
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

// AccessibleRepos 返回用户自己拥有的仓库 + 所在组织的仓库 + 作为协作者可访问的仓库（带 role）。
func (s *Store) AccessibleRepos(username string) ([]Repo, error) {
	rows, err := s.db.Query(`
SELECT r.id, r.owner, r.name, r.description, r.private, r.created_at, 'owner' AS role
  FROM repos r WHERE r.owner = ?
UNION ALL
SELECT r.id, r.owner, r.name, r.description, r.private, r.created_at,
       CASE m.role WHEN 'owner' THEN 'owner' ELSE 'write' END AS role
  FROM org_members m JOIN repos r ON r.owner = m.org
 WHERE m.username = ?
UNION ALL
SELECT r.id, r.owner, r.name, r.description, r.private, r.created_at, c.permission AS role
  FROM repo_collabs c JOIN repos r ON r.owner = c.owner AND r.name = c.repo
 WHERE c.username = ?
ORDER BY owner, name`, username, username, username)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
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
