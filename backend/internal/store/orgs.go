package store

import (
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (s *Store) IsOrg(name string) bool {
	var cnt int64
	err := s.db.Model(&orgRow{}).Where("name = ?", name).Count(&cnt).Error
	return err == nil && cnt > 0
}

// GetOrg 读取组织资料；不存在返回 ErrNotFound。
func (s *Store) GetOrg(name string) (Org, error) {
	var row orgRow
	if err := s.db.Where("name = ?", name).First(&row).Error; err != nil {
		return Org{}, notFoundErr(err)
	}
	return Org(row), nil
}

// SetOrgInfo 更新组织显示名与简介（仅 owner 调用）。
func (s *Store) SetOrgInfo(name, display, bio string) error {
	res := s.db.Model(&orgRow{}).Where("name = ?", name).
		Updates(map[string]any{"display": display, "bio": bio})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) CreateOrg(name, display, creator string) (Org, error) {
	if _, err := s.GetByUsername(name); err == nil {
		return Org{}, ErrExists // 用户名占用
	}
	createMu.Lock()
	defer createMu.Unlock()
	if err := s.checkOrgQuota(creator); err != nil {
		return Org{}, err
	}
	o := Org{Name: name, Display: display, CreatedAt: now()}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		row := orgRow{Name: name, Display: display, CreatedAt: o.CreatedAt}
		if err := tx.Create(&row).Error; err != nil {
			if isUniqueErr(err) {
				return ErrExists
			}
			return err
		}
		o.ID = row.ID
		return tx.Create(&orgMemberRow{Org: name, Username: creator, Role: "owner", CreatedAt: now()}).Error
	})
	if err != nil {
		return o, err
	}
	return o, nil
}

// ListMyOrgs 我所属的组织（分页）；limit<=0 表示不限制。
func (s *Store) ListMyOrgs(username string, limit, offset int) ([]Org, error) {
	var rows []orgRow
	q := s.db.Table("orgs").
		Select("orgs.*").
		Joins("JOIN org_members ON org_members.org = orgs.name").
		Where("org_members.username = ? AND orgs.banned = ?", username, false).
		Order("orgs.name")
	err := paginate(q, limit, offset).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := []Org{}
	for _, r := range rows {
		out = append(out, Org(r))
	}
	return out, nil
}

// CountMyOrgs 我所属的组织总数。
func (s *Store) CountMyOrgs(username string) (int, error) {
	var n int64
	err := s.db.Table("org_members").
		Joins("JOIN orgs ON orgs.name = org_members.org").
		Where("org_members.username = ? AND orgs.banned = ?", username, false).
		Count(&n).Error
	return int(n), err
}

func (s *Store) OrgRole(org, username string) string {
	var row orgMemberRow
	err := s.db.Where("org = ? AND username = ?", org, username).First(&row).Error
	if err != nil {
		return ""
	}
	return row.Role
}

// MyOwnedOrgs 用户拥有 owner 角色的组织名列表（runner 管理范围判定用）。
func (s *Store) MyOwnedOrgs(username string) ([]string, error) {
	var orgs []string
	err := s.db.Model(&orgMemberRow{}).Where("username = ? AND role = ?", username, "owner").
		Order("org").Pluck("org", &orgs).Error
	return orgs, err
}

func (s *Store) OrgMembers(org string) ([]OrgMember, error) {
	var rows []orgMemberRow
	err := s.db.Where("org = ?", org).Order("username").Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := []OrgMember{}
	for _, r := range rows {
		out = append(out, OrgMember{Org: r.Org, Username: r.Username, Role: r.Role})
	}
	return out, nil
}

// ListOrgMembers 组织成员（分页，按用户名排序）。
func (s *Store) ListOrgMembers(org string, limit, offset int) ([]OrgMember, error) {
	var rows []orgMemberRow
	err := paginate(s.db.Where("org = ?", org).Order("username"), limit, offset).Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := []OrgMember{}
	for _, r := range rows {
		out = append(out, OrgMember{Org: r.Org, Username: r.Username, Role: r.Role})
	}
	return out, nil
}

// CountOrgMembers 组织成员总数。
func (s *Store) CountOrgMembers(org string) (int, error) {
	var n int64
	err := s.db.Model(&orgMemberRow{}).Where("org = ?", org).Count(&n).Error
	return int(n), err
}

func (s *Store) AddOrgMember(org, username, role string) error {
	createMu.Lock()
	defer createMu.Unlock()
	// 已存在只改角色，不占用新名额
	var existing int64
	if err := s.db.Model(&orgMemberRow{}).Where("org = ? AND username = ?", org, username).Count(&existing).Error; err != nil {
		return err
	}
	if existing == 0 {
		if err := s.checkOrgMemberQuota(org); err != nil {
			return err
		}
	}
	row := orgMemberRow{Org: org, Username: username, Role: role, CreatedAt: now()}
	return s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "org"}, {Name: "username"}},
		DoUpdates: clause.AssignmentColumns([]string{"role"}),
	}).Create(&row).Error
}

func (s *Store) RemoveOrgMember(org, username string) error {
	res := s.db.Where("org = ? AND username = ?", org, username).Delete(&orgMemberRow{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteOrg(org string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var cnt int64
		if err := tx.Model(&repoRow{}).Where("owner = ?", org).Count(&cnt).Error; err != nil {
			return err
		}
		if cnt > 0 {
			return errors.New("org not empty")
		}
		if err := tx.Where("org = ?", org).Delete(&orgMemberRow{}).Error; err != nil {
			return err
		}
		return tx.Where("name = ?", org).Delete(&orgRow{}).Error
	})
}
