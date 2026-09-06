package store

import (
	"errors"

	"gorm.io/gorm"
)

// CreateWebhook 新建仓库 webhook（同 owner+repo+url 重复返回 ErrExists）。
func (s *Store) CreateWebhook(owner, repo, url, secret string) (Webhook, error) {
	w := Webhook{Owner: owner, Repo: repo, URL: url, Secret: secret, CreatedAt: now()}
	row := webhookRow{Owner: owner, Repo: repo, URL: url, Secret: secret, CreatedAt: w.CreatedAt}
	if err := s.db.Create(&row).Error; err != nil {
		if isUniqueErr(err) {
			return w, ErrExists
		}
		return w, err
	}
	w.ID = row.ID
	return w, nil
}

// ListWebhooks 列出仓库全部 webhook（按 id 升序）。
func (s *Store) ListWebhooks(owner, repo string) ([]Webhook, error) {
	var rows []webhookRow
	if err := s.db.Where("owner = ? AND repo = ?", owner, repo).Order("id").Find(&rows).Error; err != nil {
		return nil, err
	}
	ws := make([]Webhook, 0, len(rows))
	for _, r := range rows {
		ws = append(ws, Webhook(r))
	}
	return ws, nil
}

// DeleteWebhook 删除指定 webhook（不存在返回 ErrNotFound）。
func (s *Store) DeleteWebhook(owner, repo string, id int64) error {
	res := s.db.Where("owner = ? AND repo = ? AND id = ?", owner, repo, id).Delete(&webhookRow{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	_ = s.db.Where("hook_id = ?", id).Delete(&webhookDeliveryRow{}).Error
	return nil
}

// RecordDelivery 落一条投递记录（首次投递 attempts=1；重试在 UpdateDelivery 上累加）。
func (s *Store) RecordDelivery(hookID int64, event, payload, status string, code int, errMsg string, nextRetry string) (int64, error) {
	row := webhookDeliveryRow{
		HookID: hookID, Event: event, Payload: payload, Status: status,
		Code: code, Error: errMsg, Attempts: 1, NextRetry: nextRetry, CreatedAt: now(),
	}
	if err := s.db.Create(&row).Error; err != nil {
		return 0, err
	}
	return row.ID, nil
}

// UpdateDelivery 更新投递记录（重试后调用；attempts+1）。
func (s *Store) UpdateDelivery(id int64, status string, code int, errMsg, nextRetry string) error {
	return s.db.Model(&webhookDeliveryRow{}).Where("id = ?", id).
		Updates(map[string]any{"status": status, "code": code, "error": errMsg, "next_retry": nextRetry, "attempts": gorm.Expr("attempts + 1")}).Error
}

// DueRetries 取出到期待重试的投递记录（status=retry 且 next_retry <= nowStr，最旧优先）。
func (s *Store) DueRetries(nowStr string, limit int) ([]WebhookDelivery, error) {
	if limit <= 0 {
		limit = 50
	}
	var rows []webhookDeliveryRow
	if err := s.db.Where("status = ? AND next_retry <> '' AND next_retry <= ?", "retry", nowStr).
		Order("id").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]WebhookDelivery, 0, len(rows))
	for _, r := range rows {
		out = append(out, WebhookDelivery{
			ID: r.ID, HookID: r.HookID, Event: r.Event, Status: r.Status,
			Code: r.Code, Error: r.Error, Attempts: r.Attempts, NextRetry: r.NextRetry, CreatedAt: r.CreatedAt,
		})
	}
	return out, nil
}

// GetDeliveryPayload 取投递记录的事件 payload（重试时需要）。
func (s *Store) GetDeliveryPayload(id int64) (string, error) {
	var row webhookDeliveryRow
	if err := s.db.Select("payload").First(&row, "id = ?", id).Error; err != nil {
		return "", err
	}
	return row.Payload, nil
}

// GetWebhookByID 取单个 webhook（重试时需要 URL 与 secret）。
func (s *Store) GetWebhookByID(id int64) (Webhook, bool, error) {
	var row webhookRow
	err := s.db.First(&row, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Webhook{}, false, nil
	}
	if err != nil {
		return Webhook{}, false, err
	}
	return Webhook(row), true, nil
}

// ListDeliveries 列出某 webhook 最近的投递记录（校验 hook 归属；id 降序）。
func (s *Store) ListDeliveries(owner, repo string, hookID, limit int64) ([]WebhookDelivery, error) {
	var hook webhookRow
	if err := s.db.Where("owner = ? AND repo = ? AND id = ?", owner, repo, hookID).First(&hook).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var rows []webhookDeliveryRow
	if err := s.db.Where("hook_id = ?", hookID).Order("id desc").Limit(int(limit)).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]WebhookDelivery, 0, len(rows))
	for _, r := range rows {
		out = append(out, WebhookDelivery{
			ID: r.ID, HookID: r.HookID, Event: r.Event, Status: r.Status,
			Code: r.Code, Error: r.Error, Attempts: r.Attempts, NextRetry: r.NextRetry, CreatedAt: r.CreatedAt,
		})
	}
	return out, nil
}

// PruneDeliveries 删除早于 older（RFC3339）的投递记录，返回删除行数。
func (s *Store) PruneDeliveries(older string) (int64, error) {
	res := s.db.Where("created_at < ? AND status <> ?", older, "retry").Delete(&webhookDeliveryRow{})
	return res.RowsAffected, res.Error
}
