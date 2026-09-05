package store

// countPairs 一次 GROUP BY 查询统计多个 (owner,repo) 的计数，避免逐条 COUNT。
// 用 owner IN (...) 圈定候选行，再 GROUP BY owner, repo，最后在内存按请求的 pair 过滤
// （替代原 SQLite/PG 方言不一致的 VALUES 行构造写法，两后端均兼容）。
func (s *Store) countPairs(table string, pairs [][2]string) map[[2]string]int {
	out := map[[2]string]int{}
	if len(pairs) == 0 {
		return out
	}
	want := map[[2]string]bool{}
	owners := []string{}
	seen := map[string]bool{}
	for _, p := range pairs {
		want[p] = true
		if !seen[p[0]] {
			seen[p[0]] = true
			owners = append(owners, p[0])
		}
	}
	var rows []struct {
		Owner string
		Repo  string
		N     int
	}
	if err := s.db.Table(table).Select("owner, repo, COUNT(*) AS n").
		Where("owner IN ?", owners).
		Group("owner, repo").Scan(&rows).Error; err != nil {
		return out
	}
	for _, r := range rows {
		p := [2]string{r.Owner, r.Repo}
		if want[p] {
			out[p] = r.N
		}
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
	var rows []struct {
		Owner string
		Repo  string
	}
	if err := s.db.Table(table).Select("owner, repo").Where("username = ?", username).Scan(&rows).Error; err != nil {
		return out
	}
	for _, r := range rows {
		out[[2]string{r.Owner, r.Repo}] = true
	}
	return out
}

func (s *Store) StarRepo(username, owner, repo string) error {
	row := starRow{Username: username, Owner: owner, Repo: repo, CreatedAt: now()}
	if err := s.db.Create(&row).Error; err != nil {
		if isUniqueErr(err) {
			return ErrExists
		}
		return err
	}
	return nil
}

func (s *Store) UnstarRepo(username, owner, repo string) error {
	return s.db.Where("username = ? AND owner = ? AND repo = ?", username, owner, repo).Delete(&starRow{}).Error
}

func (s *Store) IsStarred(username, owner, repo string) bool {
	var n int64
	err := s.db.Model(&starRow{}).Where("username = ? AND owner = ? AND repo = ?", username, owner, repo).
		Limit(1).Count(&n).Error
	return err == nil && n > 0
}

// StarCounts 返回若干 (owner,repo) 的 star 数（单次 GROUP BY 查询）。
func (s *Store) StarCounts(pairs [][2]string) map[[2]string]int {
	return s.countPairs("repo_stars", pairs)
}

// StarredRepos 我 star 过的公开/可访问仓库。
func (s *Store) StarredRepos(username string) ([]Repo, error) {
	var rows []repoRow
	if err := s.db.Select("repos.*").Joins("JOIN repo_stars st ON repos.owner = st.owner AND repos.name = st.repo").
		Where("st.username = ?", username).Order("st.created_at DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := []Repo{}
	for _, r := range rows {
		out = append(out, toRepo(r))
	}
	return out, nil
}

func (s *Store) DeleteRepoStars(owner, repo string) error {
	return s.db.Where("owner = ? AND repo = ?", owner, repo).Delete(&starRow{}).Error
}

// ---- watch ----
