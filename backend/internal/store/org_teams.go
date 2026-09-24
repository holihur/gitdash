package store

import (
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// OrgTeam 组织团队（给仓库批量授权的载体）。
type OrgTeam struct {
	ID          int64  `json:"id"`
	Org         string `json:"org"`
	Name        string `json:"name"`
	MemberCount int    `json:"member_count"`
	CreatedAt   string `json:"created_at"`
}

// RepoTeamGrant 仓库上的团队授权。
type RepoTeamGrant struct {
	TeamID     int64  `json:"team_id"`
	TeamName   string `json:"team_name"`
	Permission string `json:"permission"`
}

// AccessEntry 权限审计条目：谁通过什么途径获得什么角色。
type AccessEntry struct {
	Subject string `json:"subject"`
	Role    string `json:"role"`
	Source  string `json:"source"` // owner | org_member | collaborator | team:<name>
}

// roleRankSQL 把角色字符串映射为排序等级（SQL 片段，供 CASE 复用）。
const roleRankSQL = `CASE %s
	WHEN 'admin' THEN 5
	WHEN 'maintain' THEN 4
	WHEN 'write' THEN 3
	WHEN 'triage' THEN 2
	WHEN 'read' THEN 1
	ELSE 0 END`

func rankToRole(rank int) string {
	switch {
	case rank >= 5:
		return RoleAdmin
	case rank == 4:
		return RoleMaintain
	case rank == 3:
		return RoleWrite
	case rank == 2:
		return RoleTriage
	case rank == 1:
		return RoleRead
	}
	return ""
}

// ---- team CRUD ----

func (s *Store) CreateOrgTeam(org, name string) (OrgTeam, error) {
	if _, err := s.GetOrg(org); err != nil {
		return OrgTeam{}, err
	}
	r := orgTeamRow{Org: org, Name: strings.TrimSpace(name), CreatedAt: now()}
	if err := s.db.Create(&r).Error; err != nil {
		if isUniqueErr(err) {
			return OrgTeam{}, ErrExists
		}
		return OrgTeam{}, err
	}
	return OrgTeam{ID: r.ID, Org: r.Org, Name: r.Name, CreatedAt: r.CreatedAt}, nil
}

func (s *Store) ListOrgTeams(org string) ([]OrgTeam, error) {
	var rows []orgTeamRow
	if err := s.db.Where("org = ?", org).Order("name").Find(&rows).Error; err != nil {
		return nil, err
	}
	counts := map[int64]int{}
	var crows []struct {
		TeamID int64
		N      int
	}
	_ = s.db.Table("org_team_members").Select("team_id, COUNT(*) AS n").Group("team_id").Scan(&crows).Error
	for _, r := range crows {
		counts[r.TeamID] = r.N
	}
	out := make([]OrgTeam, 0, len(rows))
	for _, r := range rows {
		out = append(out, OrgTeam{ID: r.ID, Org: r.Org, Name: r.Name, MemberCount: counts[r.ID], CreatedAt: r.CreatedAt})
	}
	return out, nil
}

// DeleteOrgTeam 删除团队并级联清理成员与仓库授权。
func (s *Store) DeleteOrgTeam(org string, id int64) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var row orgTeamRow
		if err := tx.Where("id = ? AND org = ?", id, org).First(&row).Error; err != nil {
			return notFoundErr(err)
		}
		if err := tx.Where("team_id = ?", id).Delete(&orgTeamMemberRow{}).Error; err != nil {
			return err
		}
		if err := tx.Where("team_id = ?", id).Delete(&repoTeamGrantRow{}).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", id).Delete(&orgTeamRow{}).Error
	})
}

// ---- team members ----

func (s *Store) teamOrg(id int64) (string, bool) {
	var row orgTeamRow
	if err := s.db.Select("org").Where("id = ?", id).First(&row).Error; err != nil {
		return "", false
	}
	return row.Org, true
}

func (s *Store) AddOrgTeamMember(org string, teamID int64, username string) error {
	if o, ok := s.teamOrg(teamID); !ok || o != org {
		return ErrNotFound
	}
	if _, err := s.GetByUsername(username); err != nil {
		return ErrNotFound
	}
	row := orgTeamMemberRow{TeamID: teamID, Username: username}
	return s.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error
}

func (s *Store) RemoveOrgTeamMember(org string, teamID int64, username string) error {
	if o, ok := s.teamOrg(teamID); !ok || o != org {
		return ErrNotFound
	}
	res := s.db.Where("team_id = ? AND username = ?", teamID, username).Delete(&orgTeamMemberRow{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) OrgTeamMembers(teamID int64) ([]string, error) {
	var rows []orgTeamMemberRow
	if err := s.db.Select("username").Where("team_id = ?", teamID).Order("username").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Username)
	}
	return out, nil
}

