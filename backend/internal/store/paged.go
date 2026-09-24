package store

import "sort"

// paged.go 汇总各“不分页列表”的分页变体与计数。
//
// 设计取舍：保留原有全量方法供内部逻辑（校验、聚合）调用，另外提供
// XxxPaged / CountXxx 给 API 列表端点，避免一次把整表载入内存并序列化。
// 所有分页方法都通过 paginate() 统一处理 limit/offset 语义（limit<=0 = 不限制）。

// ---- labels ----

func (s *Store) ListLabelsPaged(owner, repo string, limit, offset int) ([]Label, error) {
	var rows []repoLabelRow
	q := s.db.Where("owner = ? AND repo = ?", owner, repo).Order("name")
	if err := paginate(q, limit, offset).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := []Label{}
	for _, r := range rows {
		out = append(out, Label(r))
	}
	return out, nil
}

func (s *Store) CountLabels(owner, repo string) (int, error) {
	var n int64
	err := s.db.Model(&repoLabelRow{}).Where("owner = ? AND repo = ?", owner, repo).Count(&n).Error
	return int(n), err
}

// ---- milestones ----

func (s *Store) ListMilestonesPaged(owner, repo string, limit, offset int) ([]Milestone, error) {
	var rows []struct {
		ID           int64
		Owner        string
		Repo         string
		Title        string
		Description  string
		State        string
		CreatedAt    string
		OpenIssues   int
		ClosedIssues int
	}
	sql := `SELECT m.id, m.owner, m.repo, m.title, m.description, m.state, m.created_at,
		COALESCE(SUM(CASE WHEN i.state = 'open' THEN 1 ELSE 0 END), 0) AS open_issues,
		COALESCE(SUM(CASE WHEN i.state = 'closed' THEN 1 ELSE 0 END), 0) AS closed_issues
		FROM milestones m LEFT JOIN issues i ON i.milestone_id = m.id AND i.owner = m.owner AND i.repo = m.repo
		WHERE m.owner = ? AND m.repo = ? GROUP BY m.id ORDER BY m.title`
	args := []any{owner, repo}
	if limit > 0 {
		if offset < 0 {
			offset = 0
		}
		sql += ` LIMIT ? OFFSET ?`
		args = append(args, limit, offset)
	}
	if err := s.db.Raw(sql, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := []Milestone{}
	for _, r := range rows {
		out = append(out, Milestone{
			ID: r.ID, Owner: r.Owner, Repo: r.Repo, Title: r.Title,
			Description: r.Description, State: r.State, CreatedAt: r.CreatedAt,
			OpenIssues: r.OpenIssues, ClosedIssues: r.ClosedIssues,
		})
	}
	return out, nil
}

func (s *Store) CountMilestones(owner, repo string) (int, error) {
	var n int64
	err := s.db.Model(&milestoneRow{}).Where("owner = ? AND repo = ?", owner, repo).Count(&n).Error
	return int(n), err
}

// ---- webhooks ----

func (s *Store) ListWebhooksPaged(owner, repo string, limit, offset int) ([]Webhook, error) {
	var rows []webhookRow
	q := s.db.Where("owner = ? AND repo = ?", owner, repo).Order("id")
	if err := paginate(q, limit, offset).Find(&rows).Error; err != nil {
		return nil, err
	}
	ws := make([]Webhook, 0, len(rows))
	for _, r := range rows {
		w, err := toWebhook(r)
		if err != nil {
			return nil, err
		}
		ws = append(ws, w)
	}
	return ws, nil
}

func (s *Store) CountWebhooks(owner, repo string) (int, error) {
	var n int64
	err := s.db.Model(&webhookRow{}).Where("owner = ? AND repo = ?", owner, repo).Count(&n).Error
	return int(n), err
}

// ---- release assets ----

func (s *Store) ListAssetsPaged(owner, repo string, releaseID int64, limit, offset int) ([]ReleaseAsset, error) {
	var rows []releaseAssetRow
	q := s.db.Select("id, owner, repo, release_id, filename, size, created_at").
		Where("owner = ? AND repo = ? AND release_id = ?", owner, repo, releaseID).
		Order("id")
	if err := paginate(q, limit, offset).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]ReleaseAsset, 0, len(rows))
	for _, row := range rows {
		out = append(out, ReleaseAsset(row))
	}
	return out, nil
}

// ---- collaborators ----

