package store

import (
	"fmt"

	"gorm.io/gorm"
)

func (s *Store) CreateLabel(owner, repo, name, color string) (Label, error) {
	r := repoLabelRow{Owner: owner, Repo: repo, Name: name, Color: color, CreatedAt: now()}
	if err := s.db.Create(&r).Error; err != nil {
		if isUniqueErr(err) {
			return Label{}, ErrExists
		}
		return Label{}, err
	}
	return Label{ID: r.ID, Owner: owner, Repo: repo, Name: name, Color: color, CreatedAt: r.CreatedAt}, nil
}

func (s *Store) ListLabels(owner, repo string) ([]Label, error) {
	var rows []repoLabelRow
	if err := s.db.Where("owner = ? AND repo = ?", owner, repo).Order("name").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := []Label{}
	for _, r := range rows {
		out = append(out, Label(r))
	}
	return out, nil
}

func (s *Store) UpdateLabel(owner, repo string, id int64, name, color string) (Label, error) {
	res := s.db.Model(&repoLabelRow{}).Where("id = ? AND owner = ? AND repo = ?", id, owner, repo).
		Updates(map[string]any{"name": name, "color": color})
	if res.Error != nil {
		return Label{}, res.Error
	}
	if res.RowsAffected == 0 {
		return Label{}, ErrNotFound
	}
	var r repoLabelRow
	if err := s.db.First(&r, id).Error; err != nil {
		return Label{}, notFoundErr(err)
	}
	return Label(r), nil
}

func (s *Store) DeleteLabel(owner, repo string, id int64) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("label_id = ? AND issue_id IN (?)", id,
			tx.Model(&issueRow{}).Select("id").Where("owner = ? AND repo = ?", owner, repo)).
			Delete(&issueLabelRow{}).Error; err != nil {
			return err
		}
		res := tx.Where("id = ? AND owner = ? AND repo = ?", id, owner, repo).Delete(&repoLabelRow{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrNotFound
		}
		return nil
	})
}

// SetIssueLabels 全量替换 issue 标签（校验标签属于该仓库）。
func (s *Store) SetIssueLabels(owner, repo string, number int64, labelIDs []int64) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var r issueRow
		if err := tx.Where("owner = ? AND repo = ? AND number = ?", owner, repo, number).
			First(&r).Error; err != nil {
			return notFoundErr(err)
		}
		for _, id := range labelIDs {
			var cnt int64
			if err := tx.Model(&repoLabelRow{}).Where("id = ? AND owner = ? AND repo = ?", id, owner, repo).
				Count(&cnt).Error; err != nil || cnt == 0 {
				if err != nil {
					return err
				}
				return fmt.Errorf("label %d does not belong to this repository", id)
			}
		}
		if err := tx.Where("issue_id = ?", r.ID).Delete(&issueLabelRow{}).Error; err != nil {
			return err
		}
		for _, id := range labelIDs {
			if err := tx.Create(&issueLabelRow{IssueID: r.ID, LabelID: id}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// IssueLabels 返回若干 issue（number）的标签映射。
func (s *Store) IssueLabels(owner, repo string, numbers []int64) (map[int64][]Label, error) {
	out := map[int64][]Label{}
	if len(numbers) == 0 {
		return out, nil
	}
	var rows []struct {
		Number int64
		ID     int64
		Name   string
		Color  string
	}
	err := s.db.Raw(`SELECT i.number AS number, l.id AS id, l.name AS name, l.color AS color
		FROM issue_labels il
		JOIN issues i ON i.id = il.issue_id
		JOIN repo_labels l ON l.id = il.label_id
		WHERE i.owner = ? AND i.repo = ?`, owner, repo).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.Number] = append(out[r.Number], Label{ID: r.ID, Name: r.Name, Color: r.Color})
	}
	return out, nil
}
