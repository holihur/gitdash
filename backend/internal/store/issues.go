package store

import (
	"errors"
)

// issueToDTO 把 ORM 行转换为公共 DTO。
func issueToDTO(r issueRow) Issue {
	it := Issue{
		ID: r.ID, Owner: r.Owner, Repo: r.Repo, Number: r.Number,
		Title: r.Title, Body: r.Body, State: r.State, Author: r.Author,
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
	var it Issue
	var err error
	now := now()
	// 号码在同一仓库内递增；并发冲突时重试（UNIQUE(owner, repo, number)）
	for attempt := 0; attempt < 5; attempt++ {
		var max int64
		if e := s.db.Raw(`SELECT COALESCE(MAX(number), 0) + 1 FROM issues WHERE owner = ? AND repo = ?`,
			owner, repo).Scan(&max).Error; e != nil {
			return it, e
		}
		r := issueRow{
			Owner: owner, Repo: repo, Number: max,
			Title: title, Body: body, State: "open", Author: author,
			CreatedAt: now, UpdatedAt: now,
		}
		e := s.db.Create(&r).Error
		if e == nil {
			return Issue{ID: r.ID, Owner: owner, Repo: repo, Number: r.Number,
				Title: title, Body: body, State: "open", Author: author,
				CreatedAt: now, UpdatedAt: now}, nil
		}
		if !isUniqueErr(e) {
			return it, e
		}
		err = e
	}
	return it, err
}

// ListIssues 分页列出 issue；limit<=0 表示不限制。
func (s *Store) ListIssues(owner, repo string, limit, offset int) ([]Issue, error) {
	q := s.db.Model(&issueRow{}).Where("owner = ? AND repo = ?", owner, repo).
		Order("state = 'open' DESC, number DESC")
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
