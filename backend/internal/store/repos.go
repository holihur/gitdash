package store

import (
	"gorm.io/gorm"
)

func toRepo(r repoRow) Repo {
	def := r.DefaultBranch
	if def == "" {
		def = "main"
	}
	vis := r.Visibility
	if vis == "" {
		if r.Private {
			vis = "private"
		} else {
			vis = "public"
		}
	}
	return Repo{
		ID:            r.ID,
		Owner:         r.Owner,
		Name:          r.Name,
		Description:   r.Description,
		Private:       vis == "private",
		Visibility:    vis,
		IsTemplate:    r.IsTemplate,
		Banned:        r.Banned,
		DefaultBranch: def,
		HasIssues:     r.HasIssues,
		PagesEnabled:  r.PagesEnabled,
		PagesBranch:   r.PagesBranch,
		PagesDir:      r.PagesDir,
		CreatedAt:     r.CreatedAt,
	}
}

func (s *Store) CreateRepo(owner, name, description string, private bool) (Repo, error) {
	createMu.Lock()
	defer createMu.Unlock()
	if err := s.checkRepoQuota(owner); err != nil {
		return Repo{}, err
	}
	// 系统专用模版用户：其名下仓库强制公开 + 标记为模版。
	isTemplate := owner == TemplateUser
	if isTemplate {
		private = false
	}
	row := repoRow{Owner: owner, Name: name, Description: description, Private: private, IsTemplate: isTemplate, DefaultBranch: "main", HasIssues: true, CreatedAt: now()}
	vis := "private"
	if !private {
		vis = "public"
	}
	// 用 map 插入绕过 GORM 对 default 字段零值的改写（private=false 必须显式落库）
	// 仓库与默认标签同一事务：任一失败则整体回滚，避免出现无初始标签的半成品仓库。
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Table("repos").Create(map[string]any{
			"owner":          row.Owner,
			"name":           row.Name,
			"description":    row.Description,
			"private":        private,
			"visibility":     vis,
			"is_template":    isTemplate,
			"default_branch": row.DefaultBranch,
			"has_issues":     row.HasIssues,
			"created_at":     row.CreatedAt,
		}).Error; err != nil {
			return err
		}
		return tx.Create(defaultLabelRows(owner, name)).Error
	})
	if err != nil {
		if isUniqueErr(err) {
			return toRepo(row), ErrExists
		}
		return toRepo(row), err
	}
	if r, err := s.GetRepo(owner, name); err == nil {
		return r, nil
	}
	return toRepo(row), nil
}

func (s *Store) ListRepos(owner string) ([]Repo, error) {
	var rows []repoRow
	if err := s.db.Where("owner = ?", owner).Order("name").Find(&rows).Error; err != nil {
		return nil, err
	}
	repos := []Repo{}
	for _, r := range rows {
		repos = append(repos, toRepo(r))
	}
	return repos, nil
}

// ListReposPaged 某 owner（用户或组织）下的仓库，分页按 name 排序。
func (s *Store) ListReposPaged(owner string, limit, offset int) ([]Repo, error) {
	var rows []repoRow
	q := s.db.Where("owner = ? AND banned = ?", owner, false).Order("name")
	if err := paginate(q, limit, offset).Find(&rows).Error; err != nil {
		return nil, err
	}
	repos := []Repo{}
	for _, r := range rows {
		repos = append(repos, toRepo(r))
	}
	return repos, nil
}

// CountRepos 某 owner 下未封禁仓库总数。
func (s *Store) CountRepos(owner string) (int, error) {
	var n int64
	err := s.db.Model(&repoRow{}).Where("owner = ? AND banned = ?", owner, false).Count(&n).Error
	return int(n), err
}

// ExploreRepos 分页列出公开仓库（供发现页使用）；limit<=0 表示不限制。
func (s *Store) ExploreRepos(limit, offset int) ([]Repo, error) {
	q := s.db.Where("private = ? AND banned = ? AND owner NOT IN (SELECT name FROM orgs WHERE banned = ?)",
		false, false, true).Order("id DESC")
	if limit > 0 {
		q = q.Limit(limit).Offset(offset)
	}
	var rows []repoRow
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	repos := []Repo{}
	for _, r := range rows {
		repos = append(repos, toRepo(r))
	}
	return repos, nil
}

// SetRepoPrivate 切换可见性（仅 owner 调用）。
func (s *Store) SetRepoPrivate(owner, name string, private bool) error {
	vis := "public"
	if private {
		vis = "private"
	}
	return s.SetRepoVisibility(owner, name, vis)
}

