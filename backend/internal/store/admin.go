package store

import (
	"encoding/json"
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

// ---- MFA 二次验证 challenge（持久化：重启 / 多实例下 mfa-verify 仍有效）----

type mfaChallengeData struct {
	Username string `json:"u"`
	Expires  string `json:"e"`
	Attempts int    `json:"a"`
}

// PutMFAChallenge 写入待二次验证的登录 challenge。
func (s *Store) PutMFAChallenge(token, username, expiresAt string) error {
	v, err := json.Marshal(mfaChallengeData{Username: username, Expires: expiresAt})
	if err != nil {
		return err
	}
	return s.SetSetting("mfa_challenge:"+token, string(v))
}

// GetMFAChallenge 读取 challenge（不消费）；不存在返回 ErrNotFound。
func (s *Store) GetMFAChallenge(token string) (username, expiresAt string, attempts int, err error) {
	v := s.GetSetting("mfa_challenge:" + token)
	if v == "" {
		return "", "", 0, ErrNotFound
	}
	var d mfaChallengeData
	if err := json.Unmarshal([]byte(v), &d); err != nil {
		return "", "", 0, err
	}
	return d.Username, d.Expires, d.Attempts, nil
}

// SaveMFAChallenge 更新 challenge 的失败尝试次数。
func (s *Store) SaveMFAChallenge(token, username, expiresAt string, attempts int) error {
	v, err := json.Marshal(mfaChallengeData{Username: username, Expires: expiresAt, Attempts: attempts})
	if err != nil {
		return err
	}
	return s.SetSetting("mfa_challenge:"+token, string(v))
}

// DeleteMFAChallenge 删除 challenge（校验通过 / 过期 / 尝试超限时调用）。
func (s *Store) DeleteMFAChallenge(token string) error {
	return s.db.Where("\"key\" = ?", "mfa_challenge:"+token).Delete(&settingRow{}).Error
}

// PruneMFAChallenges 删除已过期的 MFA challenge，返回清理条数。
func (s *Store) PruneMFAChallenges(nowStr string) (int64, error) {
	// settings 表的 value 是 JSON（含 "e":"<expires>"），过期判断需在应用层做
	var rows []settingRow
	if err := s.db.Where("\"key\" LIKE 'mfa_challenge:%' OR \"key\" LIKE 'email_mfa:%'").Find(&rows).Error; err != nil {
		return 0, err
	}
	var pruned int64
	for _, row := range rows {
		var d mfaChallengeData
		if json.Unmarshal([]byte(row.Value), &d) != nil || d.Expires <= nowStr {
			if err := s.db.Where("\"key\" = ?", row.Key).Delete(&settingRow{}).Error; err != nil {
				return pruned, err
			}
			pruned++
		}
	}
	return pruned, nil
}

// ---- email MFA 验证码（6 位邮箱验证码，key 为 email_mfa:<username 或 mfa_token>）----

type emailMFACodeData struct {
	Code    string `json:"c"`
	Expires string `json:"e"`
}

// PutEmailMFACode 写入（或覆盖）邮箱 MFA 验证码。
func (s *Store) PutEmailMFACode(key, code, expiresAt string) error {
	v, err := json.Marshal(emailMFACodeData{Code: code, Expires: expiresAt})
	if err != nil {
		return err
	}
	return s.SetSetting("email_mfa:"+key, string(v))
}

// GetEmailMFACode 读取邮箱 MFA 验证码（不消费）；不存在返回 ErrNotFound。
func (s *Store) GetEmailMFACode(key string) (code, expiresAt string, err error) {
	v := s.GetSetting("email_mfa:" + key)
	if v == "" {
		return "", "", ErrNotFound
	}
	var d emailMFACodeData
	if err := json.Unmarshal([]byte(v), &d); err != nil {
		return "", "", err
	}
	return d.Code, d.Expires, nil
}

// DeleteEmailMFACode 删除邮箱 MFA 验证码。
func (s *Store) DeleteEmailMFACode(key string) error {
	return s.db.Where("\"key\" = ?", "email_mfa:"+key).Delete(&settingRow{}).Error
}
