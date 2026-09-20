package store

import (
	"errors"
	"strconv"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// LinkedAccount 是用户绑定的第三方账号（用于列出/批量导入远程仓库），
// 不含访问令牌等机密字段。
type LinkedAccount struct {
	Provider  string `json:"provider"`
	Login     string `json:"login"`
	AvatarURL string `json:"avatar_url"`
	BaseURL   string `json:"base_url"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// LinkedAccountSecret 是绑定账号的完整记录（含解密后的访问令牌），仅服务端使用。
type LinkedAccountSecret struct {
	LinkedAccount
	ExternalID   string
	AccessToken  string
	RefreshToken string
	Scope        string
}

// UpsertLinkedAccount 新增或更新 (userID, provider) 的绑定，令牌加密存储。
func (s *Store) UpsertLinkedAccount(userID int64, provider, externalID, login, avatarURL, baseURL, accessToken, refreshToken, scope string) error {
	row := linkedAccountRow{
		UserID:       userID,
		Provider:     provider,
		ExternalID:   externalID,
		Login:        login,
		AvatarURL:    avatarURL,
		BaseURL:      baseURL,
		AccessToken:  sealSecret(accessToken),
		RefreshToken: sealSecret(refreshToken),
		Scope:        scope,
		CreatedAt:    now(),
		UpdatedAt:    now(),
	}
	return s.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}, {Name: "provider"}},
		DoUpdates: clause.Assignments(map[string]any{
			"external_id":   externalID,
			"login":         login,
			"avatar_url":    avatarURL,
			"base_url":      baseURL,
			"access_token":  row.AccessToken,
			"refresh_token": row.RefreshToken,
			"scope":         scope,
			"updated_at":    now(),
		}),
	}).Create(&row).Error
}

// LinkedAccounts 返回用户的全部绑定（不含令牌）。
func (s *Store) LinkedAccounts(userID int64) ([]LinkedAccount, error) {
	var rows []linkedAccountRow
	if err := s.db.Where("user_id = ?", userID).Order("provider").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]LinkedAccount, 0, len(rows))
	for _, r := range rows {
		out = append(out, LinkedAccount{
			Provider:  r.Provider,
			Login:     r.Login,
			AvatarURL: r.AvatarURL,
			BaseURL:   r.BaseURL,
			CreatedAt: r.CreatedAt,
			UpdatedAt: r.UpdatedAt,
		})
	}
	return out, nil
}

// GetLinkedAccount 返回用户某个 provider 的绑定（含解密后的令牌）。
func (s *Store) GetLinkedAccount(userID int64, provider string) (*LinkedAccountSecret, error) {
	var row linkedAccountRow
	err := s.db.Where("user_id = ? AND provider = ?", userID, provider).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	token, err := openSecret(row.AccessToken)
	if err != nil {
		return nil, err
	}
	refresh, err := openSecret(row.RefreshToken)
	if err != nil {
		return nil, err
	}
	return &LinkedAccountSecret{
		LinkedAccount: LinkedAccount{
			Provider:  row.Provider,
			Login:     row.Login,
			AvatarURL: row.AvatarURL,
			BaseURL:   row.BaseURL,
			CreatedAt: row.CreatedAt,
			UpdatedAt: row.UpdatedAt,
		},
		ExternalID:   row.ExternalID,
		AccessToken:  token,
		RefreshToken: refresh,
		Scope:        row.Scope,
	}, nil
}

// DeleteLinkedAccount 解绑用户的一个 provider；不存在返回 ErrNotFound。
func (s *Store) DeleteLinkedAccount(userID int64, provider string) error {
	res := s.db.Where("user_id = ? AND provider = ?", userID, provider).Delete(&linkedAccountRow{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- 账号绑定用的 OAuth state（一次性，绑定到发起用户）----

// PutConnectState 持久化 (state → userID) 绑定意图，供回调校验并防 CSRF。
func (s *Store) PutConnectState(state string, userID int64, expiresAt string) error {
	return s.SetSetting("connect_state:"+state, strconv.FormatInt(userID, 10)+"\x1f"+expiresAt)
}

// TakeConnectState 取出并删除 state；返回绑定的 userID。不存在或已过期返回 (0,false)。
func (s *Store) TakeConnectState(state, nowStr string) (int64, bool) {
	key := "connect_state:" + state
	v := s.GetSetting(key)
	if v == "" {
		return 0, false
	}
	_ = s.db.Where("\"key\" = ?", key).Delete(&settingRow{}).Error
	i := strings.IndexByte(v, '\x1f')
	if i < 0 {
		return 0, false
	}
	id, err := strconv.ParseInt(v[:i], 10, 64)
	if err != nil || v[i+1:] <= nowStr {
		return 0, false
	}
	return id, true
}
