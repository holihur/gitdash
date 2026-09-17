package store

import "strings"

// ---- 全局搜索（跨仓库：repo / issue / 用户与组织）----

func likePat(q string) string {
	q = strings.ToLower(q)
	q = strings.ReplaceAll(q, "%", "\\%")
	q = strings.ReplaceAll(q, "_", "\\_")
	return "%" + q + "%"
}

// SearchRepos 公开仓库模糊搜索（owner/name/description，大小写不敏感）。
func (s *Store) SearchRepos(q string, limit int) ([]Repo, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	pat := likePat(q)
	var rows []repoRow
	if err := s.db.Where("private = ? AND banned = ? AND owner NOT IN (SELECT name FROM orgs WHERE banned = ?) AND (LOWER(owner) LIKE ? OR LOWER(name) LIKE ? OR LOWER(description) LIKE ?)",
		false, false, true, pat, pat, pat).Order("id DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := []Repo{}
	for _, r := range rows {
		out = append(out, toRepo(r))
	}
	return out, nil
}

// SearchIssues 标题/正文模糊搜索：
// 命中范围 = 公开仓库的 issue + 搜索者自己仓库的 issue（不泄露私有仓库）。
func (s *Store) SearchIssues(q, me string, limit int) ([]Issue, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	pat := likePat(q)
	var rows []issueRow
	err := s.db.Where(
		"(LOWER(title) LIKE ? OR LOWER(body) LIKE ?) AND (owner = ? OR EXISTS (SELECT 1 FROM repos r WHERE r.owner = issues.owner AND r.name = issues.repo AND r.private = ?))",
		pat, pat, me, false,
	).Order("updated_at DESC").Limit(limit).Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := []Issue{}
	for _, r := range rows {
		out = append(out, Issue{
			ID: r.ID, Owner: r.Owner, Repo: r.Repo, Number: r.Number,
			Title: r.Title, Body: r.Body, State: r.State, Author: r.Author,
			CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, ClosedAt: r.ClosedAt,
		})
	}
	return out, nil
}

// SearchUsersResult 用户/组织搜索结果条目。
type SearchUsersResult struct {
	Kind      string `json:"kind"` // user | org
	Name      string `json:"name"`
	Display   string `json:"display,omitempty"`
	CreatedAt string `json:"created_at"`
}

// SearchUsers 用户与组织名模糊搜索（仅公开信息：用户名 / 组织名与显示名）。
func (s *Store) SearchUsers(q string, limit int) ([]SearchUsersResult, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	pat := likePat(q)
	var userRows []userRow
	if err := s.db.Where("LOWER(username) LIKE ? AND banned = ?", pat, false).Order("username").Limit(limit).Find(&userRows).Error; err != nil {
		return nil, err
	}
	out := []SearchUsersResult{}
	for _, r := range userRows {
		out = append(out, SearchUsersResult{Kind: "user", Name: r.Username, CreatedAt: r.CreatedAt})
	}
	var orgRows []orgRow
	if err := s.db.Where("(LOWER(name) LIKE ? OR LOWER(display) LIKE ?) AND banned = ?", pat, pat, false).
		Order("name").Limit(limit).Find(&orgRows).Error; err != nil {
		return nil, err
	}
	for _, r := range orgRows {
		out = append(out, SearchUsersResult{Kind: "org", Name: r.Name, Display: r.Display, CreatedAt: r.CreatedAt})
	}
	return out, nil
}

// CodeSearchRepos 返回全局代码搜索的候选仓库：当前用户可访问的仓库
// （自有 / 组织 / 协作者）+ 公开仓库，过滤封禁仓库与被封禁组织，去重后排序。
// max<=0 表示不限制；否则在 SQL 层限制条数。
func (s *Store) CodeSearchRepos(username string, max int) ([]Repo, error) {
	sql := `SELECT r.id, r.owner, r.name, r.description, r.private, r.is_template, r.banned,
			r.default_branch, r.has_issues, r.created_at
		FROM repos r
		WHERE r.banned = ? AND NOT EXISTS (SELECT 1 FROM orgs o WHERE o.name = r.owner AND o.banned = ?)
		  AND (r.private = ? OR r.id IN (SELECT t.id FROM (` + accessibleReposSubquery + `) t))
		ORDER BY r.owner, r.name`
	args := []any{false, true, false, username, username, username, false, true}
	if max > 0 {
		sql += " LIMIT ?"
		args = append(args, max)
	}
	var rows []repoRow
	if err := s.db.Raw(sql, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]Repo, 0, len(rows))
	for _, r := range rows {
		out = append(out, toRepo(r))
	}
	return out, nil
}
