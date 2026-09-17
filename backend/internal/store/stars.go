package store

import "gitdash/backend/internal/logx"

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
	// 按 owner 分块，避免超出 SQLite/PostgreSQL 的绑定参数上限。
	for _, part := range chunkStrings(owners, 0) {
		var rows []struct {
			Owner string
			Repo  string
			N     int
		}
		if err := s.db.Table(table).Select("owner, repo, COUNT(*) AS n").
			Where("owner IN ?", part).
			Group("owner, repo").Scan(&rows).Error; err != nil {
			// 不静默吞错：计数失败会让前端显示 0，必须留下可观测日志。
			logx.Error("countPairs " + table + ": " + err.Error())
			continue
		}
		for _, r := range rows {
			p := [2]string{r.Owner, r.Repo}
			if want[p] {
				out[p] = r.N
			}
		}
	}
	return out
}

// StarredAmong 返回我 star 过的、且落在给定 pairs 中的集合（只查这些 owner，避免全量加载）。
func (s *Store) StarredAmong(username string, pairs [][2]string) map[[2]string]bool {
	return s.pairSubset("repo_stars", username, pairs)
}

// WatchingAmong 返回我 watch 过的、且落在给定 pairs 中的集合。
func (s *Store) WatchingAmong(username string, pairs [][2]string) map[[2]string]bool {
	return s.pairSubset("repo_watches", username, pairs)
}

// pairSubset 查询 username 在 table 中命中给定 pairs 的子集。
// 只按 pairs 涉及的 owner 分块查询，结果规模与传入列表同阶，不再随用户历史增长。
func (s *Store) pairSubset(table, username string, pairs [][2]string) map[[2]string]bool {
	out := map[[2]string]bool{}
	if len(pairs) == 0 {
		return out
	}
	want := make(map[[2]string]bool, len(pairs))
	owners := make([]string, 0, len(pairs))
	seen := map[string]bool{}
	for _, p := range pairs {
		want[p] = true
		if !seen[p[0]] {
			seen[p[0]] = true
			owners = append(owners, p[0])
		}
	}
	for _, part := range chunkStrings(owners, 0) {
		var rows []struct {
			Owner string
			Repo  string
		}
		if err := s.db.Table(table).Select("owner, repo").
			Where("username = ? AND owner IN ?", username, part).Scan(&rows).Error; err != nil {
			logx.Error("pairSubset " + table + ": " + err.Error())
			continue
		}
		for _, r := range rows {
			p := [2]string{r.Owner, r.Repo}
			if want[p] {
				out[p] = true
			}
		}
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

// StarredRepos 我 star 过的公开/可访问仓库（分页，按 star 时间倒序）。
func (s *Store) StarredRepos(username string, limit, offset int) ([]Repo, error) {
	var rows []repoRow
	q := s.db.Select("repos.*").
		Joins("JOIN repo_stars st ON repos.owner = st.owner AND repos.name = st.repo").
		Where("st.username = ? AND repos.banned = ?", username, false).
		// owner/repo 作为唯一的次级排序键，保证 offset 分页稳定，不会重复/漏项。
		Order("st.created_at DESC, st.owner, st.repo")
	if err := paginate(q, limit, offset).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := []Repo{}
	for _, r := range rows {
		out = append(out, toRepo(r))
	}
	return out, nil
}

// CountStarredRepos 我 star 过的仓库总数（与 StarredRepos 口径一致）。
func (s *Store) CountStarredRepos(username string) (int, error) {
	var n int64
	err := s.db.Model(&starRow{}).
		Joins("JOIN repos ON repos.owner = repo_stars.owner AND repos.name = repo_stars.repo").
		Where("repo_stars.username = ? AND repos.banned = ?", username, false).
		Count(&n).Error
	return int(n), err
}

func (s *Store) DeleteRepoStars(owner, repo string) error {
	return s.db.Where("owner = ? AND repo = ?", owner, repo).Delete(&starRow{}).Error
}

// ---- watch ----
