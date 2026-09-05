package store

import (
	"database/sql"
	"errors"
	"time"
)

func (s *Store) AdminCount() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM admin_users`).Scan(&n)
	return n, err
}

func (s *Store) CreateAdminUser(username, passwordHash string) error {
	_, err := s.db.Exec(`INSERT INTO admin_users (username, password_hash, created_at) VALUES (?, ?, ?)`,
		username, passwordHash, now())
	if err != nil && isUniqueErr(err) {
		return ErrExists
	}
	return err
}

func (s *Store) AdminAuth(username string) (int64, string, error) {
	var id int64
	var hash string
	err := s.db.QueryRow(`SELECT id, password_hash FROM admin_users WHERE username = ?`, username).Scan(&id, &hash)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", ErrNotFound
	}
	return id, hash, err
}

func (s *Store) UpdateAdminPassword(username, passwordHash string) error {
	res, err := s.db.Exec(`UPDATE admin_users SET password_hash = ? WHERE username = ?`, passwordHash, username)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) CreateAdminSession(token string, adminID int64) error {
	_, err := s.db.Exec(`INSERT INTO admin_sessions (token, admin_id, created_at, expires_at) VALUES (?, ?, ?, ?)`,
		token, adminID, now(), time.Now().Add(12*time.Hour).UTC().Format(time.RFC3339))
	return err
}

func (s *Store) GetAdminSession(token string) (int64, string, error) {
	var id int64
	var username string
	err := s.db.QueryRow(`SELECT a.id, a.username FROM admin_sessions s
		JOIN admin_users a ON a.id = s.admin_id
		WHERE s.token = ? AND s.expires_at > ?`, token, now()).Scan(&id, &username)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", ErrNotFound
	}
	return id, username, err
}

func (s *Store) DeleteAdminSession(token string) error {
	_, err := s.db.Exec(`DELETE FROM admin_sessions WHERE token = ?`, token)
	return err
}

func (s *Store) GetSetting(key string) string {
	var v string
	_ = s.db.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	return v
}

func (s *Store) SetSetting(key, value string) error {
	_, err := s.db.Exec(`INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}
