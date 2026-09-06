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

// SetImportStatus 更新导入任务状态；errMsg 仅 failed 时非空。
func (s *Store) SetImportStatus(owner, repo, status, errMsg string) error {
	res := s.db.Model(&importRow{}).Where("owner = ? AND repo = ?", owner, repo).
		Updates(map[string]any{"status": status, "error": errMsg})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// ImportStatus 返回导入任务状态与最近失败原因；非导入仓库返回空串。
func (s *Store) ImportStatus(owner, repo string) (string, string, error) {
	var row importRow
	err := s.db.Select("status", "\"error\"").Where("owner = ? AND repo = ?", owner, repo).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", "", nil
		}
		return "", "", err
	}
	return row.Status, row.Error, nil
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

// SetMirrorStatus 更新镜像同步任务状态；errMsg 仅 failed 时非空。
func (s *Store) SetMirrorStatus(owner, repo, status, errMsg string) error {
	res := s.db.Model(&mirrorRow{}).Where("owner = ? AND repo = ?", owner, repo).
		Updates(map[string]any{"status": status, "error": errMsg})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// PendingImports 返回卡在 queued/running 的导入任务（启动续跑用）。
func (s *Store) PendingImports() ([]importRow, error) {
	var rows []importRow
	err := s.db.Where("status IN ?", []string{"queued", "running"}).Find(&rows).Error
	return rows, err
}

// PendingMirrors 返回卡在 queued/running 的镜像同步任务（启动续跑用）。
func (s *Store) PendingMirrors() ([]mirrorRow, error) {
	var rows []mirrorRow
	err := s.db.Where("status IN ?", []string{"queued", "running"}).Find(&rows).Error
	return rows, err
}

// ---- orgs (namespace) ----
