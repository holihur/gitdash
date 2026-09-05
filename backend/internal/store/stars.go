package store

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

func (s *Store) DeleteRepoStars(owner, repo string) error {
	_, err := s.db.Exec(`DELETE FROM repo_stars WHERE owner = ? AND repo = ?`, owner, repo)
	return err
}

// ---- watch ----
