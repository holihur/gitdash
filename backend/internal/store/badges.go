package store

import (
	"strings"

	"gorm.io/gorm"
)

// MaxDisplayedBadges 每个目标最多挂出的徽章数。
const MaxDisplayedBadges = 3

// Badge 徽章定义（对外 DTO）。
type Badge struct {
	ID             int64  `json:"id"`
	Slug           string `json:"slug"`
	Label          string `json:"label"`
	Description    string `json:"description"`
	HasImage       bool   `json:"has_image"`
	ImageUpdatedAt string `json:"image_updated_at,omitempty"`
	CreatedAt      string `json:"created_at"`
}

// BadgeGrant 授予记录（管理端视图）。
type BadgeGrant struct {
	BadgeID   int64  `json:"badge_id"`
	Label     string `json:"label"`
	Kind      string `json:"kind"`
	Owner     string `json:"owner"`
	Repo      string `json:"repo"`
	CreatedAt string `json:"created_at"`
}

func badgeToDTO(r badgeRow, hasImage bool, imageUpdatedAt string) Badge {
	return Badge{
		ID: r.ID, Slug: r.Slug, Label: r.Label, Description: r.Description,
		HasImage: hasImage, ImageUpdatedAt: imageUpdatedAt, CreatedAt: r.CreatedAt,
	}
}

// badgeImageMetas 批量读取徽章图片元信息（只取 badge_id/updated_at，不加载数据）。
func (s *Store) badgeImageMetas(ids []int64) map[int64]string {
	out := map[int64]string{}
	if len(ids) == 0 {
		return out
	}
	var rows []struct {
		BadgeID   int64
		UpdatedAt string
	}
	_ = s.db.Model(&badgeImageRow{}).Select("badge_id, updated_at").Where("badge_id IN ?", ids).Scan(&rows).Error
	for _, r := range rows {
		out[r.BadgeID] = r.UpdatedAt
	}
	return out
}

// ---- badge definitions ----

// CreateBadge 创建徽章定义；slug 冲突返回 ErrExists。
func (s *Store) CreateBadge(slug, label, description string) (Badge, error) {
	r := badgeRow{Slug: strings.TrimSpace(slug), Label: strings.TrimSpace(label), Description: description, CreatedAt: now()}
	if err := s.db.Create(&r).Error; err != nil {
		if isUniqueErr(err) {
			return Badge{}, ErrExists
		}
		return Badge{}, err
	}
	return badgeToDTO(r, false, ""), nil
}

// BadgeUpdate 徽章部分更新（nil 不修改）。
type BadgeUpdate struct {
	Slug        *string
	Label       *string
	Description *string
}

// UpdateBadge 更新徽章定义；不存在返回 ErrNotFound，slug 冲突返回 ErrExists。
func (s *Store) UpdateBadge(id int64, up BadgeUpdate) (Badge, error) {
	updates := map[string]any{}
	if up.Slug != nil {
		updates["slug"] = strings.TrimSpace(*up.Slug)
	}
	if up.Label != nil {
		updates["label"] = strings.TrimSpace(*up.Label)
	}
	if up.Description != nil {
		updates["description"] = *up.Description
	}
	if len(updates) > 0 {
		res := s.db.Model(&badgeRow{}).Where("id = ?", id).Updates(updates)
		if res.Error != nil {
			if isUniqueErr(res.Error) {
				return Badge{}, ErrExists
			}
			return Badge{}, res.Error
		}
	}
	return s.GetBadge(id)
}

// GetBadge 读取单个徽章。
func (s *Store) GetBadge(id int64) (Badge, error) {
	var r badgeRow
	if err := s.db.Where("id = ?", id).First(&r).Error; err != nil {
		return Badge{}, notFoundErr(err)
	}
	metas := s.badgeImageMetas([]int64{id})
	u, has := metas[id]
	return badgeToDTO(r, has, u), nil
}

// ListBadges 列出全部徽章定义（按创建顺序）。
func (s *Store) ListBadges() ([]Badge, error) {
	var rows []badgeRow
	if err := s.db.Order("id").Find(&rows).Error; err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.ID)
	}
	metas := s.badgeImageMetas(ids)
	out := make([]Badge, 0, len(rows))
	for _, r := range rows {
		u, has := metas[r.ID]
		out = append(out, badgeToDTO(r, has, u))
	}
	return out, nil
}

// DeleteBadge 删除徽章及其实授予 / 展示 / 图片记录。
func (s *Store) DeleteBadge(id int64) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		res := tx.Where("id = ?", id).Delete(&badgeRow{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrNotFound
		}
		if err := tx.Where("badge_id = ?", id).Delete(&badgeGrantRow{}).Error; err != nil {
			return err
		}
		if err := tx.Where("badge_id = ?", id).Delete(&badgeDisplayRow{}).Error; err != nil {
			return err
		}
		return tx.Where("badge_id = ?", id).Delete(&badgeImageRow{}).Error
	})
}

