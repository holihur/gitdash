package store

import "strings"

// countPairs 一次 GROUP BY 查询统计多个 (owner,repo) 的计数，避免逐条 COUNT。
func (s *Store) countPairs(table string, pairs [][2]string) map[[2]string]int {
	out := map[[2]string]int{}
	if len(pairs) == 0 {
		return out
	}
	var sb strings.Builder
	args := make([]any, 0, len(pairs)*2)
	for i, p := range pairs {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString("(?, ?)")
		args = append(args, p[0], p[1])
	}
	q := `SELECT t.column1, t.column2, COUNT(s.owner) FROM (VALUES ` + sb.String() + `) AS t
		LEFT JOIN ` + table + ` s ON s.owner = t.column1 AND s.repo = t.column2
		GROUP BY t.column1, t.column2`
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return out
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var o, r string
		var n int
		if err := rows.Scan(&o, &r, &n); err != nil {
			continue
		}
		out[[2]string{o, r}] = n
	}
	return out
}

// StarredSet 我 star 过的 (owner,repo) 集合。
func (s *Store) StarredSet(username string) map[[2]string]bool {
	return s.pairSet("repo_stars", username)
}

// WatchingSet 我 watch 过的 (owner,repo) 集合。
func (s *Store) WatchingSet(username string) map[[2]string]bool {
	return s.pairSet("repo_watches", username)
}

func (s *Store) pairSet(table, username string) map[[2]string]bool {
	out := map[[2]string]bool{}
	rows, err := s.db.Query(`SELECT owner, repo FROM `+table+` WHERE username = ?`, username)
	if err != nil {
		return out
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var o, r string
		if err := rows.Scan(&o, &r); err != nil {
			continue
		}
		out[[2]string{o, r}] = true
	}
	return out
}

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

// StarCounts 返回若干 (owner,repo) 的 star 数（单次 GROUP BY 查询）。
func (s *Store) StarCounts(pairs [][2]string) map[[2]string]int {
	return s.countPairs("repo_stars", pairs)
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
