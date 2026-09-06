package store

import (
	"errors"

	"gorm.io/gorm"
)

// Release 仓库 release（对应 git tag 的发布说明与附件）。
type Release struct {
	ID        int64  `json:"id"`
	Owner     string `json:"owner"`
	Repo      string `json:"repo"`
	TagName   string `json:"tag_name"`
	Name      string `json:"name"`
	Body      string `json:"body"` // markdown
	Author    string `json:"author"`
	CreatedAt string `json:"created_at"`
}

// ReleaseAsset release 附件元数据（Content 仅下载端点填充）。
type ReleaseAsset struct {
	ID        int64  `json:"id"`
	Owner     string `json:"-"`
	Repo      string `json:"-"`
	ReleaseID int64  `json:"release_id"`
	Filename  string `json:"filename"`
	Size      int64  `json:"size"`
	Content   []byte `json:"-"`
	CreatedAt string `json:"created_at"`
}

// CreateRelease 创建 release；(owner, repo, tag_name) 唯一，重复返回 ErrExists。
func (s *Store) CreateRelease(rel *Release) error {
	row := releaseRow{
		Owner: rel.Owner, Repo: rel.Repo, TagName: rel.TagName,
		Name: rel.Name, Body: rel.Body, Author: rel.Author, CreatedAt: now(),
	}
	if err := s.db.Create(&row).Error; err != nil {
		if isUniqueErr(err) {
			return ErrExists
		}
		return err
	}
	*rel = Release(row)
	return nil
}

// GetRelease 按 tag 读取 release；不存在返回 ErrNotFound。
func (s *Store) GetRelease(owner, repo, tag string) (Release, error) {
	var row releaseRow
	err := s.db.Where("owner = ? AND repo = ? AND tag_name = ?", owner, repo, tag).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Release{}, ErrNotFound
		}
		return Release{}, err
	}
	return releaseFromRow(row), nil
}

func releaseFromRow(row releaseRow) Release { return Release(row) }

// ListReleases 分页列出 release，返回列表与总数。
func (s *Store) ListReleases(owner, repo string, limit, offset int) ([]Release, int, error) {
	var total int64
	if err := s.db.Model(&releaseRow{}).Where("owner = ? AND repo = ?", owner, repo).
		Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []releaseRow
	if err := s.db.Where("owner = ? AND repo = ?", owner, repo).
		Order("created_at DESC, id DESC").
		Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	out := make([]Release, 0, len(rows))
	for _, row := range rows {
		out = append(out, releaseFromRow(row))
	}
	return out, int(total), nil
}

// DeleteRelease 删除 release 并级联删除其附件。
func (s *Store) DeleteRelease(owner, repo, tag string) error {
	var rel releaseRow
	err := s.db.Select("id").Where("owner = ? AND repo = ? AND tag_name = ?", owner, repo, tag).First(&rel).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		return err
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("release_id = ?", rel.ID).Delete(&releaseAssetRow{}).Error; err != nil {
			return err
		}
		return tx.Delete(&releaseRow{}, rel.ID).Error
	})
}

// CountAssets 统计某 release 的附件数。
func (s *Store) CountAssets(releaseID int64) (int64, error) {
	var n int64
	err := s.db.Model(&releaseAssetRow{}).Where("release_id = ?", releaseID).Count(&n).Error
	return n, err
}

// AddAsset 新增附件（同 release 内文件名重复返回 ErrExists）。
func (s *Store) AddAsset(a *ReleaseAsset) error {
	row := releaseAssetRow{
		Owner: a.Owner, Repo: a.Repo, ReleaseID: a.ReleaseID,
		Filename: a.Filename, Size: a.Size, Content: a.Content, CreatedAt: now(),
	}
	if err := s.db.Create(&row).Error; err != nil {
		if isUniqueErr(err) {
			return ErrExists
		}
		return err
	}
	a.ID = row.ID
	a.CreatedAt = row.CreatedAt
	return nil
}

// ListAssets 列出附件元数据（不含内容）。
func (s *Store) ListAssets(owner, repo string, releaseID int64) ([]ReleaseAsset, error) {
	var rows []releaseAssetRow
	err := s.db.Select("id, owner, repo, release_id, filename, size, created_at").
		Where("owner = ? AND repo = ? AND release_id = ?", owner, repo, releaseID).
		Order("id").Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]ReleaseAsset, 0, len(rows))
	for _, row := range rows {
		out = append(out, ReleaseAsset(row))
	}
	return out, nil
}

// GetAsset 读取单个附件（含内容）。
func (s *Store) GetAsset(owner, repo string, releaseID int64, filename string) (ReleaseAsset, error) {
	var row releaseAssetRow
	err := s.db.Where("owner = ? AND repo = ? AND release_id = ? AND filename = ?",
		owner, repo, releaseID, filename).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ReleaseAsset{}, ErrNotFound
		}
		return ReleaseAsset{}, err
	}
	return ReleaseAsset(row), nil
}

// DeleteAsset 删除附件。
func (s *Store) DeleteAsset(owner, repo string, releaseID int64, filename string) error {
	res := s.db.Where("owner = ? AND repo = ? AND release_id = ? AND filename = ?",
		owner, repo, releaseID, filename).Delete(&releaseAssetRow{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}
