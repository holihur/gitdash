package store

import "strings"

// ---- 密码重置令牌（存 settings 表，避免改动 users 表结构）----
//
// key 为 pwreset:<sha256(token)>，value 为 "<username>|<expires RFC3339>"。
// 明文 token 仅通过邮件链接下发，不落库。

const pwResetKeyPrefix = "pwreset:"

// PutPasswordReset 保存（或覆盖）一个密码重置令牌。
func (s *Store) PutPasswordReset(token, username, expiresAt string) error {
	return s.SetSetting(pwResetKeyPrefix+patHash(token), username+"|"+expiresAt)
}

// ClearPasswordResets 删除某用户的全部密码重置令牌。
// 逐个比对 value 前缀（而非 SQL LIKE），避免用户名中的 '_' 被当作通配符误删他人令牌。
func (s *Store) ClearPasswordResets(username string) error {
	var rows []settingRow
	if err := s.db.Where("\"key\" LIKE ?", pwResetKeyPrefix+"%").Find(&rows).Error; err != nil {
		return err
	}
	prefix := username + "|"
	keys := make([]string, 0, len(rows))
	for _, r := range rows {
		if strings.HasPrefix(r.Value, prefix) {
			keys = append(keys, r.Key)
		}
	}
	if len(keys) == 0 {
		return nil
	}
	return s.db.Where("\"key\" IN ?", keys).Delete(&settingRow{}).Error
}

// TakePasswordReset 校验并一次性消费重置令牌，成功返回用户名；
// 令牌不存在或已过期返回 ErrNotFound。
func (s *Store) TakePasswordReset(token string) (string, error) {
	key := pwResetKeyPrefix + patHash(token)
	value := s.GetSetting(key)
	if value == "" {
		return "", ErrNotFound
	}
	_ = s.db.Where("\"key\" = ?", key).Delete(&settingRow{}).Error
	username, expires, ok := strings.Cut(value, "|")
	if !ok || username == "" || expires == "" || expires <= now() {
		return "", ErrNotFound
	}
	return username, nil
}
