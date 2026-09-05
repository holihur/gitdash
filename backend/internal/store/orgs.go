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

func (s *Store) CreateOrg(name, display, creator string) (Org, error) {
	if _, err := s.GetByUsername(name); err == nil {
		return Org{}, ErrExists // 用户名占用
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

func (s *Store) ListMyOrgs(username string) ([]Org, error) {
	var rows []orgRow
	err := s.db.Table("orgs").
		Select("orgs.*").
		Joins("JOIN org_members ON org_members.org = orgs.name").
		Where("org_members.username = ?", username).
		Order("orgs.name").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := []Org{}
	for _, r := range rows {
		out = append(out, Org(r))
	}
	return out, nil
}

func (s *Store) OrgRole(org, username string) string {
	var row orgMemberRow
	err := s.db.Where("org = ? AND username = ?", org, username).First(&row).Error
	if err != nil {
		return ""
	}
	return row.Role
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

func (s *Store) AddOrgMember(org, username, role string) error {
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
