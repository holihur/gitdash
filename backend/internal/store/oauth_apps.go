package store

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"gorm.io/gorm"
)

// ---- OAuth 2.0 provider（gitdash 作为授权服务器，供第三方应用接入）----
//
// 授权码流程：应用注册 → 用户 /login/oauth/authorize 授权 → 回调携带 code →
// 应用 /login/oauth/access_token 用 client_secret 换 access_token。
// access_token 复用 PAT 表（oauth_app_id 关联应用），因此现有 Bearer 校验、
// scope 校验、过期与来源 IP 白名单全部沿用。

// OAuthApp DTO（不含 client_secret；明文仅在创建/重置时返回一次）。
type OAuthApp struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Homepage    string `json:"homepage"`
	Description string `json:"description"`
	CallbackURL string `json:"callback_url"`
	ClientID    string `json:"client_id"`
	CreatedAt   string `json:"created_at"`
}

// OAuthAppSecret 创建/重置应用时返回的完整信息（含明文 client_secret）。
type OAuthAppSecret struct {
	OAuthApp
	ClientSecret string `json:"client_secret"`
}

// OAuthAuthorization 某应用签发的一个 access token（用于「已授权应用」列表与撤销）。
type OAuthAuthorization struct {
	ID         int64    `json:"id"`
	AppID      int64    `json:"app_id"`
	AppName    string   `json:"app_name"`
	Scopes     []string `json:"scopes"`
	CreatedAt  string   `json:"created_at"`
	LastUsedAt string   `json:"last_used_at"`
}

// oauthGrant 授权码消费结果。
type oauthGrant struct {
	AppID       int64
	UserID      int64
	Scopes      string
	RedirectURI string
}

// OAuthFirstPartyClientID 内置第一方公开客户端（gitdash-cli 设备流专用，无 client_secret）。
const OAuthFirstPartyClientID = "gitdash-cli"

// ensureFirstPartyOAuthApp 幂等引导第一方 CLI 客户端（user_id=0，系统所有）。
func (s *Store) ensureFirstPartyOAuthApp() error {
	if _, err := s.GetOAuthAppByClientID(OAuthFirstPartyClientID); err == nil {
		return nil
	} else if !errors.Is(err, ErrNotFound) {
		return err
	}
	row := oauthAppRow{
		UserID:           0,
		Name:             "gitdash CLI",
		Homepage:         "",
		Description:      "First-party CLI client (device flow)",
		CallbackURL:      "",
		ClientID:         OAuthFirstPartyClientID,
		ClientSecretHash: "", // 公开客户端无需 secret
		CreatedAt:        now(),
	}
	return s.db.Create(&row).Error
}

func hashOAuthSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

func oauthCodeHash(code string) string {
	sum := sha256.Sum256([]byte(code))
	return hex.EncodeToString(sum[:])
}

func newOAuthClientID() (string, error) {
	raw := make([]byte, 10) // 20 hex
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

func newOAuthSecret() (string, error) {
	raw := make([]byte, 20) // 40 hex
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

// CreateOAuthApp 注册第三方应用，返回含明文 client_secret 的完整信息（明文仅此一次）。
func (s *Store) CreateOAuthApp(userID int64, name, homepage, description, callbackURL string) (OAuthAppSecret, error) {
	createMu.Lock()
	defer createMu.Unlock()
	clientID, err := newOAuthClientID()
	if err != nil {
		return OAuthAppSecret{}, err
	}
	secret, err := newOAuthSecret()
	if err != nil {
		return OAuthAppSecret{}, err
	}
	row := oauthAppRow{
		UserID:           userID,
		Name:             name,
		Homepage:         homepage,
		Description:      description,
		CallbackURL:      callbackURL,
		ClientID:         clientID,
		ClientSecretHash: hashOAuthSecret(secret),
		CreatedAt:        now(),
	}
	if err := s.db.Create(&row).Error; err != nil {
		return OAuthAppSecret{}, err
	}
	return OAuthAppSecret{
		OAuthApp: OAuthApp{
			ID: row.ID, Name: name, Homepage: homepage, Description: description,
			CallbackURL: callbackURL, ClientID: clientID, CreatedAt: row.CreatedAt,
		},
		ClientSecret: secret,
	}, nil
}

// ListOAuthApps 列出用户注册的所有应用（不含 secret）。
func (s *Store) ListOAuthApps(userID int64) ([]OAuthApp, error) {
	var rows []oauthAppRow
	if err := s.db.Where("user_id = ?", userID).Order("id DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]OAuthApp, 0, len(rows))
	for _, r := range rows {
		out = append(out, OAuthApp{ID: r.ID, Name: r.Name, Homepage: r.Homepage, Description: r.Description, CallbackURL: r.CallbackURL, ClientID: r.ClientID, CreatedAt: r.CreatedAt})
	}
	return out, nil
}

// GetOAuthApp 按 (userID, id) 取应用；不存在返回 ErrNotFound。
func (s *Store) GetOAuthApp(userID, id int64) (oauthAppRow, error) {
	var row oauthAppRow
	if err := s.db.Where("id = ? AND user_id = ?", id, userID).First(&row).Error; err != nil {
		return oauthAppRow{}, notFoundErr(err)
	}
	return row, nil
}

// GetOAuthAppByClientID 按 client_id 取应用（授权端点校验用）；不存在返回 ErrNotFound。
func (s *Store) GetOAuthAppByClientID(clientID string) (oauthAppRow, error) {
	var row oauthAppRow
	if err := s.db.Where("client_id = ?", clientID).First(&row).Error; err != nil {
		return oauthAppRow{}, notFoundErr(err)
	}
	return row, nil
}

// VerifyOAuthAppSecret 校验 client_secret 是否匹配。
func (s *Store) VerifyOAuthAppSecret(app oauthAppRow, secret string) bool {
	return app.ClientSecretHash == hashOAuthSecret(secret)
}

// DeleteOAuthApp 删除应用及其全部授权码与已签发 token（级联）。
func (s *Store) DeleteOAuthApp(userID, id int64) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var row oauthAppRow
		if err := tx.Where("id = ? AND user_id = ?", id, userID).First(&row).Error; err != nil {
			return notFoundErr(err)
		}
		if err := tx.Where("app_id = ?", id).Delete(&oauthGrantRow{}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ? AND oauth_app_id = ?", userID, id).Delete(&patRow{}).Error; err != nil {
			return err
		}
		return tx.Delete(&row).Error
	})
}

