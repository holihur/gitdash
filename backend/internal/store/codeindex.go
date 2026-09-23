package store

// AllReposAfter 按 id 升序返回 id > afterID 的未封禁仓库（游标式全量遍历，
// 供代码索引启动回填）。返回 DefaultBranch 为空时按 main 处理。
func (s *Store) AllReposAfter(afterID int64, limit int) ([]RepoLanguageRef, error) {
	var rows []repoRow
	q := s.db.Where("id > ? AND banned = ?", afterID, false).Order("id")
	if limit > 0 {
		q = q.Limit(limit)
	}
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]RepoLanguageRef, 0, len(rows))
	for _, r := range rows {
		def := r.DefaultBranch
		if def == "" {
			def = "main"
		}
		out = append(out, RepoLanguageRef{ID: r.ID, Owner: r.Owner, Repo: r.Name, DefaultBranch: def})
	}
	return out, nil
}
