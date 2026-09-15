package store

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math/big"
	"time"
)

// ---- OAuth 2.0 设备流（RFC 8628）----
//
// CLI 等无浏览器环境：POST /login/oauth/device/code 拿 device_code + user_code，
// 用户在浏览器打开 verification_uri 输入 user_code 授权，CLI 轮询 token 端点
// 直到拿到 access_token。

var (
	// ErrDevicePending 用户尚未完成授权，CLI 应稍后重试。
	ErrDevicePending = errors.New("authorization_pending")
	// ErrDeviceDenied 用户拒绝了授权。
	ErrDeviceDenied = errors.New("access_denied")
	// ErrDeviceExpired 设备码已过期。
	ErrDeviceExpired = errors.New("expired_token")
)

const oauthDeviceCodeTTL = 15 * time.Minute

// deviceGrant token 换发结果。
type deviceGrant struct {
	UserID   int64
	Scopes   string
	ClientID string
}

func oauthDeviceCodeHash(code string) string {
	sum := sha256.Sum256([]byte(code))
	return hex.EncodeToString(sum[:])
}

// userCodeAlphabet 去除易混淆字符（0/O、1/I/L）的字母表。
const userCodeAlphabet = "BCDFGHJKLMNPQRSTVWXYZ23456789"

func newUserCode() (string, error) {
	b := make([]byte, 8)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(userCodeAlphabet))))
		if err != nil {
			return "", err
		}
		b[i] = userCodeAlphabet[n.Int64()]
	}
	return string(b[:4]) + "-" + string(b[4:]), nil
}

// CreateDeviceGrant 生成设备授权（返回 device_code 明文与 user_code）。
func (s *Store) CreateDeviceGrant(clientID, scopes string) (string, string, error) {
	deviceRaw := make([]byte, 32)
	if _, err := rand.Read(deviceRaw); err != nil {
		return "", "", err
	}
	deviceCode := hex.EncodeToString(deviceRaw)
	userCode, err := newUserCode()
	if err != nil {
		return "", "", err
	}
	row := oauthDeviceGrantRow{
		DeviceCodeHash: oauthDeviceCodeHash(deviceCode),
		UserCode:       userCode,
		ClientID:       clientID,
		Scopes:         scopes,
		Status:         "pending",
		ExpiresAt:      time.Now().Add(oauthDeviceCodeTTL).UTC().Format(time.RFC3339),
		CreatedAt:      now(),
	}
	if err := s.db.Create(&row).Error; err != nil {
		return "", "", err
	}
	return deviceCode, userCode, nil
}

// GetDeviceGrantByUserCode 按 user_code 查询（验证页展示 scope 用）。
func (s *Store) GetDeviceGrantByUserCode(userCode string) (oauthDeviceGrantRow, error) {
	var row oauthDeviceGrantRow
	if err := s.db.Where("user_code = ?", userCode).First(&row).Error; err != nil {
		return oauthDeviceGrantRow{}, notFoundErr(err)
	}
	return row, nil
}

// ApproveDeviceGrant 用户确认：pending → approved，绑定 userID。
func (s *Store) ApproveDeviceGrant(userCode string, userID int64) error {
	res := s.db.Model(&oauthDeviceGrantRow{}).
		Where("user_code = ? AND status = 'pending'", userCode).
		Updates(map[string]any{"status": "approved", "user_id": userID})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// DenyDeviceGrant 用户拒绝：pending → denied。
func (s *Store) DenyDeviceGrant(userCode string) error {
	res := s.db.Model(&oauthDeviceGrantRow{}).
		Where("user_code = ? AND status = 'pending'", userCode).
		Update("status", "denied")
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// ConsumeDeviceGrant 轮询消费 device_code：pending/denied/expired 返回对应哨兵错误；
// approved 时一次性删除并返回授权结果。
func (s *Store) ConsumeDeviceGrant(deviceCode string) (deviceGrant, error) {
	var row oauthDeviceGrantRow
	if err := s.db.Where("device_code_hash = ?", oauthDeviceCodeHash(deviceCode)).First(&row).Error; err != nil {
		return deviceGrant{}, notFoundErr(err)
	}
	if row.ExpiresAt != "" && row.ExpiresAt <= now() {
		_ = s.db.Delete(&row).Error
		return deviceGrant{}, ErrDeviceExpired
	}
	switch row.Status {
	case "pending":
		return deviceGrant{}, ErrDevicePending
	case "denied":
		_ = s.db.Delete(&row).Error
		return deviceGrant{}, ErrDeviceDenied
	case "approved":
		if err := s.db.Delete(&row).Error; err != nil {
			return deviceGrant{}, err
		}
		return deviceGrant{UserID: row.UserID, Scopes: row.Scopes, ClientID: row.ClientID}, nil
	default:
		return deviceGrant{}, ErrNotFound
	}
}

// PruneDeviceGrants 清理过期设备授权。
func (s *Store) PruneDeviceGrants(nowStr string) (int64, error) {
	res := s.db.Where("expires_at < ?", nowStr).Delete(&oauthDeviceGrantRow{})
	return res.RowsAffected, res.Error
}
