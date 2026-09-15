package store

import (
	"errors"
	"strings"
)

// TemplateUser 系统专用模版用户：启动时幂等 seed，封禁禁止登录；
// 其名下仓库自动 is_template + 公开，供所有用户作为模版 fork。
const TemplateUser = "template"

// EnsureTemplateUser 幂等引导系统专用模版用户（已存在则确保封禁态）。
func (s *Store) EnsureTemplateUser(passwordHash string) error {
	if _, err := s.GetByUsername(TemplateUser); err == nil {
		return s.SetUserBanned(TemplateUser, true)
	}
	if _, err := s.CreateUser(TemplateUser, passwordHash); err != nil {
		if errors.Is(err, ErrExists) {
			return s.SetUserBanned(TemplateUser, true)
		}
		return err
	}
	return s.SetUserBanned(TemplateUser, true)
}

// ---- 封禁（admin 专用）----
//
// 用户 / 仓库 / 组织三级的 banned 标记。封禁只影响访问与登录，
// 不删除任何数据；解封后数据原样恢复。

// SetUserBanned 切换用户封禁状态。
func (s *Store) SetUserBanned(username string, banned bool) error {
	res := s.db.Model(&userRow{}).Where("username = ?", username).Update("banned", banned)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// IsUserBanned 用户是否被封禁。
func (s *Store) IsUserBanned(username string) bool {
	var n int64
	err := s.db.Model(&userRow{}).Where("username = ? AND banned = ?", username, true).
		Count(&n).Error
	return err == nil && n > 0
}

// SetRepoBanned 切换仓库封禁状态。
func (s *Store) SetRepoBanned(owner, name string, banned bool) error {
	res := s.db.Model(&repoRow{}).Where("owner = ? AND name = ?", owner, name).
		Update("banned", banned)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// IsRepoBanned 仓库（或其所在组织）是否被封禁。
func (s *Store) IsRepoBanned(owner, name string) bool {
	var n int64
	if err := s.db.Model(&repoRow{}).
		Where("owner = ? AND name = ? AND banned = ?", owner, name, true).
		Count(&n).Error; err == nil && n > 0 {
		return true
	}
	return s.IsOrgBanned(owner)
}

// SetOrgBanned 切换组织封禁状态。
func (s *Store) SetOrgBanned(org string, banned bool) error {
	res := s.db.Model(&orgRow{}).Where("name = ?", org).Update("banned", banned)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// IsOrgBanned 组织是否被封禁。
func (s *Store) IsOrgBanned(org string) bool {
	var n int64
	err := s.db.Model(&orgRow{}).Where("name = ? AND banned = ?", org, true).Count(&n).Error
	return err == nil && n > 0
}

// OrgOwners 返回组织的 owner 角色成员（封禁通知用）。
func (s *Store) OrgOwners(org string) []string {
	var out []string
	if err := s.db.Model(&orgMemberRow{}).
		Where("org = ? AND role = ?", org, "owner").Pluck("username", &out).Error; err != nil {
		return nil
	}
	return out
}

// AdminListRepos 管理端仓库列表：q 过滤 owner/name（不区分大小写），limit/offset 分页。
func (s *Store) AdminListRepos(q string, limit, offset int) ([]Repo, int, error) {
	db := s.db.Model(&repoRow{})
	if q = strings.ToLower(strings.TrimSpace(q)); q != "" {
		db = db.Where("LOWER(owner) LIKE ? OR LOWER(name) LIKE ?", "%"+q+"%", "%"+q+"%")
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []repoRow
	if err := db.Order("id DESC").Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	out := make([]Repo, 0, len(rows))
	for _, r := range rows {
		out = append(out, toRepo(r))
	}
	return out, int(total), nil
}

// AdminListOrgs 管理端组织列表：q 过滤名称（不区分大小写），limit/offset 分页。
func (s *Store) AdminListOrgs(q string, limit, offset int) ([]Org, int, error) {
	db := s.db.Model(&orgRow{})
	if q = strings.ToLower(strings.TrimSpace(q)); q != "" {
		db = db.Where("LOWER(name) LIKE ?", "%"+q+"%")
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []orgRow
	if err := db.Order("id ASC").Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	out := make([]Org, 0, len(rows))
	for _, r := range rows {
		out = append(out, Org(r))
	}
	return out, int(total), nil
}
