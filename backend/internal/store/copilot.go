package store

// ---- BYOK（bring your own key）----

// ByokKey 用户配置的一个 BYOK 密钥；APIKey 永不回传，仅 KeySet 标记是否已填写。
type ByokKey struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Provider  string `json:"provider"`
	BaseURL   string `json:"base_url,omitempty"`
	Model     string `json:"model,omitempty"`
	KeySet    bool   `json:"key_set"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// ByokSecret BYOK 密钥明文（仅注入容器时在服务端内部使用）。
type ByokSecret struct {
	Provider string
	APIKey   string
	BaseURL  string
	Model    string
}

func byokRowToDTO(r byokKeyRow) ByokKey {
	return ByokKey{
		ID: r.ID, Name: r.Name, Provider: r.Provider, BaseURL: r.BaseURL, Model: r.Model,
		KeySet: r.APIKey != "", CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

// CreateByokKey 创建 BYOK 密钥。
func (s *Store) CreateByokKey(username, name, provider, apiKey, baseURL, model string) (ByokKey, error) {
	ts := now()
	row := byokKeyRow{
		Username: username, Name: name, Provider: provider, APIKey: apiKey,
		BaseURL: baseURL, Model: model, CreatedAt: ts, UpdatedAt: ts,
	}
	if err := s.db.Create(&row).Error; err != nil {
		return ByokKey{}, err
	}
	return byokRowToDTO(row), nil
}

// ListByokKeys 列出用户的全部 BYOK 密钥（不含明文）。
func (s *Store) ListByokKeys(username string) ([]ByokKey, error) {
	var rows []byokKeyRow
	if err := s.db.Where("username = ?", username).Order("id DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]ByokKey, 0, len(rows))
	for _, r := range rows {
		out = append(out, byokRowToDTO(r))
	}
	return out, nil
}

// GetByokKey 返回用户某个 BYOK 密钥（不含明文）；不存在返回 ErrNotFound。
func (s *Store) GetByokKey(username string, id int64) (ByokKey, error) {
	var row byokKeyRow
	if err := s.db.Where("username = ? AND id = ?", username, id).First(&row).Error; err != nil {
		return ByokKey{}, notFoundErr(err)
	}
	return byokRowToDTO(row), nil
}

// GetByokSecret 返回用户某个 BYOK 密钥的明文（服务端内部注入容器用）。
func (s *Store) GetByokSecret(username string, id int64) (ByokSecret, error) {
	var row byokKeyRow
	if err := s.db.Where("username = ? AND id = ?", username, id).First(&row).Error; err != nil {
		return ByokSecret{}, notFoundErr(err)
	}
	return ByokSecret{Provider: row.Provider, APIKey: row.APIKey, BaseURL: row.BaseURL, Model: row.Model}, nil
}

// UpdateByokKey 更新 BYOK 密钥；apiKey 为空表示保留原密钥。
func (s *Store) UpdateByokKey(username string, id int64, name, provider, apiKey, baseURL, model string) (ByokKey, error) {
	updates := map[string]any{
		"name": name, "provider": provider, "base_url": baseURL, "model": model, "updated_at": now(),
	}
	if apiKey != "" {
		updates["api_key"] = apiKey
	}
	res := s.db.Model(&byokKeyRow{}).Where("username = ? AND id = ?", username, id).Updates(updates)
	if res.Error != nil {
		return ByokKey{}, res.Error
	}
	if res.RowsAffected == 0 {
		return ByokKey{}, ErrNotFound
	}
	return s.GetByokKey(username, id)
}

// DeleteByokKey 删除 BYOK 密钥。
func (s *Store) DeleteByokKey(username string, id int64) error {
	res := s.db.Where("username = ? AND id = ?", username, id).Delete(&byokKeyRow{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- copilot sessions ----

// CopilotSession 仓库内的一个 AI copilot 会话。会话由进程内嵌的
// github.com/holihur/agent 驱动，工作区为仓库的本地克隆副本；每轮对话
// 结束后 gitdash 自动提交并推送改动到 Branch（闭环）。
type CopilotSession struct {
	ID        int64  `json:"id"`
	Owner     string `json:"-"`
	Repo      string `json:"-"`
	CreatedBy string `json:"created_by"`
	ByokID    int64  `json:"byok_id"`
	Prompt    string `json:"prompt"`
	Branch    string `json:"branch,omitempty"`
	HeadSHA   string `json:"head_sha,omitempty"`
	Status    string `json:"status"`
	Error     string `json:"error,omitempty"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