// ---- repo team grants ----

// GrantRepoTeam 给组织仓库授权一个团队（team 必须属于 owner 组织）。
func (s *Store) GrantRepoTeam(owner, repo string, teamID int64, permission string) error {
	if !ValidCollabRole(permission) {
		return ErrNotFound
	}
	if o, ok := s.teamOrg(teamID); !ok || o != owner {
		return ErrNotFound
	}
	if _, err := s.GetRepo(owner, repo); err != nil {
		return ErrNotFound
	}
	row := repoTeamGrantRow{Owner: owner, Repo: repo, TeamID: teamID, Permission: permission}
	return s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "owner"}, {Name: "repo"}, {Name: "team_id"}},
		DoUpdates: clause.Assignments(map[string]any{"permission": permission}),
	}).Create(&row).Error
}

func (s *Store) RevokeRepoTeam(owner, repo string, teamID int64) error {
	res := s.db.Where("owner = ? AND repo = ? AND team_id = ?", owner, repo, teamID).Delete(&repoTeamGrantRow{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// RepoTeamGrants 列出仓库上的团队授权。
func (s *Store) RepoTeamGrants(owner, repo string) ([]RepoTeamGrant, error) {
	var rows []repoTeamGrantRow
	if err := s.db.Where("owner = ? AND repo = ?", owner, repo).Order("team_id").Find(&rows).Error; err != nil {
		return nil, err
	}
	nameByID := map[int64]string{}
	var teams []orgTeamRow
	_ = s.db.Where("org = ?", owner).Find(&teams).Error
	for _, t := range teams {
		nameByID[t.ID] = t.Name
	}
	out := make([]RepoTeamGrant, 0, len(rows))
	for _, r := range rows {
		out = append(out, RepoTeamGrant{TeamID: r.TeamID, TeamName: nameByID[r.TeamID], Permission: r.Permission})
	}
	return out, nil
}

// RepoTeamRole 返回用户通过组织团队在仓库上获得的最高角色（无则 ""）。
func (s *Store) RepoTeamRole(owner, repo, username string) string {
	if username == "" {
		return ""
	}
	var rank int
	_ = s.db.Raw(`SELECT COALESCE(MAX(`+strings.Replace(roleRankSQL, "%s", "rtg.permission", 1)+`), 0)
		FROM repo_team_grants rtg
		JOIN org_team_members m ON m.team_id = rtg.team_id
		WHERE rtg.owner = ? AND rtg.repo = ? AND m.username = ?`,
		owner, repo, username).Scan(&rank).Error
	return rankToRole(rank)
}

// ---- access audit ----

// AccessEntries 汇总仓库的访问权限：所有者、组织成员默认角色、协作者、团队授权。
func (s *Store) AccessEntries(owner, repo string) ([]AccessEntry, error) {
	out := []AccessEntry{}
	if s.IsOrg(owner) {
		for _, m := range s.mustOrgMembers(owner) {
			if m.Role == RoleOwner {
				out = append(out, AccessEntry{Subject: m.Username, Role: RoleOwner, Source: "owner"})
			}
		}
		role := s.OrgDefaultMemberRole(owner)
		for _, m := range s.mustOrgMembers(owner) {
			if m.Role != RoleOwner {
				out = append(out, AccessEntry{Subject: m.Username, Role: role, Source: "org_member"})
			}
		}
	} else if owner != "" {
		out = append(out, AccessEntry{Subject: owner, Role: RoleOwner, Source: "owner"})
	}
	var collabs []collabRow
	if err := s.db.Where("owner = ? AND repo = ?", owner, repo).Order("username").Find(&collabs).Error; err != nil {
		return nil, err
	}
	for _, c := range collabs {
		out = append(out, AccessEntry{Subject: c.Username, Role: c.Permission, Source: "collaborator"})
	}
	grants, err := s.RepoTeamGrants(owner, repo)
	if err != nil {
		return nil, err
	}
	for _, g := range grants {
		members, _ := s.OrgTeamMembers(g.TeamID)
		for _, u := range members {
			out = append(out, AccessEntry{Subject: u, Role: g.Permission, Source: "team:" + g.TeamName})
		}
	}
	return out, nil
}

func (s *Store) mustOrgMembers(org string) []OrgMember {
	ms, err := s.OrgMembers(org)
	if err != nil {
		return nil
	}
	return ms
}
