package store

import "encoding/json"

// WebAuthn / Passkey 凭据存储。
//
// 每条记录对应一个 FIDO2/passkey 凭据；Data 为序列化后的 webauthn.Credential
// （包含公钥、sign count、AAGUID、flags 等）。store 层不依赖 webauthn 包，
// 仅按不透明 JSON 存取，解析由 API 层负责。

// WebAuthnCredential 一条已注册的 passkey（脱敏后的 API DTO）。
type WebAuthnCredential struct {
	ID             int64    `json:"id"`
	Name           string   `json:"name"`
	CredentialID   string   `json:"credential_id"`
	CreatedAt      string   `json:"created_at"`
	LastUsedAt     string   `json:"last_used_at,omitempty"`
	Transports     []string `json:"transports,omitempty"`
	BackupEligible bool     `json:"backup_eligible"`
	BackupState    bool     `json:"backup_state"`
}

// WebAuthnCredentialRecord 是存储层的完整记录（含序列化凭据，仅供服务端使用）。
type WebAuthnCredentialRecord struct {
	ID           int64
	UserID       int64
	Username     string
	CredentialID string
	Name         string
	Data         []byte
	CreatedAt    string
	LastUsedAt   string
}

// AddWebAuthnCredential 为 username 新增一条 passkey 记录。
// credentialID 为 base64url(rawID)，data 为序列化的 webauthn.Credential。
// 凭据 ID 全局唯一：重复注册同一 authenticator 时返回 ErrExists。
func (s *Store) AddWebAuthnCredential(username, name, credentialID string, data []byte) (int64, error) {
	var u userRow
	if err := s.db.Where("username = ?", username).First(&u).Error; err != nil {
		return 0, notFoundErr(err)
	}
	row := webauthnCredentialRow{
		UserID:       u.ID,
		CredentialID: credentialID,
		Name:         name,
		Data:         string(data),
		CreatedAt:    now(),
	}
	if err := s.db.Create(&row).Error; err != nil {
		if isUniqueErr(err) {
			return 0, ErrExists
		}
		return 0, err
	}
	return row.ID, nil
}

// ListWebAuthnCredentials 返回某用户全部 passkey 记录（按创建时间正序）。
func (s *Store) ListWebAuthnCredentials(username string) ([]WebAuthnCredentialRecord, error) {
	var rows []webauthnCredentialRow
	if err := s.db.Where("user_id = (?)",
		s.db.Model(&userRow{}).Select("id").Where("username = ?", username)).
		Order("id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]WebAuthnCredentialRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, WebAuthnCredentialRecord{
			ID:           r.ID,
			UserID:       r.UserID,
			Username:     username,
			CredentialID: r.CredentialID,
			Name:         r.Name,
			Data:         []byte(r.Data),
			CreatedAt:    r.CreatedAt,
			LastUsedAt:   r.LastUsedAt,
		})
	}
	return out, nil
}

// WebAuthnCredentialByCredentialID 按凭据 ID 定位记录（用于 discoverable 登录）。
func (s *Store) WebAuthnCredentialByCredentialID(credentialID string) (WebAuthnCredentialRecord, error) {
	var r webauthnCredentialRow
	if err := s.db.Where("credential_id = ?", credentialID).First(&r).Error; err != nil {
		return WebAuthnCredentialRecord{}, notFoundErr(err)
	}
	var username string
	if err := s.db.Model(&userRow{}).Select("username").Where("id = ?", r.UserID).Scan(&username).Error; err != nil {
		return WebAuthnCredentialRecord{}, err
	}
	if username == "" {
		return WebAuthnCredentialRecord{}, ErrNotFound
	}
	return WebAuthnCredentialRecord{
		ID:           r.ID,
		UserID:       r.UserID,
		Username:     username,
		CredentialID: r.CredentialID,
		Name:         r.Name,
		Data:         []byte(r.Data),
		CreatedAt:    r.CreatedAt,
		LastUsedAt:   r.LastUsedAt,
	}, nil
}

// UpdateWebAuthnCredential 登录成功后更新凭据（sign count / flags）与最近使用时间。
func (s *Store) UpdateWebAuthnCredential(credentialID string, data []byte) error {
	res := s.db.Model(&webauthnCredentialRow{}).Where("credential_id = ?", credentialID).
		Updates(map[string]any{"data": string(data), "last_used_at": now()})
	return res.Error
}

// DeleteWebAuthnCredential 删除某用户名下的一条 passkey；不存在返回 ErrNotFound。
func (s *Store) DeleteWebAuthnCredential(username string, id int64) error {
	res := s.db.Where("id = ? AND user_id = (?)", id,
		s.db.Model(&userRow{}).Select("id").Where("username = ?", username)).
		Delete(&webauthnCredentialRow{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// CountWebAuthnCredentials 返回某用户已注册的 passkey 数量。
func (s *Store) CountWebAuthnCredentials(username string) (int, error) {
	var n int64
	err := s.db.Model(&webauthnCredentialRow{}).
		Where("user_id = (?)", s.db.Model(&userRow{}).Select("id").Where("username = ?", username)).
		Count(&n).Error
	return int(n), err
}

// ---- WebAuthn ceremony session（一次性，存 settings 表）----

// webauthnSessionData 包一层以便过期清理时无需理解 webauthn.SessionData 细节。
type webauthnSessionData struct {
	Data    json.RawMessage `json:"d"`
	Expires string          `json:"e"`
}

// PutWebAuthnSession 持久化一次 WebAuthn ceremony 会话数据（注册 / 登录开始阶段）。
func (s *Store) PutWebAuthnSession(token string, data []byte, expires string) error {
	v, err := json.Marshal(webauthnSessionData{Data: data, Expires: expires})
	if err != nil {
		return err
	}
	return s.SetSetting("webauthn_session:"+token, string(v))
}

// TakeWebAuthnSession 取出并删除会话数据（一次性）。不存在或已过期返回 ErrNotFound。
func (s *Store) TakeWebAuthnSession(token, nowStr string) ([]byte, error) {
	v := s.GetSetting("webauthn_session:" + token)
	if v == "" {
		return nil, ErrNotFound
	}
	_ = s.db.Where("\"key\" = ?", "webauthn_session:"+token).Delete(&settingRow{}).Error
	var d webauthnSessionData
	if err := json.Unmarshal([]byte(v), &d); err != nil {
		return nil, err
	}
	if d.Expires != "" && d.Expires <= nowStr {
		return nil, ErrNotFound
	}
	return d.Data, nil
}

// DeleteWebAuthnSession 删除会话数据（未使用 / 过期清理）。
func (s *Store) DeleteWebAuthnSession(token string) error {
	return s.db.Where("\"key\" = ?", "webauthn_session:"+token).Delete(&settingRow{}).Error
}

// PruneWebAuthnSessions 删除已过期的 WebAuthn ceremony 会话，返回清理条数。
func (s *Store) PruneWebAuthnSessions(nowStr string) (int64, error) {
	var rows []settingRow
	if err := s.db.Where("\"key\" LIKE 'webauthn_session:%'").Find(&rows).Error; err != nil {
		return 0, err
	}
	var pruned int64
	for _, row := range rows {
		var d webauthnSessionData
		if json.Unmarshal([]byte(row.Value), &d) != nil || d.Expires <= nowStr {
			if err := s.db.Where("\"key\" = ?", row.Key).Delete(&settingRow{}).Error; err != nil {
				return pruned, err
			}
			pruned++
		}
	}
	return pruned, nil
}