func copilotRowToDTO(r copilotSessionRow) CopilotSession {
	return CopilotSession{
		ID: r.ID, Owner: r.Owner, Repo: r.Repo, CreatedBy: r.CreatedBy, ByokID: r.ByokID,
		Prompt: r.Prompt, Branch: r.Branch, HeadSHA: r.HeadSHA, Status: r.Status, Error: r.Error,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

// CreateCopilotSession 创建一个 copilot 会话（初始 idle）。
func (s *Store) CreateCopilotSession(owner, repo, createdBy string, byokID int64, prompt string) (CopilotSession, error) {
	ts := now()
	row := copilotSessionRow{
		Owner: owner, Repo: repo, CreatedBy: createdBy, ByokID: byokID,
		Prompt: prompt, Status: "idle",
		CreatedAt: ts, UpdatedAt: ts,
	}
	if err := s.db.Create(&row).Error; err != nil {
		return CopilotSession{}, err
	}
	return copilotRowToDTO(row), nil
}

// GetCopilotSession 按 owner/repo/id 取会话（scope 校验由 API 层负责）。
func (s *Store) GetCopilotSession(owner, repo string, id int64) (CopilotSession, error) {
	var row copilotSessionRow
	if err := s.db.Where("owner = ? AND repo = ? AND id = ?", owner, repo, id).First(&row).Error; err != nil {
		return CopilotSession{}, notFoundErr(err)
	}
	return copilotRowToDTO(row), nil
}

// ListCopilotSessions 列出仓库全部会话。
func (s *Store) ListCopilotSessions(owner, repo string) ([]CopilotSession, error) {
	var rows []copilotSessionRow
	if err := s.db.Where("owner = ? AND repo = ?", owner, repo).Order("id DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]CopilotSession, 0, len(rows))
	for _, r := range rows {
		out = append(out, copilotRowToDTO(r))
	}
	return out, nil
}

// SetCopilotSessionStatus 更新会话状态与错误信息。
func (s *Store) SetCopilotSessionStatus(owner, repo string, id int64, status, errMsg string) error {
	res := s.db.Model(&copilotSessionRow{}).Where("owner = ? AND repo = ? AND id = ?", owner, repo, id).
		Updates(map[string]any{"status": status, "error": errMsg, "updated_at": now()})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// SetCopilotSessionGit 记录会话工作区分支与最近推送的提交。
func (s *Store) SetCopilotSessionGit(owner, repo string, id int64, branch, headSHA string) error {
	res := s.db.Model(&copilotSessionRow{}).Where("owner = ? AND repo = ? AND id = ?", owner, repo, id).
		Updates(map[string]any{"branch": branch, "head_sha": headSHA, "updated_at": now()})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteCopilotSession 删除会话记录。
func (s *Store) DeleteCopilotSession(owner, repo string, id int64) error {
	res := s.db.Where("owner = ? AND repo = ? AND id = ?", owner, repo, id).Delete(&copilotSessionRow{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// CopilotSessionsByByok 返回引用某 BYOK 密钥的会话 id（删除密钥前校验用）。
func (s *Store) CopilotSessionsByByok(username string, byokID int64) ([]int64, error) {
	var ids []int64
	err := s.db.Model(&copilotSessionRow{}).
		Where("created_by = ? AND byok_id = ?", username, byokID).
		Pluck("id", &ids).Error
	return ids, err
}
