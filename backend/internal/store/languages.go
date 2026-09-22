package store

import "gorm.io/gorm"

// ReplaceRepoLanguages 用一次分析结果整体替换仓库的语言构成。
//
// stats 允许为空（仓库没有任何可识别代码）：此时仅更新 meta 行，标记该仓库
// 已分析过，避免启动时的回填任务反复扫描。
func (s *Store) ReplaceRepoLanguages(owner, repo, sha string, stats []LanguageStat) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("owner = ? AND repo = ?", owner, repo).Delete(&repoLanguageRow{}).Error; err != nil {
			return err
		}
		rows := make([]repoLanguageRow, 0, len(stats))
		primary := ""
		for _, st := range stats {
			if st.Language == "" || st.Bytes <= 0 {
				continue
			}
			if primary == "" {
				primary = st.Language
			}
			rows = append(rows, repoLanguageRow{Owner: owner, Repo: repo, Language: st.Language, Bytes: st.Bytes})
		}
		if len(rows) > 0 {
			if err := tx.Create(&rows).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("owner = ? AND repo = ?", owner, repo).Delete(&repoLanguageMetaRow{}).Error; err != nil {
			return err
		}
		return tx.Create(&repoLanguageMetaRow{
			Owner: owner, Repo: repo, Primary: primary, SHA: sha, UpdatedAt: now(),
		}).Error
	})
}

// RepoLanguages 返回仓库的语言构成（按字节降序）；limit>0 时只返回前 limit 项。
// 返回值中的 Percent 基于该项占全部返回项（截断前）的比例计算。
func (s *Store) RepoLanguages(owner, repo string, limit int) ([]LanguageStat, error) {
	var rows []repoLanguageRow
	q := s.db.Where("owner = ? AND repo = ?", owner, repo).Order("bytes DESC, language")
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	var total int64
	for _, r := range rows {
		total += r.Bytes
	}
	out := make([]LanguageStat, 0, len(rows))
	for _, r := range rows {
		st := LanguageStat{Language: r.Language, Bytes: r.Bytes}
		if total > 0 {
			st.Percent = float64(r.Bytes) * 100 / float64(total)
		}
		out = append(out, st)
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// PrimaryLanguages 批量返回仓库的主要语言（用于列表页，避免 N+1）。
// 未分析过的仓库不会出现在返回值中。
func (s *Store) PrimaryLanguages(pairs [][2]string) map[[2]string]string {
	out := map[[2]string]string{}
	if len(pairs) == 0 {
		return out
	}
	owners := make([]string, 0, len(pairs))
	seen := map[string]bool{}
	for _, p := range pairs {
		if !seen[p[0]] {
			seen[p[0]] = true
			owners = append(owners, p[0])
		}
	}
	var rows []repoLanguageMetaRow
	if err := s.db.Where("owner IN ?", owners).Find(&rows).Error; err != nil {
		return out
	}
	for _, r := range rows {
		if r.Primary != "" {
			out[[2]string{r.Owner, r.Repo}] = r.Primary
		}
	}
	return out
}

// RepoLanguageSHA 返回最近一次分析所用的 commit（无记录时为空串）。
func (s *Store) RepoLanguageSHA(owner, repo string) string {
	var row repoLanguageMetaRow
	if err := s.db.Where("owner = ? AND repo = ?", owner, repo).First(&row).Error; err != nil {
		return ""
	}
	return row.SHA
}

// RepoLanguageRef 待分析仓库（尚无语言记录）。
type RepoLanguageRef struct {
	ID            int64
	Owner         string
	Repo          string
	DefaultBranch string
}

// ReposMissingLanguages 返回尚未做过语言分析的仓库（按 id 升序，id > afterID），
// 供启动时回填。afterID 作为游标保证每个仓库在一次回填中只入队一次。
func (s *Store) ReposMissingLanguages(afterID int64, limit int) ([]RepoLanguageRef, error) {
	var rows []repoRow
	q := s.db.Where("id > ? AND banned = ?", afterID, false).
		Where("NOT EXISTS (SELECT 1 FROM repo_language_meta m WHERE m.owner = repos.owner AND m.repo = repos.name)").
		Order("id")
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
