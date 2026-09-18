package store

import (
	"errors"
	"strings"

	"gorm.io/gorm"
)

// issueToDTO 把 ORM 行转换为公共 DTO。
func issueToDTO(r issueRow) Issue {
	it := Issue{
		ID: r.ID, Owner: r.Owner, Repo: r.Repo, Number: r.Number,
		Title: r.Title, Body: r.Body, State: r.State, Pinned: r.Pinned, Author: r.Author,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
	if r.ClosedAt != nil {
		v := *r.ClosedAt
		it.ClosedAt = &v
	}
	return it
}

func (s *Store) getIssue(owner, repo string, number int64) (Issue, error) {
	var r issueRow
	if err := s.db.Where("owner = ? AND repo = ? AND number = ?", owner, repo, number).
		First(&r).Error; err != nil {
		return Issue{}, notFoundErr(err)
	}
	return issueToDTO(r), nil
}

func (s *Store) CreateIssue(owner, repo, author, title, body string) (Issue, error) {
	now := now()
	// 号码由仓库级持久计数器分配：同一仓库内单调递增，删除后不复用。
	number, err := s.nextNumber(owner, repo, counterIssue)
	if err != nil {
		return Issue{}, err
	}
	r := issueRow{
		Owner: owner, Repo: repo, Number: number,
		Title: title, Body: body, State: "open", Author: author,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := s.db.Create(&r).Error; err != nil {
		return Issue{}, err
	}
	return Issue{ID: r.ID, Owner: owner, Repo: repo, Number: r.Number,
		Title: title, Body: body, State: "open", Author: author,
		CreatedAt: now, UpdatedAt: now}, nil
}

// ListIssues 分页列出 issue；limit<=0 表示不限制。
func (s *Store) ListIssues(owner, repo string, limit, offset int) ([]Issue, error) {
	q := s.db.Model(&issueRow{}).Where("owner = ? AND repo = ?", owner, repo).
		Order("pinned DESC, state = 'open' DESC, number DESC")
	if limit > 0 {
		q = q.Limit(limit).Offset(offset)
	}
	var rows []issueRow
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	issues := []Issue{}
	for _, r := range rows {
		issues = append(issues, issueToDTO(r))
	}
	return issues, nil
}

// issueQuery 构造仓库 issue 查询（可选关键词与状态过滤）。
// 关键词命中标题 / 正文 / 作者（大小写不敏感）。
func (s *Store) issueQuery(owner, repo, q, state string) *gorm.DB {
	query := s.db.Model(&issueRow{}).Where("owner = ? AND repo = ?", owner, repo)
	if state == "open" || state == "closed" {
		query = query.Where("state = ?", state)
	}
	if q = strings.TrimSpace(q); q != "" {
		pat := likePat(q)
		query = query.Where(
			"(LOWER(title) LIKE ? OR LOWER(body) LIKE ? OR LOWER(author) LIKE ?)",
			pat, pat, pat,
		)
	}
	return query
}

// SearchIssuesInRepo 在仓库内搜索 issue（可按状态过滤），排序与 ListIssues 一致。
func (s *Store) SearchIssuesInRepo(owner, repo, q, state string, limit, offset int) ([]Issue, error) {
	query := s.issueQuery(owner, repo, q, state).Order("pinned DESC, state = 'open' DESC, number DESC")
	if limit > 0 {
		query = query.Limit(limit).Offset(offset)
	}
	var rows []issueRow
	if err := query.Find(&rows).Error; err != nil {
		return nil, err
	}
	issues := []Issue{}
	for _, r := range rows {
		issues = append(issues, issueToDTO(r))
	}
	return issues, nil
}

// CountSearchIssuesInRepo 与 SearchIssuesInRepo 同过滤条件的总数。
func (s *Store) CountSearchIssuesInRepo(owner, repo, q, state string) (int, error) {
	var n int64
	if err := s.issueQuery(owner, repo, q, state).Count(&n).Error; err != nil {
		return 0, err
	}
	return int(n), nil
}

// CountIssues 仓库 issue 总数（与列表口径一致，不含 state 过滤）。
func (s *Store) CountIssues(owner, repo string) (int, error) {
	var n int64
	err := s.db.Model(&issueRow{}).Where("owner = ? AND repo = ?", owner, repo).Count(&n).Error
	return int(n), err
}

func (s *Store) SetIssueState(owner, repo string, number int64, state string) (Issue, error) {
	if state != "open" && state != "closed" {
		return Issue{}, errors.New("invalid state")
	}
	now := now()
	var closedAt any
	if state == "closed" {
		closedAt = now
	}
	res := s.db.Model(&issueRow{}).Where("owner = ? AND repo = ? AND number = ?", owner, repo, number).
		Updates(map[string]any{"state": state, "updated_at": now, "closed_at": closedAt})
	if res.Error != nil {
		return Issue{}, res.Error
	}
	if res.RowsAffected == 0 {
		return Issue{}, ErrNotFound
	}
	return s.getIssue(owner, repo, number)
}

// GetIssue 导出版 getIssue（供 API 层校验 issue 是否存在）。
func (s *Store) GetIssue(owner, repo string, number int64) (Issue, error) {
	return s.getIssue(owner, repo, number)
}

// SetIssuePinned 置顶 / 取消置顶 issue；不存在返回 ErrNotFound。
func (s *Store) SetIssuePinned(owner, repo string, number int64, pinned bool) (Issue, error) {
	res := s.db.Model(&issueRow{}).
		Where("owner = ? AND repo = ? AND number = ?", owner, repo, number).
		Updates(map[string]any{"pinned": pinned, "updated_at": now()})
	if res.Error != nil {
		return Issue{}, res.Error
	}
	return s.getIssue(owner, repo, number)
}

// UpdateIssue 局部更新 issue 标题/正文（nil 表示不修改）；不存在返回 ErrNotFound。
func (s *Store) UpdateIssue(owner, repo string, number int64, title, body *string) (Issue, error) {
	updates := map[string]any{"updated_at": now()}
	if title != nil {
		updates["title"] = *title
	}
	if body != nil {
		updates["body"] = *body
	}
	res := s.db.Model(&issueRow{}).
		Where("owner = ? AND repo = ? AND number = ?", owner, repo, number).
		Updates(updates)
	if res.Error != nil {
		return Issue{}, res.Error
	}
	// RowsAffected==0 可能是“未找到”或“无变化”，由 getIssue 统一判定是否存在。
	return s.getIssue(owner, repo, number)
}

// DeleteIssue 删除 issue，并级联清理其标签、评论、通知与看板卡片关联。
func (s *Store) DeleteIssue(owner, repo string, number int64) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var row issueRow
		if err := tx.Select("id").Where("owner = ? AND repo = ? AND number = ?", owner, repo, number).
			First(&row).Error; err != nil {
			return notFoundErr(err)
		}
		if err := tx.Where("issue_id = ?", row.ID).Delete(&issueLabelRow{}).Error; err != nil {
			return err
		}
		if err := tx.Where("owner = ? AND repo = ? AND kind = ? AND number = ?", owner, repo, "issue", number).
			Delete(&commentRow{}).Error; err != nil {
			return err
		}
		if err := tx.Where("owner = ? AND repo = ? AND number = ?", owner, repo, number).
			Delete(&notificationRow{}).Error; err != nil {
			return err
		}
		if err := tx.Exec(
			"DELETE FROM project_cards WHERE issue_num = ? AND project_id IN (SELECT id FROM projects WHERE owner = ? AND repo = ?)",
			number, owner, repo,
		).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", row.ID).Delete(&issueRow{}).Error
	})
}
