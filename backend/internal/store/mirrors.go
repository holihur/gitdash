package store

import (
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (s *Store) SetForkSource(owner, repo, sourceOwner, sourceRepo string) error {
	row := forkRow{Owner: owner, Repo: repo, SourceOwner: sourceOwner, SourceRepo: sourceRepo, CreatedAt: now()}
	return s.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "owner"}, {Name: "repo"}},
		DoUpdates: clause.Assignments(map[string]any{
			"source_owner": sourceOwner,
			"source_repo":  sourceRepo,
		}),
	}).Create(&row).Error
}

// ForkSource 返回 fork 来源；非 fork 仓库返回空串。
func (s *Store) ForkSource(owner, repo string) (string, string, error) {
	var row forkRow
	err := s.db.Select("source_owner, source_repo").Where("owner = ? AND repo = ?", owner, repo).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", "", nil
		}
		return "", "", err
	}
	return row.SourceOwner, row.SourceRepo, nil
}

// ---- imports ----

// SetImportSource 记录仓库的导入来源 URL。
func (s *Store) SetImportSource(owner, repo, url string) error {
	row := importRow{Owner: owner, Repo: repo, SourceURL: url, CreatedAt: now()}
	return s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "owner"}, {Name: "repo"}},
		DoUpdates: clause.Assignments(map[string]any{"source_url": url}),
	}).Create(&row).Error
}

// ImportSource 返回导入来源 URL；非导入仓库返回空串。
func (s *Store) ImportSource(owner, repo string) (string, error) {
	var row importRow
	err := s.db.Select("source_url").Where("owner = ? AND repo = ?", owner, repo).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", nil
		}
		return "", err
	}
	return row.SourceURL, nil
}

// ---- mirrors ----

// SetMirror 配置仓库的 push 镜像目标（覆盖式）。
func (s *Store) SetMirror(owner, repo, url, privateKey string) error {
	row := mirrorRow{Owner: owner, Repo: repo, URL: url, PrivateKey: privateKey, CreatedAt: now()}
	return s.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "owner"}, {Name: "repo"}},
		DoUpdates: clause.Assignments(map[string]any{
			"url":         url,
			"private_key": privateKey,
		}),
	}).Create(&row).Error
}

// GetMirror 返回镜像配置；未配置返回空 URL。
func (s *Store) GetMirror(owner, repo string) (Mirror, error) {
	var row mirrorRow
	err := s.db.Where("owner = ? AND repo = ?", owner, repo).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Mirror{Owner: owner, Repo: repo}, nil
		}
		return Mirror{}, err
	}
	return Mirror(row), nil
}

// DeleteMirror 移除镜像配置。
func (s *Store) DeleteMirror(owner, repo string) error {
	return s.db.Where("owner = ? AND repo = ?", owner, repo).Delete(&mirrorRow{}).Error
}

// ---- orgs (namespace) ----
