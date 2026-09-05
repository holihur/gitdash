package store

import (
	"sort"

	"gorm.io/gorm"
)

func toRepo(r repoRow) Repo {
	return Repo{
		ID:          r.ID,
		Owner:       r.Owner,
		Name:        r.Name,
		Description: r.Description,
		Private:     r.Private,
		CreatedAt:   r.CreatedAt,
	}
}

func (s *Store) CreateRepo(owner, name, description string, private bool) (Repo, error) {
	row := repoRow{Owner: owner, Name: name, Description: description, Private: private, CreatedAt: now()}
	// 用 map 插入绕过 GORM 对 default 字段零值的改写（private=false 必须显式落库）
	if err := s.db.Table("repos").Create(map[string]any{
		"owner":       row.Owner,
		"name":        row.Name,
		"description": row.Description,
		"private":     private,
		"created_at":  row.CreatedAt,
	}).Error; err != nil {
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

// ExploreRepos 公开仓库（供发现页使用）。
func (s *Store) ExploreRepos() ([]Repo, error) {
	var rows []repoRow
	if err := s.db.Where("private = ?", false).Order("id DESC").Limit(100).Find(&rows).Error; err != nil {
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
	res := s.db.Model(&repoRow{}).Where("owner = ? AND name = ?", owner, name).
		Update("private", private)
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
			{&pipelineCfgRow{}, "owner = ? AND repo = ?"},
			{&pipelineRunRow{}, "owner = ? AND repo = ?"},
		}
		for _, d := range deletes {
			if err := tx.Where(d.cond, owner, name).Delete(d.model).Error; err != nil {
				return err
			}
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

func (s *Store) CanRead(owner, repo, username string) bool {
	if owner == username {
		return true
	}
	// 公开仓库直接放行（含组织仓库）
	if r, err := s.GetRepo(owner, repo); err == nil && !r.Private {
		return true
	}
	if s.IsOrg(owner) {
		if s.OrgRole(owner, username) != "" {
			return true
		}
	}
	var n int64
	err := s.db.Model(&collabRow{}).Where("owner = ? AND repo = ? AND username = ?", owner, repo, username).
		Limit(1).Count(&n).Error
	return err == nil && n > 0
}

func (s *Store) CanWrite(owner, repo, username string) bool {
	if owner == username {
		return true
	}
	if s.IsOrg(owner) {
		role := s.OrgRole(owner, username)
		if role == "owner" || role == "member" {
			return true
		}
	}
	var n int64
	err := s.db.Model(&collabRow{}).
		Where("owner = ? AND repo = ? AND username = ? AND permission = ?", owner, repo, username, "write").
		Limit(1).Count(&n).Error
	return err == nil && n > 0
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

// AccessibleRepos 返回用户自己拥有的仓库 + 所在组织的仓库 + 作为协作者可访问的仓库（带 role）。
func (s *Store) AccessibleRepos(username string) ([]Repo, error) {
	// 三段来源分别查询后在内存合并（与原 UNION ALL + CASE 语义一致），按 owner,name 排序
	var owned, orgRows []repoRow
	if err := s.db.Where("owner = ?", username).Find(&owned).Error; err != nil {
		return nil, err
	}
	if err := s.db.Select("repos.*").Joins("JOIN org_members m ON repos.owner = m.org").
		Where("m.username = ?", username).Find(&orgRows).Error; err != nil {
		return nil, err
	}
	var collabRows []struct {
		ID          int64
		Owner       string
		Name        string
		Description string
		Private     bool
		CreatedAt   string
		Perm        string `gorm:"column:permission"`
	}
	if err := s.db.Table("repo_collabs").Select(`repos.id AS id, repos.owner AS owner, repos.name AS name,
			repos.description AS description, repos.private AS private, repos.created_at AS created_at,
			repo_collabs.permission AS permission`).
		Joins("JOIN repos ON repos.owner = repo_collabs.owner AND repos.name = repo_collabs.repo").
		Where("repo_collabs.username = ?", username).Scan(&collabRows).Error; err != nil {
		return nil, err
	}
	// 组织成员对应的角色：owner → owner，其余 → write
	orgRole := map[string]string{}
	var members []orgMemberRow
	if err := s.db.Where("username = ?", username).Find(&members).Error; err != nil {
		return nil, err
	}
	for _, m := range members {
		if m.Role == "owner" {
			orgRole[m.Org] = "owner"
		} else if _, ok := orgRole[m.Org]; !ok {
			orgRole[m.Org] = "write"
		}
	}
	repos := []Repo{}
	for _, r := range owned {
		dto := toRepo(r)
		dto.Role = "owner"
		repos = append(repos, dto)
	}
	for _, r := range orgRows {
		dto := toRepo(r)
		dto.Role = orgRole[r.Owner]
		repos = append(repos, dto)
	}
	for _, r := range collabRows {
		dto := toRepo(repoRow{ID: r.ID, Owner: r.Owner, Name: r.Name, Description: r.Description, Private: r.Private, CreatedAt: r.CreatedAt})
		dto.Role = r.Perm
		repos = append(repos, dto)
	}
	// 按 owner, name 排序（稳定排序保持各来源内部相对顺序）
	sort.SliceStable(repos, func(i, j int) bool {
		if repos[i].Owner != repos[j].Owner {
			return repos[i].Owner < repos[j].Owner
		}
		return repos[i].Name < repos[j].Name
	})
	return repos, nil
}
