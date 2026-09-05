package store

import (
	"errors"
	"fmt"

	"gorm.io/gorm"
)

// milestoneToDTO 把 ORM 行转换为公共 DTO（计数由聚合查询填充）。
func milestoneToDTO(r milestoneRow) Milestone {
	return Milestone{
		ID: r.ID, Owner: r.Owner, Repo: r.Repo,
		Title: r.Title, Description: r.Description, State: r.State, CreatedAt: r.CreatedAt,
	}
}

func (s *Store) CreateMilestone(owner, repo, title, description string) (Milestone, error) {
	r := milestoneRow{Owner: owner, Repo: repo, Title: title, Description: description, State: "open", CreatedAt: now()}
	if err := s.db.Create(&r).Error; err != nil {
		return Milestone{}, err
	}
	m := milestoneToDTO(r)
	m.State = "open"
	return m, nil
}

func (s *Store) ListMilestones(owner, repo string) ([]Milestone, error) {
	var rows []struct {
		ID           int64
		Owner        string
		Repo         string
		Title        string
		Description  string
		State        string
		CreatedAt    string
		OpenIssues   int
		ClosedIssues int
	}
	// 聚合统计 issue 数量（PG/SQLite 兼容的 CASE 写法）
	err := s.db.Raw(`SELECT m.id, m.owner, m.repo, m.title, m.description, m.state, m.created_at,
		COALESCE(SUM(CASE WHEN i.state = 'open' THEN 1 ELSE 0 END), 0) AS open_issues,
		COALESCE(SUM(CASE WHEN i.state = 'closed' THEN 1 ELSE 0 END), 0) AS closed_issues
		FROM milestones m LEFT JOIN issues i ON i.milestone_id = m.id AND i.owner = m.owner AND i.repo = m.repo
		WHERE m.owner = ? AND m.repo = ? GROUP BY m.id ORDER BY m.title`, owner, repo).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := []Milestone{}
	for _, r := range rows {
		out = append(out, Milestone{
			ID: r.ID, Owner: r.Owner, Repo: r.Repo, Title: r.Title,
			Description: r.Description, State: r.State, CreatedAt: r.CreatedAt,
			OpenIssues: r.OpenIssues, ClosedIssues: r.ClosedIssues,
		})
	}
	return out, nil
}

func (s *Store) UpdateMilestone(owner, repo string, id int64, title, description, state string) (Milestone, error) {
	exists := func() bool {
		var cnt int64
		_ = s.db.Model(&milestoneRow{}).Where("id = ? AND owner = ? AND repo = ?", id, owner, repo).Count(&cnt).Error
		return cnt > 0
	}
	if title != "" {
		if err := s.db.Model(&milestoneRow{}).Where("id = ? AND owner = ? AND repo = ?", id, owner, repo).
			Update("title", title).Error; err != nil {
			return Milestone{}, err
		}
	}
	if description != "" {
		if err := s.db.Model(&milestoneRow{}).Where("id = ? AND owner = ? AND repo = ?", id, owner, repo).
			Update("description", description).Error; err != nil {
			return Milestone{}, err
		}
	}
	if state != "" {
		if err := s.db.Model(&milestoneRow{}).Where("id = ? AND owner = ? AND repo = ?", id, owner, repo).
			Update("state", state).Error; err != nil {
			return Milestone{}, err
		}
	}
	if !exists() {
		return Milestone{}, ErrNotFound
	}
	// 重新读取
	list, err := s.ListMilestones(owner, repo)
	if err != nil {
		return Milestone{}, err
	}
	for _, m := range list {
		if m.ID == id {
			return m, nil
		}
	}
	return Milestone{}, ErrNotFound
}

func (s *Store) DeleteMilestone(owner, repo string, id int64) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&issueRow{}).
			Where("owner = ? AND repo = ? AND milestone_id = ?", owner, repo, id).
			Update("milestone_id", nil).Error; err != nil {
			return err
		}
		res := tx.Where("id = ? AND owner = ? AND repo = ?", id, owner, repo).Delete(&milestoneRow{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrNotFound
		}
		return nil
	})
}

// SetIssueMilestone 设置/清除 issue 里程碑（返回是否属于仓库校验）。
func (s *Store) SetIssueMilestone(owner, repo string, number, milestoneID int64) error {
	var r issueRow
	if err := s.db.Where("owner = ? AND repo = ? AND number = ?", owner, repo, number).
		First(&r).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		return err
	}
	if milestoneID == 0 {
		return s.db.Model(&issueRow{}).Where("id = ?", r.ID).Update("milestone_id", nil).Error
	}
	var cnt int64
	if err := s.db.Model(&milestoneRow{}).Where("id = ? AND owner = ? AND repo = ?", milestoneID, owner, repo).
		Count(&cnt).Error; err != nil || cnt == 0 {
		if err != nil {
			return err
		}
		return fmt.Errorf("milestone does not belong to this repository")
	}
	return s.db.Model(&issueRow{}).Where("id = ?", r.ID).Update("milestone_id", milestoneID).Error
}

// IssueMilestones 返回 issue number -> 所属里程碑（精简字段）。
func (s *Store) IssueMilestones(owner, repo string, numbers []int64) (map[int64]Milestone, error) {
	out := map[int64]Milestone{}
	if len(numbers) == 0 {
		return out, nil
	}
	var rows []struct {
		Number int64
		ID     int64
		Title  string
		State  string
	}
	err := s.db.Raw(`SELECT i.number AS number, m.id AS id, m.title AS title, m.state AS state FROM issues i
		JOIN milestones m ON m.id = i.milestone_id
		WHERE i.owner = ? AND i.repo = ? AND i.milestone_id IS NOT NULL`, owner, repo).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.Number] = Milestone{ID: r.ID, Title: r.Title, State: r.State}
	}
	return out, nil
}
