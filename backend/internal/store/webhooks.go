package store

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
	defer func() { _ = rows.Close() }()
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
