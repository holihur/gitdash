package store

import "time"

// ---- users & sessions ----

func (s *Store) CreateUser(username, passwordHash string) (User, error) {
	u := User{Username: username, CreatedAt: now()}
	row := userRow{Username: username, PasswordHash: passwordHash, CreatedAt: u.CreatedAt}
	if err := s.db.Create(&row).Error; err != nil {
		if isUniqueErr(err) {
			return u, ErrExists
		}
		return u, err
	}
	u.ID = row.ID
	return u, nil
}

func (s *Store) GetByUsername(username string) (UserAuth, error) {
	var row userRow
	err := s.db.Where("username = ?", username).First(&row).Error
	if err != nil {
		return UserAuth{}, notFoundErr(err)
	}
	return UserAuth(row), nil
}

const SessionTTL = 7 * 24 * time.Hour

func (s *Store) CreateSession(token string, userID int64) error {
	row := sessionRow{
		Token:     token,
		UserID:    userID,
		CreatedAt: now(),
		ExpiresAt: time.Now().Add(SessionTTL).UTC().Format(time.RFC3339),
	}
	return s.db.Create(&row).Error
}

func (s *Store) GetSession(token string) (string, error) {
	var username string
	err := s.db.Table("sessions").
		Select("users.username").
		Joins("JOIN users ON users.id = sessions.user_id").
		Where("sessions.token = ? AND sessions.expires_at > ?", token, now()).
		Scan(&username).Error
	if err != nil {
		return "", err
	}
	if username == "" {
		return "", ErrNotFound
	}
	return username, nil
}

func (s *Store) DeleteSession(token string) error {
	return s.db.Where("token = ?", token).Delete(&sessionRow{}).Error
}

// DeleteSessionsExcept 撤销用户除 keepToken 外的全部会话（改密/安全操作后调用）。
func (s *Store) DeleteSessionsExcept(username, keepToken string) error {
	return s.db.Where("user_id = (?) AND token <> ?",
		s.db.Model(&userRow{}).Select("id").Where("username = ?", username),
		keepToken).Delete(&sessionRow{}).Error
}

// SetUserEmail 更新个人资料邮箱（空串表示清除）。
func (s *Store) SetUserEmail(username, email string) error {
	res := s.db.Model(&userRow{}).Where("username = ?", username).
		Update("email", email)
	if res.Error != nil {
		if isUniqueErr(res.Error) {
			return ErrExists
		}
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- user profile & mfa ----

func (s *Store) UpdatePassword(username, passwordHash string) error {
	res := s.db.Model(&userRow{}).Where("username = ?", username).
		Update("password_hash", passwordHash)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// SetMFASecret 写入（或覆盖）MFA secret；enable=false 时保留 secret 但标记未激活。
func (s *Store) SetMFASecret(username, secret string, enabled bool) error {
	res := s.db.Model(&userRow{}).Where("username = ?", username).
		Updates(map[string]any{"mfa_secret": secret, "mfa_enabled": enabled})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ClearMFA(username string) error {
	return s.db.Model(&userRow{}).Where("username = ?", username).
		Updates(map[string]any{"mfa_secret": "", "mfa_enabled": false}).Error
}

// ---- repos ----
func (s *Store) LinkOAuth(provider, externalID string, userID int64) error {
	row := userOAuthRow{Provider: provider, ExternalID: externalID, UserID: userID, CreatedAt: now()}
	err := s.db.Create(&row).Error
	if err != nil && isUniqueErr(err) {
		return ErrExists
	}
	return err
}

// OAuthUser 返回 (userID, username) 或 ErrNotFound。
func (s *Store) OAuthUser(provider, externalID string) (int64, string, error) {
	var dest struct {
		ID       int64
		Username string
	}
	err := s.db.Table("user_oauth").
		Select("users.id, users.username").
		Joins("JOIN users ON users.id = user_oauth.user_id").
		Where("user_oauth.provider = ? AND user_oauth.external_id = ?", provider, externalID).
		Scan(&dest).Error
	if err != nil {
		return 0, "", err
	}
	if dest.ID == 0 && dest.Username == "" {
		return 0, "", ErrNotFound
	}
	return dest.ID, dest.Username, nil
}
