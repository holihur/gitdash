package store

// ---- user avatars ----

// UserAvatar 用户头像内容。
type UserAvatar struct {
	ContentType string
	Data        []byte
	UpdatedAt   string
}

// SetUserAvatar 保存（或覆盖）用户头像。
func (s *Store) SetUserAvatar(username, contentType string, data []byte) error {
	row := userAvatarRow{Username: username, ContentType: contentType, Data: data, UpdatedAt: now()}
	return s.db.Save(&row).Error
}

// GetUserAvatar 读取用户头像；不存在返回 ErrNotFound。
func (s *Store) GetUserAvatar(username string) (UserAvatar, error) {
	var row userAvatarRow
	if err := s.db.Where("username = ?", username).First(&row).Error; err != nil {
		return UserAvatar{}, notFoundErr(err)
	}
	return UserAvatar{ContentType: row.ContentType, Data: row.Data, UpdatedAt: row.UpdatedAt}, nil
}

// HasUserAvatar 用户是否已设置头像（轻量查询，不加载图片数据）。
func (s *Store) HasUserAvatar(username string) bool {
	var n int64
	if err := s.db.Model(&userAvatarRow{}).Where("username = ?", username).Count(&n).Error; err != nil {
		return false
	}
	return n > 0
}

// DeleteUserAvatar 删除用户头像；不存在返回 ErrNotFound。
func (s *Store) DeleteUserAvatar(username string) error {
	res := s.db.Where("username = ?", username).Delete(&userAvatarRow{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}