// ResetOAuthAppSecret 重置应用密钥，返回新明文（旧密钥立即失效）。
func (s *Store) ResetOAuthAppSecret(userID, id int64) (string, error) {
	secret, err := newOAuthSecret()
	if err != nil {
		return "", err
	}
	res := s.db.Model(&oauthAppRow{}).
		Where("id = ? AND user_id = ?", id, userID).
		Update("client_secret_hash", hashOAuthSecret(secret))
	if res.Error != nil {
		return "", res.Error
	}
	if res.RowsAffected == 0 {
		return "", ErrNotFound
	}
	return secret, nil
}

// CreateOAuthGrant 生成一次性授权码（明文仅返回一次），10 分钟有效。
func (s *Store) CreateOAuthGrant(appID, userID int64, scopes, redirectURI string) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	code := hex.EncodeToString(raw)
	row := oauthGrantRow{
		CodeHash:    oauthCodeHash(code),
		AppID:       appID,
		UserID:      userID,
		Scopes:      scopes,
		RedirectURI: redirectURI,
		ExpiresAt:   time.Now().Add(10 * time.Minute).UTC().Format(time.RFC3339),
		CreatedAt:   now(),
	}
	if err := s.db.Create(&row).Error; err != nil {
		return "", err
	}
	return code, nil
}

// ConsumeOAuthGrant 一次性校验并消费授权码；不存在/已过期/已消费返回 ErrNotFound。
func (s *Store) ConsumeOAuthGrant(code string) (oauthGrant, error) {
	hash := oauthCodeHash(code)
	var row oauthGrantRow
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("code_hash = ?", hash).First(&row).Error; err != nil {
			return notFoundErr(err)
		}
		if err := tx.Where("id = ?", row.ID).Delete(&oauthGrantRow{}).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return oauthGrant{}, err
	}
	if row.ExpiresAt == "" || row.ExpiresAt <= now() {
		return oauthGrant{}, ErrNotFound
	}
	return oauthGrant{AppID: row.AppID, UserID: row.UserID, Scopes: row.Scopes, RedirectURI: row.RedirectURI}, nil
}

// PruneOAuthGrants 清理过期授权码（后台定时调用）。
func (s *Store) PruneOAuthGrants(nowStr string) (int64, error) {
	res := s.db.Where("expires_at < ?", nowStr).Delete(&oauthGrantRow{})
	return res.RowsAffected, res.Error
}

// ListOAuthAuthorizations 列出某用户所有 OAuth 应用签发的 token（用于撤销）。
func (s *Store) ListOAuthAuthorizations(userID int64) ([]OAuthAuthorization, error) {
	var rows []struct {
		ID         int64
		AppID      int64
		AppName    string
		Scopes     string
		CreatedAt  string
		LastUsedAt string
	}
	err := s.db.Table("pats").
		Select("pats.id, pats.oauth_app_id AS app_id, oauth_apps.name AS app_name, pats.scopes, pats.created_at, pats.last_used_at").
		Joins("JOIN oauth_apps ON oauth_apps.id = pats.oauth_app_id").
		Where("pats.user_id = ? AND pats.oauth_app_id <> 0", userID).
		Order("pats.id DESC").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]OAuthAuthorization, 0, len(rows))
	for _, r := range rows {
		out = append(out, OAuthAuthorization{ID: r.ID, AppID: r.AppID, AppName: r.AppName, Scopes: splitScopes(r.Scopes), CreatedAt: r.CreatedAt, LastUsedAt: r.LastUsedAt})
	}
	return out, nil
}

// RevokeOAuthAuthorization 撤销某个 OAuth 签发的 token（按 token id，仅限本人）。
func (s *Store) RevokeOAuthAuthorization(userID, tokenID int64) error {
	res := s.db.Where("id = ? AND user_id = ? AND oauth_app_id <> 0", tokenID, userID).Delete(&patRow{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}
