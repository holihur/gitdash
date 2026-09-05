package store

import (
	"database/sql"
	"errors"
	"time"
)

// ---- users & sessions ----

func (s *Store) CreateUser(username, passwordHash string) (User, error) {
	u := User{Username: username, CreatedAt: now()}
	res, err := s.db.Exec(`INSERT INTO users (username, password_hash, created_at) VALUES (?, ?, ?)`,
		u.Username, passwordHash, u.CreatedAt)
	if err != nil {
		if isUniqueErr(err) {
			return u, ErrExists
		}
		return u, err
	}
	u.ID, _ = res.LastInsertId()
	return u, nil
}

func (s *Store) GetByUsername(username string) (UserAuth, error) {
	var ua UserAuth
	var mfa int
	err := s.db.QueryRow(`SELECT id, username, password_hash, created_at, COALESCE(mfa_secret,''), mfa_enabled
		FROM users WHERE username = ?`, username).
		Scan(&ua.ID, &ua.Username, &ua.PasswordHash, &ua.CreatedAt, &ua.MFASecret, &mfa)
	ua.MFAEnabled = mfa != 0
	if errors.Is(err, sql.ErrNoRows) {
		return ua, ErrNotFound
	}
	return ua, err
}

const SessionTTL = 7 * 24 * time.Hour

func (s *Store) CreateSession(token string, userID int64) error {
	_, err := s.db.Exec(`INSERT INTO sessions (token, user_id, created_at, expires_at) VALUES (?, ?, ?, ?)`,
		token, userID, now(), time.Now().Add(SessionTTL).UTC().Format(time.RFC3339))
	return err
}

func (s *Store) GetSession(token string) (string, error) {
	var username string
	err := s.db.QueryRow(
		`SELECT u.username FROM sessions s JOIN users u ON u.id = s.user_id
		 WHERE s.token = ? AND s.expires_at > ?`, token, now()).
		Scan(&username)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return username, err
}

func (s *Store) DeleteSession(token string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE token = ?`, token)
	return err
}

// DeleteSessionsExcept 撤销用户除 keepToken 外的全部会话（改密/安全操作后调用）。
func (s *Store) DeleteSessionsExcept(username, keepToken string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE user_id = (SELECT id FROM users WHERE username = ?) AND token <> ?`,
		username, keepToken)
	return err
}

// ---- user profile & mfa ----

func (s *Store) UpdatePassword(username, passwordHash string) error {
	res, err := s.db.Exec(`UPDATE users SET password_hash = ? WHERE username = ?`, passwordHash, username)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetMFASecret 写入（或覆盖）MFA secret；enable=false 时保留 secret 但标记未激活。
func (s *Store) SetMFASecret(username, secret string, enabled bool) error {
	en := 0
	if enabled {
		en = 1
	}
	res, err := s.db.Exec(`UPDATE users SET mfa_secret = ?, mfa_enabled = ? WHERE username = ?`, secret, en, username)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ClearMFA(username string) error {
	_, err := s.db.Exec(`UPDATE users SET mfa_secret = '', mfa_enabled = 0 WHERE username = ?`, username)
	return err
}

// ---- repos ----
func (s *Store) LinkOAuth(provider, externalID string, userID int64) error {
	_, err := s.db.Exec(`INSERT INTO user_oauth (provider, external_id, user_id, created_at) VALUES (?, ?, ?, ?)`,
		provider, externalID, userID, now())
	if err != nil && isUniqueErr(err) {
		return ErrExists
	}
	return err
}

// OAuthUser 返回 (userID, username) 或 ErrNotFound。
func (s *Store) OAuthUser(provider, externalID string) (int64, string, error) {
	var id int64
	var username string
	err := s.db.QueryRow(`SELECT u.id, u.username FROM user_oauth o JOIN users u ON u.id = o.user_id
		WHERE o.provider = ? AND o.external_id = ?`, provider, externalID).Scan(&id, &username)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", ErrNotFound
	}
	return id, username, err
}

// ---- stars ----