func (s *Store) ListCollabsPaged(owner, repo string, limit, offset int) ([]Collab, error) {
	var rows []collabRow
	q := s.db.Where("owner = ? AND repo = ?", owner, repo).Order("username")
	if err := paginate(q, limit, offset).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := []Collab{}
	for _, r := range rows {
		out = append(out, Collab(r))
	}
	return out, nil
}

func (s *Store) CountCollabs(owner, repo string) (int, error) {
	var n int64
	err := s.db.Model(&collabRow{}).Where("owner = ? AND repo = ?", owner, repo).Count(&n).Error
	return int(n), err
}

// ---- repo env vars ----

func (s *Store) ListRepoEnvVarsPaged(owner, repo string, limit, offset int) ([]RepoEnvVar, error) {
	var rows []repoEnvVarRow
	q := s.db.Where("owner = ? AND repo = ?", owner, repo).Order("key")
	if err := paginate(q, limit, offset).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]RepoEnvVar, 0, len(rows))
	for _, r := range rows {
		out = append(out, RepoEnvVar{Key: r.Key, Value: r.Value, CreatedAt: r.CreatedAt})
	}
	return out, nil
}

func (s *Store) CountRepoEnvVars(owner, repo string) (int, error) {
	var n int64
	err := s.db.Model(&repoEnvVarRow{}).Where("owner = ? AND repo = ?", owner, repo).Count(&n).Error
	return int(n), err
}

// ---- copilot sessions ----

func (s *Store) ListCopilotSessionsPaged(owner, repo string, limit, offset int) ([]CopilotSession, error) {
	var rows []copilotSessionRow
	q := s.db.Where("owner = ? AND repo = ?", owner, repo).Order("id DESC")
	if err := paginate(q, limit, offset).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]CopilotSession, 0, len(rows))
	for _, r := range rows {
		out = append(out, copilotRowToDTO(r))
	}
	return out, nil
}

func (s *Store) CountCopilotSessions(owner, repo string) (int, error) {
	var n int64
	err := s.db.Model(&copilotSessionRow{}).Where("owner = ? AND repo = ?", owner, repo).Count(&n).Error
	return int(n), err
}

// ---- branch protections ----

func (s *Store) ListBranchProtectionsPaged(owner, repo string, limit, offset int) ([]BranchProtection, error) {
	var rows []branchProtectionRow
	q := s.db.Where("owner = ? AND repo = ?", owner, repo).Order("branch")
	if err := paginate(q, limit, offset).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]BranchProtection, 0, len(rows))
	for _, r := range rows {
		out = append(out, BranchProtection(r))
	}
	return out, nil
}

func (s *Store) CountBranchProtections(owner, repo string) (int, error) {
	var n int64
	err := s.db.Model(&branchProtectionRow{}).Where("owner = ? AND repo = ?", owner, repo).Count(&n).Error
	return int(n), err
}

// ---- ssh / gpg keys ----

func (s *Store) ListKeysPaged(username string, limit, offset int) ([]SSHKey, error) {
	var rows []sshKeyRow
	q := s.db.Table("ssh_keys").
		Select("ssh_keys.*").
		Joins("JOIN users ON users.id = ssh_keys.user_id").
		Where("users.username = ?", username).
		Order("ssh_keys.id DESC")
	if err := paginate(q, limit, offset).Scan(&rows).Error; err != nil {
		return nil, err
	}
	keys := []SSHKey{}
	for _, r := range rows {
		keys = append(keys, SSHKey{ID: r.ID, Name: r.Name, PublicKey: r.PublicKey, Fingerprint: r.Fingerprint, CreatedAt: r.CreatedAt})
	}
	return keys, nil
}

func (s *Store) CountKeys(username string) (int, error) {
	var n int64
	err := s.db.Table("ssh_keys").
		Joins("JOIN users ON users.id = ssh_keys.user_id").
		Where("users.username = ?", username).Count(&n).Error
	return int(n), err
}

func (s *Store) ListGPGKeysPaged(username string, limit, offset int) ([]GPGKey, error) {
	var rows []gpgKeyRow
	q := s.db.Table("gpg_keys").
		Select("gpg_keys.*").
		Joins("JOIN users ON users.id = gpg_keys.user_id").
		Where("users.username = ?", username).
		Order("gpg_keys.id")
	if err := paginate(q, limit, offset).Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := []GPGKey{}
	for _, r := range rows {
		out = append(out, GPGKey{ID: r.ID, Fingerprint: r.Fingerprint, CreatedAt: r.CreatedAt})
	}
	return out, nil
}

