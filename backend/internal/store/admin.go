package store

import (
	"time"

	"gorm.io/gorm/clause"
)

func (s *Store) AdminCount() (int, error) {
	var n int64
	err := s.db.Model(&adminUserRow{}).Count(&n).Error
	return int(n), err
}

func (s *Store) CreateAdminUser(username, passwordHash string) error {
	row := adminUserRow{Username: username, PasswordHash: passwordHash, CreatedAt: now()}
	err := s.db.Create(&row).Error
	if err != nil && isUniqueErr(err) {
		return ErrExists
	}
	return err
}

func (s *Store) AdminAuth(username string) (int64, string, error) {
	var row adminUserRow
	err := s.db.Where("username = ?", username).First(&row).Error
	if err != nil {
		return 0, "", notFoundErr(err)
	}
	return row.ID, row.PasswordHash, nil
}

func (s *Store) UpdateAdminPassword(username, passwordHash string) error {
	res := s.db.Model(&adminUserRow{}).Where("username = ?", username).
		Update("password_hash", passwordHash)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) CreateAdminSession(token string, adminID int64) error {
	row := adminSessionRow{
		Token:     token,
		AdminID:   adminID,
		CreatedAt: now(),
		ExpiresAt: time.Now().Add(12 * time.Hour).UTC().Format(time.RFC3339),
	}
	return s.db.Create(&row).Error
}

func (s *Store) GetAdminSession(token string) (int64, string, error) {
	var dest struct {
		ID       int64
		Username string
	}
	err := s.db.Table("admin_sessions").
		Select("admin_users.id, admin_users.username").
		Joins("JOIN admin_users ON admin_users.id = admin_sessions.admin_id").
		Where("admin_sessions.token = ? AND admin_sessions.expires_at > ?", token, now()).
		Scan(&dest).Error
	if err != nil {
		return 0, "", err
	}
	if dest.ID == 0 && dest.Username == "" {
		return 0, "", ErrNotFound
	}
	return dest.ID, dest.Username, nil
}

func (s *Store) DeleteAdminSession(token string) error {
	return s.db.Where("token = ?", token).Delete(&adminSessionRow{}).Error
}

func (s *Store) GetSetting(key string) string {
	var row settingRow
	if err := s.db.Where("\"key\" = ?", key).First(&row).Error; err != nil {
		return ""
	}
	return row.Value
}

func (s *Store) SetSetting(key, value string) error {
	row := settingRow{Key: key, Value: value}
	return s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value"}),
	}).Create(&row).Error
}

// PutOAuthState 持久化 OAuth/OIDC state（重启 / 多实例下登录回调仍可校验）。
func (s *Store) PutOAuthState(state, expiresAt string) error {
	return s.SetSetting("oauth_state:"+state, expiresAt)
}

// TakeOAuthState 取出并删除 state（一次性）；不存在或已过期返回 false。
func (s *Store) TakeOAuthState(state, nowStr string) (bool, error) {
	key := "oauth_state:" + state
	v := s.GetSetting(key)
	if v == "" {
		return false, nil
	}
	if err := s.db.Where("\"key\" = ?", key).Delete(&settingRow{}).Error; err != nil {
		return false, err
	}
	return v > nowStr, nil
}

// PruneOAuthStates 删除已过期的 OAuth state，返回清理条数。
func (s *Store) PruneOAuthStates(nowStr string) (int64, error) {
	res := s.db.Where("\"key\" LIKE 'oauth_state:%' AND value < ?", nowStr).Delete(&settingRow{})
	return res.RowsAffected, res.Error
}
