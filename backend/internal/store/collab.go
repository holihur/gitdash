package store

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
	defer func() { _ = rows.Close() }()
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
