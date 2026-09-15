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
		Owner:      r.Owner,
		Repo:       r.Repo,
		CreatedAt:  r.CreatedAt,
		LastUsedAt: r.LastUsedAt,
	}
}

// SetIncomingWebhook 创建或轮换仓库的入站 webhook token，返回明文 token（仅此一次）。
// 每个仓库同时只保留一个 token（重新调用即轮换）。
func (s *Store) SetIncomingWebhook(owner, repo string) (string, IncomingWebhook, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", IncomingWebhook{}, err
	}
	token := hex.EncodeToString(raw)
	row := incomingWebhookRow{
		Owner: owner, Repo: repo, TokenHash: incomingTokenHash(token), CreatedAt: now(),
	}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("owner = ? AND repo = ?", owner, repo).Delete(&incomingWebhookRow{}).Error; err != nil {
			return err
		}
		return tx.Create(&row).Error
	})
	if err != nil {
		return "", IncomingWebhook{}, err
	}
	return token, toIncomingWebhook(row), nil
}

// GetIncomingWebhook 返回仓库入站 webhook 的元信息（未配置时 ok=false）。
func (s *Store) GetIncomingWebhook(owner, repo string) (IncomingWebhook, bool, error) {
	var row incomingWebhookRow
	err := s.db.Where("owner = ? AND repo = ?", owner, repo).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return IncomingWebhook{}, false, nil
	}
	if err != nil {
		return IncomingWebhook{}, false, err
	}
	return toIncomingWebhook(row), true, nil
}

// DeleteIncomingWebhook 删除仓库入站 webhook（不存在返回 ErrNotFound）。
func (s *Store) DeleteIncomingWebhook(owner, repo string) error {
	res := s.db.Where("owner = ? AND repo = ?", owner, repo).Delete(&incomingWebhookRow{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// ResolveIncomingWebhook 校验 token 是否匹配该仓库的入站 webhook；匹配时记录最近使用时间。
func (s *Store) ResolveIncomingWebhook(owner, repo, token string) (bool, error) {
	if token == "" {
		return false, nil
	}
	var row incomingWebhookRow
	err := s.db.Where("owner = ? AND repo = ? AND token_hash = ?", owner, repo, incomingTokenHash(token)).
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