// SetBadgeImage 设置（覆盖）徽章图标。
func (s *Store) SetBadgeImage(id int64, contentType string, data []byte) error {
	var cnt int64
	if err := s.db.Model(&badgeRow{}).Where("id = ?", id).Count(&cnt).Error; err != nil {
		return err
	}
	if cnt == 0 {
		return ErrNotFound
	}
	row := badgeImageRow{BadgeID: id, ContentType: contentType, Data: data, UpdatedAt: now()}
	return s.db.Save(&row).Error
}

// GetBadgeImage 读取徽章图标；不存在返回 ErrNotFound。
func (s *Store) GetBadgeImage(id int64) (string, []byte, error) {
	var row badgeImageRow
	if err := s.db.Where("badge_id = ?", id).First(&row).Error; err != nil {
		return "", nil, notFoundErr(err)
	}
	return row.ContentType, row.Data, nil
}

// ---- grants ----

// BadgeTargetExists 检查授予目标是否真实存在。
func (s *Store) BadgeTargetExists(kind, owner, repo string) bool {
	switch kind {
	case "user":
		var n int64
		return s.db.Model(&userRow{}).Where("username = ?", owner).Count(&n).Error == nil && n > 0
	case "org":
		var n int64
		return s.db.Model(&orgRow{}).Where("name = ?", owner).Count(&n).Error == nil && n > 0
	case "repo":
		var n int64
		return s.db.Model(&repoRow{}).Where("owner = ? AND name = ?", owner, repo).Count(&n).Error == nil && n > 0
	}
	return false
}

// GrantBadge 把徽章授予目标；默认加入展示（未超过上限时）。重复授予返回 ErrExists。
func (s *Store) GrantBadge(badgeID int64, kind, owner, repo string) error {
	var bc int64
	if err := s.db.Model(&badgeRow{}).Where("id = ?", badgeID).Count(&bc).Error; err != nil {
		return err
	}
	if bc == 0 {
		return ErrNotFound
	}
	g := badgeGrantRow{BadgeID: badgeID, Kind: kind, Owner: owner, Repo: repo, CreatedAt: now()}
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&g).Error; err != nil {
			if isUniqueErr(err) {
				return ErrExists
			}
			return err
		}
		var n int64
		if err := tx.Model(&badgeDisplayRow{}).Where("kind = ? AND owner = ? AND repo = ?", kind, owner, repo).
			Count(&n).Error; err != nil {
			return err
		}
		if n < MaxDisplayedBadges {
			var maxPos int
			_ = tx.Model(&badgeDisplayRow{}).Where("kind = ? AND owner = ? AND repo = ?", kind, owner, repo).
				Select("COALESCE(MAX(position), -1)").Scan(&maxPos).Error
			d := badgeDisplayRow{Kind: kind, Owner: owner, Repo: repo, BadgeID: badgeID, Position: maxPos + 1}
			if err := tx.Create(&d).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// RevokeBadge 撤销授予并移除其展示。未授予返回 ErrNotFound。
func (s *Store) RevokeBadge(badgeID int64, kind, owner, repo string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		res := tx.Where("badge_id = ? AND kind = ? AND owner = ? AND repo = ?", badgeID, kind, owner, repo).
			Delete(&badgeGrantRow{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrNotFound
		}
		return tx.Where("badge_id = ? AND kind = ? AND owner = ? AND repo = ?", badgeID, kind, owner, repo).
			Delete(&badgeDisplayRow{}).Error
	})
}

// badgesByIDs 按给定顺序返回徽章（自动补全图片信息）。
func (s *Store) badgesByIDs(ids []int64) ([]Badge, error) {
	out := []Badge{}
	if len(ids) == 0 {
		return out, nil
	}
	var rows []badgeRow
	if err := s.db.Where("id IN ?", ids).Find(&rows).Error; err != nil {
		return nil, err
	}
	byID := map[int64]badgeRow{}
	for _, r := range rows {
		byID[r.ID] = r
	}
	metas := s.badgeImageMetas(ids)
	for _, id := range ids {
		r, ok := byID[id]
		if !ok {
			continue
		}
		u, has := metas[id]
		out = append(out, badgeToDTO(r, has, u))
	}
	return out, nil
}

// GrantedBadges 目标已获得的全部徽章（授予顺序）。
func (s *Store) GrantedBadges(kind, owner, repo string) ([]Badge, error) {
	var ids []int64
	if err := s.db.Model(&badgeGrantRow{}).
		Where("kind = ? AND owner = ? AND repo = ?", kind, owner, repo).
		Order("id").Pluck("badge_id", &ids).Error; err != nil {
		return nil, err
	}
	return s.badgesByIDs(ids)
}

