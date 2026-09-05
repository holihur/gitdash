package store

func (s *Store) WatchRepo(username, owner, repo string) error {
	_, err := s.db.Exec(`INSERT INTO repo_watches (username, owner, repo, created_at) VALUES (?, ?, ?, ?)
		ON CONFLICT(username, owner, repo) DO NOTHING`, username, owner, repo, now())
	return err
}

func (s *Store) UnwatchRepo(username, owner, repo string) error {
	_, err := s.db.Exec(`DELETE FROM repo_watches WHERE username = ? AND owner = ? AND repo = ?`, username, owner, repo)
	return err
}

func (s *Store) IsWatching(username, owner, repo string) bool {
	var one int
	err := s.db.QueryRow(`SELECT 1 FROM repo_watches WHERE username = ? AND owner = ? AND repo = ?`,
		username, owner, repo).Scan(&one)
	return err == nil
}

// WatchedRepos 我 watch 过的仓库（可能含已删除仓库的残留——join repos 过滤）。
func (s *Store) WatchedRepos(username string) ([]Repo, error) {
	rows, err := s.db.Query(`SELECT r.id, r.owner, r.name, r.description, r.private, r.created_at
		FROM repo_watches w JOIN repos r ON r.owner = w.owner AND r.name = w.repo
		WHERE w.username = ? ORDER BY w.created_at DESC`, username)
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

// WatchCounts 返回若干 (owner,repo) 的 watch 数。
func (s *Store) WatchCounts(pairs [][2]string) map[[2]string]int {
	out := map[[2]string]int{}
	for _, p := range pairs {
		var n int
		_ = s.db.QueryRow(`SELECT COUNT(*) FROM repo_watches WHERE owner = ? AND repo = ?`, p[0], p[1]).Scan(&n)
		out[p] = n
	}
	return out
}

// WatchingUsers 显式 watch 某仓库的用户（不含仓库所有者/组织成员等隐式订阅者）。
func (s *Store) WatchingUsers(owner, repo string) ([]string, error) {
	rows, err := s.db.Query(`SELECT username FROM repo_watches WHERE owner = ? AND repo = ?`, owner, repo)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []string{}
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// ---- inbox (notifications) ----