func (s *Store) CountGPGKeys(username string) (int, error) {
	var n int64
	err := s.db.Table("gpg_keys").
		Joins("JOIN users ON users.id = gpg_keys.user_id").
		Where("users.username = ?", username).Count(&n).Error
	return int(n), err
}

// ---- PAT / BYOK ----

func (s *Store) ListPATsPaged(userID int64, limit, offset int) ([]PAT, error) {
	var rows []patRow
	q := s.db.Where("user_id = ?", userID).Order("id DESC")
	if err := paginate(q, limit, offset).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := []PAT{}
	for _, r := range rows {
		out = append(out, PAT{ID: r.ID, Name: r.Name, Scopes: splitScopes(r.Scopes), CIDRs: splitCIDRs(r.CIDRs), ExpiresAt: r.ExpiresAt, CreatedAt: r.CreatedAt, LastUsedAt: r.LastUsedAt})
	}
	return out, nil
}

func (s *Store) CountPATs(userID int64) (int, error) {
	var n int64
	err := s.db.Model(&patRow{}).Where("user_id = ?", userID).Count(&n).Error
	return int(n), err
}

func (s *Store) ListByokKeysPaged(username string, limit, offset int) ([]ByokKey, error) {
	var rows []byokKeyRow
	q := s.db.Where("username = ?", username).Order("id DESC")
	if err := paginate(q, limit, offset).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]ByokKey, 0, len(rows))
	for _, r := range rows {
		out = append(out, byokRowToDTO(r))
	}
	return out, nil
}

func (s *Store) CountByokKeys(username string) (int, error) {
	var n int64
	err := s.db.Model(&byokKeyRow{}).Where("username = ?", username).Count(&n).Error
	return int(n), err
}

// ---- oauth apps / authorizations ----

func (s *Store) ListOAuthAppsPaged(userID int64, limit, offset int) ([]OAuthApp, error) {
	var rows []oauthAppRow
	q := s.db.Where("user_id = ?", userID).Order("id DESC")
	if err := paginate(q, limit, offset).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]OAuthApp, 0, len(rows))
	for _, r := range rows {
		out = append(out, OAuthApp{ID: r.ID, Name: r.Name, Homepage: r.Homepage, Description: r.Description, CallbackURL: r.CallbackURL, ClientID: r.ClientID, CreatedAt: r.CreatedAt})
	}
	return out, nil
}

func (s *Store) CountOAuthApps(userID int64) (int, error) {
	var n int64
	err := s.db.Model(&oauthAppRow{}).Where("user_id = ?", userID).Count(&n).Error
	return int(n), err
}

func (s *Store) ListOAuthAuthorizationsPaged(userID int64, limit, offset int) ([]OAuthAuthorization, error) {
	var rows []struct {
		ID         int64
		AppID      int64
		AppName    string
		Scopes     string
		CreatedAt  string
		LastUsedAt string
	}
	q := s.db.Table("pats").
		Select("pats.id, pats.oauth_app_id AS app_id, oauth_apps.name AS app_name, pats.scopes, pats.created_at, pats.last_used_at").
		Joins("JOIN oauth_apps ON oauth_apps.id = pats.oauth_app_id").
		Where("pats.user_id = ? AND pats.oauth_app_id <> 0", userID).
		Order("pats.id DESC")
	if err := paginate(q, limit, offset).Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]OAuthAuthorization, 0, len(rows))
	for _, r := range rows {
		out = append(out, OAuthAuthorization{ID: r.ID, AppID: r.AppID, AppName: r.AppName, Scopes: splitScopes(r.Scopes), CreatedAt: r.CreatedAt, LastUsedAt: r.LastUsedAt})
	}
	return out, nil
}

func (s *Store) CountOAuthAuthorizations(userID int64) (int, error) {
	var n int64
	err := s.db.Table("pats").
		Joins("JOIN oauth_apps ON oauth_apps.id = pats.oauth_app_id").
		Where("pats.user_id = ? AND pats.oauth_app_id <> 0", userID).Count(&n).Error
	return int(n), err
}

// ---- runners ----