// DisplayedBadges 目标实际挂出的徽章（按 position 顺序）。
func (s *Store) DisplayedBadges(kind, owner, repo string) ([]Badge, error) {
	var rows []badgeDisplayRow
	if err := s.db.Where("kind = ? AND owner = ? AND repo = ?", kind, owner, repo).
		Order("position, id").Find(&rows).Error; err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.BadgeID)
	}
	return s.badgesByIDs(ids)
}

// DisplayedBadgesBatch 批量返回多个目标挂出的徽章，key 为 "owner/repo"（user/org 时 repo 为空）。
// 用于列表卡片 / 搜索结果一次性拉取，避免逐条请求。
//
// targets 按 chunk 分组，每组用行值 IN `(owner, repo) IN ((?,?),...)`，
// 使复合索引 idx_badge_display_target 可用（SQLite 3.15+ / PostgreSQL 均支持）。
func (s *Store) DisplayedBadgesBatch(kind string, targets [][2]string) (map[string][]Badge, error) {
	out := map[string][]Badge{}
	if len(targets) == 0 {
		return out, nil
	}
	const chunkSize = 50
	var rows []badgeDisplayRow
	for start := 0; start < len(targets); start += chunkSize {
		end := min(start+chunkSize, len(targets))
		chunk := targets[start:end]
		placeholders := make([]string, 0, len(chunk))
		args := make([]any, 0, len(chunk)*2)
		for _, t := range chunk {
			placeholders = append(placeholders, "(?, ?)")
			args = append(args, t[0], t[1])
		}
		where := "(owner, repo) IN (" + strings.Join(placeholders, ",") + ")"
		var part []badgeDisplayRow
		if err := s.db.Model(&badgeDisplayRow{}).Where("kind = ?", kind).Where(where, args...).
			Order("position, id").Find(&part).Error; err != nil {
			return nil, err
		}
		rows = append(rows, part...)
	}
	if len(rows) == 0 {
		return out, nil
	}
	ids := make([]int64, 0, len(rows))
	seen := map[int64]bool{}
	for _, r := range rows {
		if !seen[r.BadgeID] {
			seen[r.BadgeID] = true
			ids = append(ids, r.BadgeID)
		}
	}
	var brows []badgeRow
	if err := s.db.Where("id IN ?", ids).Find(&brows).Error; err != nil {
		return nil, err
	}
	byID := map[int64]badgeRow{}
	for _, b := range brows {
		byID[b.ID] = b
	}
	metas := s.badgeImageMetas(ids)
	for _, r := range rows {
		b, ok := byID[r.BadgeID]
		if !ok {
			continue
		}
		u, has := metas[r.BadgeID]
		key := r.Owner + "/" + r.Repo
		out[key] = append(out[key], badgeToDTO(b, has, u))
	}
	return out, nil
}

// SetDisplayedBadges 全量替换目标挂出的徽章。仅接受已授予的徽章，去重并截断到上限。
func (s *Store) SetDisplayedBadges(kind, owner, repo string, badgeIDs []int64) error {
	var granted []int64
	if err := s.db.Model(&badgeGrantRow{}).
		Where("kind = ? AND owner = ? AND repo = ?", kind, owner, repo).
		Pluck("badge_id", &granted).Error; err != nil {
		return err
	}
	gset := map[int64]bool{}
	for _, id := range granted {
		gset[id] = true
	}
	seen := map[int64]bool{}
	clean := make([]int64, 0, len(badgeIDs))
	for _, id := range badgeIDs {
		if !gset[id] || seen[id] {
			continue
		}
		seen[id] = true
		clean = append(clean, id)
		if len(clean) >= MaxDisplayedBadges {
			break
		}
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("kind = ? AND owner = ? AND repo = ?", kind, owner, repo).
			Delete(&badgeDisplayRow{}).Error; err != nil {
			return err
		}
		if len(clean) == 0 {
			return nil
		}
		rows := make([]badgeDisplayRow, 0, len(clean))
		for i, id := range clean {
			rows = append(rows, badgeDisplayRow{Kind: kind, Owner: owner, Repo: repo, BadgeID: id, Position: i})
		}
		return tx.Create(&rows).Error
	})
}

// ListBadgeGrants 列出某徽章的全部授予记录。
func (s *Store) ListBadgeGrants(badgeID int64) ([]BadgeGrant, error) {
	var rows []badgeGrantRow
	if err := s.db.Where("badge_id = ?", badgeID).Order("id").Find(&rows).Error; err != nil {
		return nil, err
	}
	label := ""
	var b badgeRow
	if err := s.db.Select("label").Where("id = ?", badgeID).First(&b).Error; err == nil {
		label = b.Label
	}
	out := make([]BadgeGrant, 0, len(rows))
	for _, r := range rows {
		out = append(out, BadgeGrant{
			BadgeID: r.BadgeID, Label: label, Kind: r.Kind, Owner: r.Owner, Repo: r.Repo, CreatedAt: r.CreatedAt,
		})
	}
	return out, nil
}
