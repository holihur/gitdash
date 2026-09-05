package store

import (
	"gorm.io/gorm/clause"
)

func (s *Store) WatchRepo(username, owner, repo string) error {
	row := watchRow{Username: username, Owner: owner, Repo: repo, CreatedAt: now()}
	return s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "username"}, {Name: "owner"}, {Name: "repo"}},
		DoNothing: true,
	}).Create(&row).Error
}

func (s *Store) UnwatchRepo(username, owner, repo string) error {
	return s.db.Where("username = ? AND owner = ? AND repo = ?", username, owner, repo).Delete(&watchRow{}).Error
}

func (s *Store) IsWatching(username, owner, repo string) bool {
	var n int64
	err := s.db.Model(&watchRow{}).Where("username = ? AND owner = ? AND repo = ?", username, owner, repo).
		Limit(1).Count(&n).Error
	return err == nil && n > 0
}

// WatchedRepos 我 watch 过的仓库（可能含已删除仓库的残留——join repos 过滤）。
func (s *Store) WatchedRepos(username string) ([]Repo, error) {
	var rows []repoRow
	if err := s.db.Select("repos.*").Joins("JOIN repo_watches w ON repos.owner = w.owner AND repos.name = w.repo").
		Where("w.username = ?", username).Order("w.created_at DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := []Repo{}
	for _, r := range rows {
		out = append(out, toRepo(r))
	}
	return out, nil
}

// WatchCounts 返回若干 (owner,repo) 的 watch 数（单次 GROUP BY 查询）。
func (s *Store) WatchCounts(pairs [][2]string) map[[2]string]int {
	return s.countPairs("repo_watches", pairs)
}

// WatchingUsers 显式 watch 某仓库的用户（不含仓库所有者/组织成员等隐式订阅者）。
func (s *Store) WatchingUsers(owner, repo string) ([]string, error) {
	var rows []watchRow
	if err := s.db.Select("username").Where("owner = ? AND repo = ?", owner, repo).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := []string{}
	for _, r := range rows {
		out = append(out, r.Username)
	}
	return out, nil
}

// ---- inbox (notifications) ----
