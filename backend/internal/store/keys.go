package store

import (
	"database/sql"
	"errors"
)

func (s *Store) UserID(username string) (int64, error) {
	var id int64
	err := s.db.QueryRow(`SELECT id FROM users WHERE username = ?`, username).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	return id, err
}

func (s *Store) CreateKey(username, name, publicKey, fingerprint string) (SSHKey, error) {
	k := SSHKey{Name: name, PublicKey: publicKey, Fingerprint: fingerprint, CreatedAt: now()}
	res, err := s.db.Exec(`INSERT INTO ssh_keys (user_id, name, public_key, fingerprint, created_at)
		VALUES ((SELECT id FROM users WHERE username = ?), ?, ?, ?, ?)`,
		username, k.Name, k.PublicKey, k.Fingerprint, k.CreatedAt)
	if err != nil {
		if isUniqueErr(err) {
			return k, ErrExists
		}
		return k, err
	}
	k.ID, _ = res.LastInsertId()
	return k, nil
}

func (s *Store) ListKeys(username string) ([]SSHKey, error) {
	rows, err := s.db.Query(`SELECT k.id, k.name, k.public_key, k.fingerprint, k.created_at
		FROM ssh_keys k JOIN users u ON u.id = k.user_id WHERE u.username = ? ORDER BY k.id DESC`, username)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	keys := []SSHKey{}
	for rows.Next() {
		var k SSHKey
		if err := rows.Scan(&k.ID, &k.Name, &k.PublicKey, &k.Fingerprint, &k.CreatedAt); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

func (s *Store) PublicKeys() ([]PublicKeyAuth, error) {
	rows, err := s.db.Query(`SELECT k.user_id, u.username, k.public_key FROM ssh_keys k JOIN users u ON u.id = k.user_id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var keys []PublicKeyAuth
	for rows.Next() {
		var k PublicKeyAuth
		if err := rows.Scan(&k.UserID, &k.Username, &k.Line); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

func (s *Store) DeleteKey(username string, id int64) error {
	res, err := s.db.Exec(`DELETE FROM ssh_keys WHERE id = ? AND user_id = (SELECT id FROM users WHERE username = ?)`, id, username)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- gpg keys ----

func (s *Store) AddGPGKey(username, fingerprint, armor string) (GPGKey, error) {
	k := GPGKey{Fingerprint: fingerprint, CreatedAt: now()}
	res, err := s.db.Exec(`INSERT INTO gpg_keys (user_id, fingerprint, armor, created_at)
		VALUES ((SELECT id FROM users WHERE username = ?), ?, ?, ?)`,
		username, k.Fingerprint, armor, k.CreatedAt)
	if err != nil {
		if isUniqueErr(err) {
			return k, ErrExists
		}
		return k, err
	}
	k.ID, _ = res.LastInsertId()
	return k, nil
}

func (s *Store) ListGPGKeys(username string) ([]GPGKey, error) {
	rows, err := s.db.Query(`SELECT k.id, k.fingerprint, k.created_at
		FROM gpg_keys k JOIN users u ON u.id = k.user_id WHERE u.username = ? ORDER BY k.id`, username)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []GPGKey{}
	for rows.Next() {
		var k GPGKey
		if err := rows.Scan(&k.ID, &k.Fingerprint, &k.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

func (s *Store) DeleteGPGKey(username string, id int64) error {
	res, err := s.db.Exec(`DELETE FROM gpg_keys WHERE id = ? AND user_id = (SELECT id FROM users WHERE username = ?)`, id, username)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// AllGPGKeys 返回全部用户注册的公钥（供提交签名校验使用）。
func (s *Store) AllGPGKeys() ([]GPGKeyAuth, error) {
	rows, err := s.db.Query(`SELECT u.username, k.fingerprint, k.armor FROM gpg_keys k JOIN users u ON u.id = k.user_id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []GPGKeyAuth{}
	for rows.Next() {
		var k GPGKeyAuth
		if err := rows.Scan(&k.Username, &k.Fingerprint, &k.Armor); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// ---- pull requests ----