// SetRepoVisibility 设置仓库可见性：private | public | anonymous（仅 owner 调用）。
func (s *Store) SetRepoVisibility(owner, name, visibility string) error {
	res := s.db.Model(&repoRow{}).Where("owner = ? AND name = ?", owner, name).
		Updates(map[string]any{"visibility": visibility, "private": visibility == "private"})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// SetRepoTemplate 切换模版仓库标记（仅 owner 调用）。
func (s *Store) SetRepoTemplate(owner, name string, isTemplate bool) error {
	res := s.db.Model(&repoRow{}).Where("owner = ? AND name = ?", owner, name).
		Update("is_template", isTemplate)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// SetRepoDefaultBranch 修改仓库默认分支（仅 owner 调用）。
func (s *Store) SetRepoDefaultBranch(owner, name, branch string) error {
	res := s.db.Model(&repoRow{}).Where("owner = ? AND name = ?", owner, name).
		Update("default_branch", branch)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// SetRepoHasIssues 开启/关闭仓库 issue 功能（仅 owner 调用）。
func (s *Store) SetRepoHasIssues(owner, name string, hasIssues bool) error {
	res := s.db.Model(&repoRow{}).Where("owner = ? AND name = ?", owner, name).
		Update("has_issues", hasIssues)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// SetRepoPages 更新仓库静态网站托管配置（仅 owner 调用）。
// branch/dir 为空时分别回退默认分支/仓库根目录。
func (s *Store) SetRepoPages(owner, name string, enabled bool, branch, dir string) error {
	res := s.db.Model(&repoRow{}).Where("owner = ? AND name = ?", owner, name).
		Updates(map[string]any{
			"pages_enabled": enabled,
			"pages_branch":  branch,
			"pages_dir":     dir,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// SetRepoDescription 修改仓库描述（仅 owner 调用；空字符串表示清空）。
func (s *Store) SetRepoDescription(owner, name, description string) error {
	res := s.db.Model(&repoRow{}).Where("owner = ? AND name = ?", owner, name).
		Update("description", description)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) GetRepo(owner, name string) (Repo, error) {
	var row repoRow
	err := s.db.Where("owner = ? AND name = ?", owner, name).First(&row).Error
	if err != nil {
		return Repo{}, notFoundErr(err)
	}
	return toRepo(row), nil
}

func (s *Store) DeleteRepo(owner, name string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		deletes := []struct {
			model any
			cond  string
		}{
			{&issueRow{}, "owner = ? AND repo = ?"},
			{&repoCounterRow{}, "owner = ? AND repo = ?"},
			{&repoLanguageRow{}, "owner = ? AND repo = ?"},
			{&repoLanguageMetaRow{}, "owner = ? AND repo = ?"},
			{&repoLabelRow{}, "owner = ? AND repo = ?"},
			{&milestoneRow{}, "owner = ? AND repo = ?"},
			{&starRow{}, "owner = ? AND repo = ?"},
			{&watchRow{}, "owner = ? AND repo = ?"},
			{&notificationRow{}, "owner = ? AND repo = ?"},
			{&forkRow{}, "owner = ? AND repo = ?"},
			{&forkRow{}, "source_owner = ? AND source_repo = ?"},
			{&importRow{}, "owner = ? AND repo = ?"},
			{&mirrorRow{}, "owner = ? AND repo = ?"},
			{&collabRow{}, "owner = ? AND repo = ?"},
			{&webhookRow{}, "owner = ? AND repo = ?"},
			{&pullRequestRow{}, "owner = ? AND repo = ?"},
			{&commentRow{}, "owner = ? AND repo = ?"},
			{&pullReviewRow{}, "owner = ? AND repo = ?"},
			{&releaseRow{}, "owner = ? AND repo = ?"},
			{&releaseAssetRow{}, "owner = ? AND repo = ?"},
			{&branchProtectionRow{}, "owner = ? AND repo = ?"},
			{&refNoteRow{}, "owner = ? AND repo = ?"},
			{&pipelineCfgRow{}, "owner = ? AND repo = ?"},
			{&pipelineRunRow{}, "owner = ? AND repo = ?"},
			{&repoEnvVarRow{}, "owner = ? AND repo = ?"},
			{&incomingWebhookRow{}, "owner = ? AND repo = ?"},
			{&deployKeyRow{}, "owner = ? AND repo = ?"},
			{&repoCommitRuleRow{}, "owner = ? AND repo = ?"},
		}
		for _, d := range deletes {
			if err := tx.Where(d.cond, owner, name).Delete(d.model).Error; err != nil {
				return err
			}
		}
		// webhook 投递记录按 hook_id 关联，需在 hooks 删除前用子查询清理
		if err := tx.Exec(
			"DELETE FROM webhook_deliveries WHERE hook_id IN (SELECT id FROM webhooks WHERE owner = ? AND repo = ?)",
			owner, name,
		).Error; err != nil {
			return err
		}
		res := tx.Where("owner = ? AND name = ?", owner, name).Delete(&repoRow{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrNotFound
		}
		return nil
	})
}

// ---- issues ----
func (s *Store) OwnedByName(username, name string) (string, error) {
	var row repoRow
	err := s.db.Select("owner").Where("owner = ? AND name = ?", username, name).First(&row).Error
	if err != nil {
		return "", notFoundErr(err)
	}
	return row.Owner, nil
}

// SharedByName 返回用户以协作者身份可访问的、指定名称的仓库 owner（同名多仓库取其一）。
func (s *Store) SharedByName(username, name string) (string, error) {
	var row collabRow
	err := s.db.Select("owner").Where("username = ? AND repo = ?", username, name).
		Order("owner").First(&row).Error
	if err != nil {
		return "", notFoundErr(err)
	}
	return row.Owner, nil
}

// RepoRole 返回用户在仓库中的有效角色（"" 表示无权限）。
// 优先级：owner（本人 / 组织 owner）> 组织成员 write > 协作者权限 > 公开仓库 read。
func (s *Store) RepoRole(owner, repo, username string) string {
	if s.IsRepoBanned(owner, repo) {
		return ""
	}
	if username != "" && owner == username {
		return RoleOwner
	}
	if username != "" && s.IsOrg(owner) {
		switch s.OrgRole(owner, username) {
		case RoleOwner:
			return RoleOwner
		case "member":
			return s.OrgDefaultMemberRole(owner)
		}
	}
	best := ""
	if username != "" {
		var row collabRow
		if err := s.db.Where("owner = ? AND repo = ? AND username = ?", owner, repo, username).
			First(&row).Error; err == nil && ValidCollabRole(row.Permission) {
			best = row.Permission
		}
		// 组织团队授权：取协作者与团队中的最高角色。
		if tr := s.RepoTeamRole(owner, repo, username); RoleRank(tr) > RoleRank(best) {
			best = tr
		}
	}
	if best != "" {
		return best
	}
	// 公开仓库：任何访问者（含匿名）至少 read。
	if r, err := s.GetRepo(owner, repo); err == nil && !r.Private {
		return RoleRead
	}
	return ""
}

// CanRead 报告用户（可为匿名）是否可读仓库。
func (s *Store) CanRead(owner, repo, username string) bool {
	return s.RepoRole(owner, repo, username) != ""
}

// CanWrite 报告用户是否具备写代码权限（>= write）。
func (s *Store) CanWrite(owner, repo, username string) bool {
	return RoleAtLeast(s.RepoRole(owner, repo, username), RoleWrite)
}

// CanDo 报告用户在仓库中的角色是否达到 min 等级。
func (s *Store) CanDo(owner, repo, username, min string) bool {
	return RoleAtLeast(s.RepoRole(owner, repo, username), min)
}

// IsRepoOwner owner 语义：用户本人，或该用户是仓库所属组织的 owner。
func (s *Store) IsRepoOwner(owner, username string) bool {
	if owner == username {
		return true
	}
	if s.IsOrg(owner) {
		return s.OrgRole(owner, username) == "owner"
	}
	return false
}

// QueryOrgRepos 组织的全部仓库。
func (s *Store) QueryOrgRepos(org string) ([]Repo, error) {
	return s.ListRepos(org)
}

// CountExploreRepos 公开仓库总数。
func (s *Store) CountExploreRepos() (int, error) {
	var n int64
	if err := s.db.Model(&repoRow{}).
		Where("private = ? AND banned = ? AND owner NOT IN (SELECT name FROM orgs WHERE banned = ?)", false, false, true).
		Count(&n).Error; err != nil {
		return 0, err
	}
	return int(n), nil
}

// accessibleReposSubquery 是“用户可访问仓库 id + 权限等级”的 UNION 子查询。
// 三段来源：自有仓库 / 所属组织仓库 / 协作者仓库；用 MAX(role_rank) 去重取最高权限。
// banned 仓库与被封禁组织的仓库会被过滤，保证列表与计数口径完全一致。
// role_rank：6=owner，5=admin，4=maintain，3=write，2=triage，1=read。
const accessibleReposSubquery = `SELECT r.id, MAX(src.role_rank) AS role_rank
	FROM (
		SELECT repos.owner AS owner, repos.name AS name, 6 AS role_rank
			FROM repos WHERE repos.owner = ?
		UNION ALL
		SELECT repos.owner AS owner, repos.name AS name,
			CASE WHEN m.role = 'owner' THEN 6
				ELSE COALESCE((SELECT CASE o.default_member_role
					WHEN 'admin' THEN 5
					WHEN 'maintain' THEN 4
					WHEN 'write' THEN 3
					WHEN 'triage' THEN 2
					WHEN 'read' THEN 1
					ELSE 3 END FROM orgs o WHERE o.name = repos.owner), 3)
			END AS role_rank
			FROM repos JOIN org_members m ON repos.owner = m.org WHERE m.username = ?
		UNION ALL
		SELECT repo_collabs.owner AS owner, repo_collabs.repo AS name,
			CASE repo_collabs.permission
				WHEN 'admin' THEN 5
				WHEN 'maintain' THEN 4
				WHEN 'write' THEN 3
				WHEN 'triage' THEN 2
				ELSE 1 END AS role_rank
			FROM repo_collabs WHERE repo_collabs.username = ?
		UNION ALL
		SELECT rtg.owner AS owner, rtg.repo AS name,
			CASE rtg.permission
				WHEN 'admin' THEN 5
				WHEN 'maintain' THEN 4
				WHEN 'write' THEN 3
				WHEN 'triage' THEN 2
				ELSE 1 END AS role_rank
			FROM repo_team_grants rtg
			JOIN org_team_members tm ON tm.team_id = rtg.team_id
			WHERE tm.username = ?
	) src
	JOIN repos r ON r.owner = src.owner AND r.name = src.name
	WHERE r.banned = ? AND NOT EXISTS (SELECT 1 FROM orgs o WHERE o.name = r.owner AND o.banned = ?)
	GROUP BY r.id`

func roleFromRank(rank int) string {
	switch {
	case rank >= 6:
		return RoleOwner
	case rank == 5:
		return RoleAdmin
	case rank == 4:
		return RoleMaintain
	case rank == 3:
		return RoleWrite
	case rank == 2:
		return RoleTriage
	default:
		return RoleRead
	}
}

// AccessibleRepos 返回用户自己拥有的仓库 + 所在组织的仓库 + 作为协作者可访问的仓库（带 role）。
// limit<=0 表示不限制；分页在数据库层完成，不再把全部仓库载入内存。
func (s *Store) AccessibleRepos(username string, limit, offset int) ([]Repo, error) {
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
		ORDER BY r.owner, r.name`
	args := []any{username, username, username, username, false, true}
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
	repos := make([]Repo, 0, len(rows))
	for _, r := range rows {
		dto := toRepo(repoRow{
			ID: r.ID, Owner: r.Owner, Name: r.Name, Description: r.Description,
			Private: r.Private, IsTemplate: r.IsTemplate, Banned: r.Banned,
			DefaultBranch: r.DefaultBranch, HasIssues: r.HasIssues, CreatedAt: r.CreatedAt,
		})
		dto.Role = roleFromRank(r.RoleRank)
		repos = append(repos, dto)
	}
	return repos, nil
}

// CountAccessibleRepos 可访问仓库总数（与 AccessibleRepos 使用同一子查询，口径完全一致）。
func (s *Store) CountAccessibleRepos(username string) (int, error) {
	var n int64
	row := s.db.Raw(`SELECT COUNT(*) FROM (`+accessibleReposSubquery+`) t`,
		username, username, username, username, false, true).Row()
	if err := row.Scan(&n); err != nil {
		return 0, err
	}
	return int(n), nil
}

// ListAccessibleTemplateRepos 返回当前用户可访问的、标记为模版的仓库。
func (s *Store) ListAccessibleTemplateRepos(username string) ([]Repo, error) {
	repos, err := s.AccessibleRepos(username, 0, 0)
	if err != nil {
		return nil, err
	}
	out := []Repo{}
	for _, r := range repos {
		if r.IsTemplate {
			out = append(out, r)
		}
	}
	return out, nil
}
