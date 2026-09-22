package store

// ---- user covers ----

// UserCover 用户封面（个人主页顶部横幅）内容。
type UserCover struct {
	ContentType string
	Data        []byte
	UpdatedAt   string
}

// SetUserCover 保存（或覆盖）用户封面。
func (s *Store) SetUserCover(username, contentType string, data []byte) error {
	row := userCoverRow{Username: username, ContentType: contentType, Data: data, UpdatedAt: now()}
	return s.db.Save(&row).Error
}

// GetUserCover 读取用户封面；不存在返回 ErrNotFound。
func (s *Store) GetUserCover(username string) (UserCover, error) {
	var row userCoverRow
	if err := s.db.Where("username = ?", username).First(&row).Error; err != nil {
		return UserCover{}, notFoundErr(err)
	}
	return UserCover{ContentType: row.ContentType, Data: row.Data, UpdatedAt: row.UpdatedAt}, nil
}

// HasUserCover 用户是否已设置封面（轻量查询，不加载图片数据）。
func (s *Store) HasUserCover(username string) bool {
	var n int64
	if err := s.db.Model(&userCoverRow{}).Where("username = ?", username).Count(&n).Error; err != nil {
		return false
	}
	return n > 0
}

// DeleteUserCover 删除用户封面；不存在返回 ErrNotFound。
func (s *Store) DeleteUserCover(username string) error {
	res := s.db.Where("username = ?", username).Delete(&userCoverRow{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- org covers ----

// OrgCover 组织封面（组织主页顶部横幅）内容。
type OrgCover struct {
	ContentType string
	Data        []byte
	UpdatedAt   string
}

// SetOrgCover 保存（或覆盖）组织封面。
func (s *Store) SetOrgCover(org, contentType string, data []byte) error {
	row := orgCoverRow{Org: org, ContentType: contentType, Data: data, UpdatedAt: now()}
	return s.db.Save(&row).Error
}

// GetOrgCover 读取组织封面；不存在返回 ErrNotFound。
func (s *Store) GetOrgCover(org string) (OrgCover, error) {
	var row orgCoverRow
	if err := s.db.Where("org = ?", org).First(&row).Error; err != nil {
		return OrgCover{}, notFoundErr(err)
	}
	return OrgCover{ContentType: row.ContentType, Data: row.Data, UpdatedAt: row.UpdatedAt}, nil
}

// HasOrgCover 组织是否已设置封面（轻量查询，不加载图片数据）。
func (s *Store) HasOrgCover(org string) bool {
	var n int64
	if err := s.db.Model(&orgCoverRow{}).Where("org = ?", org).Count(&n).Error; err != nil {
		return false
	}
	return n > 0
}

// DeleteOrgCover 删除组织封面；不存在返回 ErrNotFound。
func (s *Store) DeleteOrgCover(org string) error {
	res := s.db.Where("org = ?", org).Delete(&orgCoverRow{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}
