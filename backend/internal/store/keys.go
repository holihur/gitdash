package store

import "gorm.io/gorm"

func (s *Store) UserID(username string) (int64, error) {
	var row userRow
	err := s.db.Select("id").Where("username = ?", username).First(&row).Error
	if err != nil {
		return 0, notFoundErr(err)
	}
	return row.ID, nil
}

func (s *Store) CreateKey(username, name, publicKey, fingerprint string) (SSHKey, error) {
	k := SSHKey{Name: name, PublicKey: publicKey, Fingerprint: fingerprint, CreatedAt: now()}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var u userRow
		if err := tx.Select("id").Where("username = ?", username).First(&u).Error; err != nil {
			return notFoundErr(err)
		}
		row := sshKeyRow{UserID: u.ID, Name: name, PublicKey: publicKey, Fingerprint: fingerprint, CreatedAt: k.CreatedAt}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		k.ID = row.ID
		return nil
	})
	if err != nil {
		if isUniqueErr(err) {
			return k, ErrExists
		}
		return k, err
	}
	return k, nil
}

func (s *Store) ListKeys(username string) ([]SSHKey, error) {
	var rows []sshKeyRow
	err := s.db.Table("ssh_keys").
		Select("ssh_keys.*").
		Joins("JOIN users ON users.id = ssh_keys.user_id").
		Where("users.username = ?", username).
		Order("ssh_keys.id DESC").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	keys := []SSHKey{}
	for _, r := range rows {
		keys = append(keys, SSHKey{ID: r.ID, Name: r.Name, PublicKey: r.PublicKey, Fingerprint: r.Fingerprint, CreatedAt: r.CreatedAt})
	}
	return keys, nil
}

func (s *Store) PublicKeys() ([]PublicKeyAuth, error) {
	var out []PublicKeyAuth
	err := s.db.Table("ssh_keys").
		Select("ssh_keys.user_id, users.username, ssh_keys.public_key").
		Joins("JOIN users ON users.id = ssh_keys.user_id").
		Scan(&out).Error
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) DeleteKey(username string, id int64) error {
	res := s.db.Where("id = ? AND user_id = (?)", id,
		s.db.Model(&userRow{}).Select("id").Where("username = ?", username)).
		Delete(&sshKeyRow{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- gpg keys ----

func (s *Store) AddGPGKey(username, fingerprint, armor string) (GPGKey, error) {
	k := GPGKey{Fingerprint: fingerprint, CreatedAt: now()}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var u userRow
		if err := tx.Select("id").Where("username = ?", username).First(&u).Error; err != nil {
			return notFoundErr(err)
		}
		row := gpgKeyRow{UserID: u.ID, Fingerprint: fingerprint, Armor: armor, CreatedAt: k.CreatedAt}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		k.ID = row.ID
		return nil
	})
	if err != nil {
		if isUniqueErr(err) {
			return k, ErrExists
		}
		return k, err
	}
	return k, nil
}

func (s *Store) ListGPGKeys(username string) ([]GPGKey, error) {
	var rows []gpgKeyRow
	err := s.db.Table("gpg_keys").
		Select("gpg_keys.*").
		Joins("JOIN users ON users.id = gpg_keys.user_id").
		Where("users.username = ?", username).
		Order("gpg_keys.id").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := []GPGKey{}
	for _, r := range rows {
		out = append(out, GPGKey{ID: r.ID, Fingerprint: r.Fingerprint, CreatedAt: r.CreatedAt})
	}
	return out, nil
}

func (s *Store) DeleteGPGKey(username string, id int64) error {
	res := s.db.Where("id = ? AND user_id = (?)", id,
		s.db.Model(&userRow{}).Select("id").Where("username = ?", username)).
		Delete(&gpgKeyRow{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// AllGPGKeys 返回全部用户注册的公钥（供提交签名校验使用）。
func (s *Store) AllGPGKeys() ([]GPGKeyAuth, error) {
	var out []GPGKeyAuth
	err := s.db.Table("gpg_keys").
		Select("users.username, gpg_keys.fingerprint, gpg_keys.armor").
		Joins("JOIN users ON users.id = gpg_keys.user_id").
		Scan(&out).Error
	if err != nil {
		return nil, err
	}
	return out, nil
}
