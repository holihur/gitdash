package store

import (
	"errors"
	"strconv"
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
	if r.StateReason != nil {
		v := *r.StateReason
		it.StateReason = &v
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

// IssueFilter 构造 issue 查询的可选过滤 / 排序条件。
// Label/Assignee 取值：空 = 不过滤；"none" = 未设置；否则为 id / 用户名。
type IssueFilter struct {
	Label    string
	Assignee string
	Sort     string // newest（默认）| oldest | updated | popular
}

// issueQuery 构造仓库 issue 查询（可选关键词、状态、里程碑、标签与负责人过滤）。
// 关键词命中标题 / 正文 / 作者（大小写不敏感）。
// milestone 取值："" 不过滤；"none" 仅未指派里程碑；数字字符串按里程碑 id 过滤。
func (s *Store) issueQuery(owner, repo, q, state, milestone string, opts ...IssueFilter) *gorm.DB {
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
	switch {
	case milestone == "none":
		query = query.Where("milestone_id IS NULL")
	case milestone != "":
		if id, err := strconv.ParseInt(milestone, 10, 64); err == nil {
			query = query.Where("milestone_id = ?", id)
		} else {
			// 非法值不应匹配任何行，而不是被忽略而返回全部。
			query = query.Where("1 = 0")
		}
	}
	if len(opts) > 0 {
		f := opts[0]
		switch {
		case f.Label == "none":
			query = query.Where("id NOT IN (SELECT issue_id FROM issue_labels)")
		case f.Label != "":
			if id, err := strconv.ParseInt(f.Label, 10, 64); err == nil && id > 0 {
				query = query.Where("id IN (SELECT issue_id FROM issue_labels WHERE label_id = ?)", id)
			} else {
				query = query.Where("1 = 0")
			}
		}
		switch {
		case f.Assignee == "none":
			query = query.Where("id NOT IN (SELECT issue_id FROM issue_assignees)")
		case f.Assignee != "":
			query = query.Where("id IN (SELECT issue_id FROM issue_assignees WHERE username = ?)", f.Assignee)
		}
	}
	return query
}

// issueOrder 根据排序选项返回稳定排序。默认与旧行为一致（置顶 > open > 编号倒序）。
func issueOrder(sort string) string {
	base := "pinned DESC, state = 'open' DESC, "
	switch sort {
	case "oldest":
		return base + "number ASC"
	case "updated":
		return base + "updated_at DESC, number DESC"
	case "popular":
		return base + "(SELECT COUNT(*) FROM issue_comments c WHERE c.owner = issues.owner AND c.repo = issues.repo AND c.kind = 'issue' AND c.number = issues.number) DESC, number DESC"
	default:
		return base + "number DESC"
	}
}

// SearchIssuesInRepo 在仓库内搜索 issue（可按状态、里程碑、标签、负责人过滤并排序）。
func (s *Store) SearchIssuesInRepo(owner, repo, q, state, milestone string, limit, offset int, opts ...IssueFilter) ([]Issue, error) {
	sort := ""
	if len(opts) > 0 {
		sort = opts[0].Sort
	}
	query := s.issueQuery(owner, repo, q, state, milestone, opts...).Order(issueOrder(sort))
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
func (s *Store) CountSearchIssuesInRepo(owner, repo, q, state, milestone string, opts ...IssueFilter) (int, error) {
	var n int64
	if err := s.issueQuery(owner, repo, q, state, milestone, opts...).Count(&n).Error; err != nil {
		return 0, err
	}
	return int(n), nil
}

// CountIssueStates 返回同过滤条件下 open / closed 的真实数量（不受分页影响）。
func (s *Store) CountIssueStates(owner, repo, q, milestone, label, assignee string) (open, closed int, err error) {
	filter := []IssueFilter{{Label: label, Assignee: assignee}}
	var o, c int64
	if err = s.issueQuery(owner, repo, q, "open", milestone, filter...).Count(&o).Error; err != nil {
		return 0, 0, err
	}
	if err = s.issueQuery(owner, repo, q, "closed", milestone, filter...).Count(&c).Error; err != nil {
		return 0, 0, err
	}
	return int(o), int(c), nil
}

// CountIssues 仓库 issue 总数（与列表口径一致，不含 state 过滤）。
func (s *Store) CountIssues(owner, repo string) (int, error) {
	var n int64
	err := s.db.Model(&issueRow{}).Where("owner = ? AND repo = ?", owner, repo).Count(&n).Error
	return int(n), err
}

func (s *Store) SetIssueState(owner, repo string, number int64, state string) (Issue, error) {
	return s.SetIssueStateWithReason(owner, repo, number, state, "")
}

// SetIssueStateWithReason 与 SetIssueState 相同，但可同时记录关闭原因
// （completed | not_planned）；state 为 open 时清空原因。
func (s *Store) SetIssueStateWithReason(owner, repo string, number int64, state, reason string) (Issue, error) {
	if state != "open" && state != "closed" {
		return Issue{}, errors.New("invalid state")
	}
	now := now()
	updates := map[string]any{"state": state, "updated_at": now}
	if state == "closed" {
		updates["closed_at"] = now
		if reason == "not_planned" {
			updates["state_reason"] = "not_planned"
		} else if reason == "completed" {
			updates["state_reason"] = "completed"
		} else {
			updates["state_reason"] = "completed"
		}
	} else {
		updates["closed_at"] = nil
		updates["state_reason"] = nil
	}
	res := s.db.Model(&issueRow{}).Where("owner = ? AND repo = ? AND number = ?", owner, repo, number).
		Updates(updates)
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