// runnerScopesAll 返回 scope=""（全局）与给定 scopes 命中的 runner，按 id 倒序去重。
// scope 列表分块查询，避免绑定参数上限。
func (s *Store) runnerScopesAll(scopes []string) ([]runnerRow, error) {
	seen := map[string]bool{}
	out := []runnerRow{}
	add := func(rows []runnerRow) {
		for _, r := range rows {
			if !seen[r.Name] {
				seen[r.Name] = true
				out = append(out, r)
			}
		}
	}
	var globals []runnerRow
	if err := s.db.Where("scope = ?", "").Find(&globals).Error; err != nil {
		return nil, err
	}
	add(globals)
	for _, part := range chunkStrings(scopes, 0) {
		var rs []runnerRow
		if err := s.db.Where("scope IN ?", part).Find(&rs).Error; err != nil {
			return nil, err
		}
		add(rs)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out, nil
}

func (s *Store) ListRunnersByScopesPaged(scopes []string, limit, offset int) ([]Runner, error) {
	rows, err := s.runnerScopesAll(scopes)
	if err != nil {
		return nil, err
	}
	out := make([]Runner, 0)
	for _, r := range pageSliceRows(rows, limit, offset) {
		out = append(out, runnerRowToDTO(r))
	}
	return out, nil
}

func (s *Store) CountRunnersByScopes(scopes []string) (int, error) {
	rows, err := s.runnerScopesAll(scopes)
	if err != nil {
		return 0, err
	}
	return len(rows), nil
}

func (s *Store) ListAllRunnersPaged(limit, offset int) ([]Runner, error) {
	var rows []runnerRow
	q := s.db.Order("id DESC")
	if err := paginate(q, limit, offset).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]Runner, 0, len(rows))
	for _, r := range rows {
		out = append(out, runnerRowToDTO(r))
	}
	return out, nil
}

func (s *Store) CountAllRunners() (int, error) {
	var n int64
	err := s.db.Model(&runnerRow{}).Count(&n).Error
	return int(n), err
}

// ---- template repos ----

func (s *Store) ListAccessibleTemplateReposPaged(username string, limit, offset int) ([]Repo, error) {
	var rows []struct {
		ID            int64
		Owner         string
		Name          string
		Description   string
		Private       bool
		IsTemplate    bool
		Banned        bool
		DefaultBranch string
		HasIssues     bool
		CreatedAt     string
		RoleRank      int
	}
	sql := `SELECT r.id, r.owner, r.name, r.description, r.private, r.is_template, r.banned,
			r.default_branch, r.has_issues, r.created_at, t.role_rank
		FROM (` + accessibleReposSubquery + `) t
		JOIN repos r ON r.id = t.id
		WHERE r.is_template = ?
		ORDER BY r.owner, r.name`
	// 子查询含 4 个用户名占位（自有 / 组织成员 / 协作者 / 团队），再加 is_template。
	args := []any{username, username, username, username, false, true, true}
	if limit > 0 {
		if offset < 0 {
			offset = 0
		}
		sql += ` LIMIT ? OFFSET ?`
		args = append(args, limit, offset)
	}
	if err := s.db.Raw(sql, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]Repo, 0, len(rows))
	for _, r := range rows {
		dto := toRepo(repoRow{
			ID: r.ID, Owner: r.Owner, Name: r.Name, Description: r.Description,
			Private: r.Private, IsTemplate: r.IsTemplate, Banned: r.Banned,
			DefaultBranch: r.DefaultBranch, HasIssues: r.HasIssues, CreatedAt: r.CreatedAt,
		})
		dto.Role = roleFromRank(r.RoleRank)
		out = append(out, dto)
	}
	return out, nil
}

func (s *Store) CountAccessibleTemplateRepos(username string) (int, error) {
	var n int64
	row := s.db.Raw(`SELECT COUNT(*) FROM (`+accessibleReposSubquery+`) t
		JOIN repos r ON r.id = t.id WHERE r.is_template = ?`,
		username, username, username, username, false, true, true).Row()
	if err := row.Scan(&n); err != nil {
		return 0, err
	}
	return int(n), nil
}

// pageSliceRows 对内存切片做 limit/offset（limit<=0 = 不限制）。
func pageSliceRows[T any](s []T, limit, offset int) []T {
	if offset < 0 {
		offset = 0
	}
	if offset >= len(s) {
		return []T{}
	}
	s = s[offset:]
	if limit > 0 && limit < len(s) {
		s = s[:limit]
	}
	return s
}
