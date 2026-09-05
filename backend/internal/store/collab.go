package store

import (
	"gorm.io/gorm/clause"
)

func (s *Store) UpsertCollab(owner, repo, username, permission string) error {
	row := collabRow{Owner: owner, Repo: repo, Username: username, Permission: permission, CreatedAt: now()}
	return s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "owner"}, {Name: "repo"}, {Name: "username"}},
		DoUpdates: clause.Assignments(map[string]any{"permission": permission}),
	}).Create(&row).Error
}

func (s *Store) ListCollabs(owner, repo string) ([]Collab, error) {
	var rows []collabRow
	if err := s.db.Where("owner = ? AND repo = ?", owner, repo).Order("username").Find(&rows).Error; err != nil {
		return nil, err
	}
	collabs := []Collab{}
	for _, r := range rows {
		collabs = append(collabs, Collab(r))
	}
	return collabs, nil
}

func (s *Store) RemoveCollab(owner, repo, username string) error {
	res := s.db.Where("owner = ? AND repo = ? AND username = ?", owner, repo, username).Delete(&collabRow{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- webhooks ----
