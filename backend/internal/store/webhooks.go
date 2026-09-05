package store

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
	return nil
}
