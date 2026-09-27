package store

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"

	"gorm.io/gorm"
)

// incomingTokenHash 返回入站 webhook token 的 sha256 hex（只存散列，明文不落库）。
func incomingTokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func toIncomingWebhook(r incomingWebhookRow) IncomingWebhook {
	return IncomingWebhook{
		ID:         r.ID,
		Name:       r.Name,
		Owner:      r.Owner,
		Repo:       r.Repo,
		Enabled:    r.Enabled,
		CreatedAt:  r.CreatedAt,
		LastUsedAt: r.LastUsedAt,
	}
}

// CreateIncomingWebhook 为仓库新增一个入站 webhook token，返回明文 token（仅此一次）。
// 一个仓库可有多个 token，各自独立（Name 仅用于展示/区分）。
func (s *Store) CreateIncomingWebhook(owner, repo, name string) (string, IncomingWebhook, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", IncomingWebhook{}, err
	}
	token := hex.EncodeToString(raw)
	row := incomingWebhookRow{
		Owner: owner, Repo: repo, Name: name, TokenHash: incomingTokenHash(token), Enabled: true, CreatedAt: now(),
	}
	if err := s.db.Create(&row).Error; err != nil {
		return "", IncomingWebhook{}, err
	}
	return token, toIncomingWebhook(row), nil
}

// ListIncomingWebhooks 返回仓库的全部入站 webhook（按创建顺序）。
func (s *Store) ListIncomingWebhooks(owner, repo string) ([]IncomingWebhook, error) {
	var rows []incomingWebhookRow
	if err := s.db.Where("owner = ? AND repo = ?", owner, repo).Order("id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]IncomingWebhook, 0, len(rows))
	for _, r := range rows {
		out = append(out, toIncomingWebhook(r))
	}
	return out, nil
}

// DeleteIncomingWebhook 按 id 删除仓库的一个入站 webhook（不存在返回 ErrNotFound）。
func (s *Store) DeleteIncomingWebhook(owner, repo string, id int64) error {
	res := s.db.Where("id = ? AND owner = ? AND repo = ?", id, owner, repo).Delete(&incomingWebhookRow{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// SetIncomingWebhookEnabled 启用 / 禁用某个入站 webhook（禁用后 token 立即失效，但保留配置）。
func (s *Store) SetIncomingWebhookEnabled(owner, repo string, id int64, enabled bool) error {
	res := s.db.Model(&incomingWebhookRow{}).
		Where("id = ? AND owner = ? AND repo = ?", id, owner, repo).
		Update("enabled", enabled)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// ResolveIncomingWebhook 校验 token 是否匹配该仓库的某个启用中的入站 webhook；匹配时记录最近使用时间。
func (s *Store) ResolveIncomingWebhook(owner, repo, token string) (bool, error) {
	if token == "" {
		return false, nil
	}
	var row incomingWebhookRow
	err := s.db.Where("owner = ? AND repo = ? AND token_hash = ? AND enabled = ?", owner, repo, incomingTokenHash(token), true).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	// best-effort：记录最近使用时间，失败不影响创建结果
	_ = s.db.Model(&incomingWebhookRow{}).Where("id = ?", row.ID).
		Update("last_used_at", now()).Error
	return true, nil
}
