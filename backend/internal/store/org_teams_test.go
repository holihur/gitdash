package store

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestOrgTeams(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range []string{"alice", "bob"} {
		if _, err := s.CreateUser(u, u+"-pass-123456"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.CreateOrg("acme", "ACME", "alice"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateRepo("acme", "api", "", true); err != nil {
		t.Fatal(err)
	}

	team, err := s.CreateOrgTeam("acme", "backend")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateOrgTeam("acme", "backend"); !errors.Is(err, ErrExists) {
		t.Fatalf("duplicate team -> %v", err)
	}
	if err := s.AddOrgTeamMember("acme", team.ID, "bob"); err != nil {
		t.Fatal(err)
	}
	// 团队成员无授权前无权限（私有仓库）。
	if got := s.RepoRole("acme", "api", "bob"); got != "" {
		t.Fatalf("before grant role = %q", got)
	}
	// 授权团队 write。
	if err := s.GrantRepoTeam("acme", "api", team.ID, RoleWrite); err != nil {
		t.Fatal(err)
	}
	if got := s.RepoRole("acme", "api", "bob"); got != RoleWrite {
		t.Fatalf("team role = %q, want write", got)
	}
	if !s.CanDo("acme", "api", "bob", RoleWrite) {
		t.Fatal("team member should have write")
	}
	// 协作者权限与团队取最高。
	if err := s.UpsertCollab("acme", "api", "bob", RoleRead); err != nil {
		t.Fatal(err)
	}
	if got := s.RepoRole("acme", "api", "bob"); got != RoleWrite {
		t.Fatalf("max(collab,team) = %q, want write", got)
	}
	// 审计条目按 subject 聚合：bob 只有一条，有效角色 write，来源含组织成员 / 协作者 / 团队。
	entries, err := s.AccessEntries("acme", "api")
	if err != nil {
		t.Fatal(err)
	}
	var bob *AccessEntry
	bobCount := 0
	for i := range entries {
		if entries[i].Subject == "bob" {
			bob = &entries[i]
			bobCount++
		}
	}
	if bobCount != 1 || bob == nil {
		t.Fatalf("bob should have exactly one aggregated entry, got %d: %+v", bobCount, entries)
	}
	if bob.Role != RoleWrite {
		t.Fatalf("bob effective role = %q, want write", bob.Role)
	}
	hasSource := func(want string) bool {
		for _, src := range bob.Sources {
			if src == want {
				return true
			}
		}
		return false
	}
	for _, want := range []string{"collaborator", "team:backend"} {
		if !hasSource(want) {
			t.Fatalf("bob sources missing %q: %+v", want, bob.Sources)
		}
	}
	// 移除成员后失去团队权限（仍有 read 协作者）。
	if err := s.RemoveOrgTeamMember("acme", team.ID, "bob"); err != nil {
		t.Fatal(err)
	}
	if got := s.RepoRole("acme", "api", "bob"); got != RoleRead {
		t.Fatalf("after removal role = %q, want read (collab)", got)
	}
	// 删除团队级联清理授权。
	if err := s.DeleteOrgTeam("acme", team.ID); err != nil {
		t.Fatal(err)
	}
	grants, _ := s.RepoTeamGrants("acme", "api")
	if len(grants) != 0 {
		t.Fatalf("grants should be gone: %+v", grants)
	}
}

func TestAccessibleReposViaTeam(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range []string{"alice", "bob"} {
		if _, err := s.CreateUser(u, u+"-pass-123456"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.CreateOrg("acme", "ACME", "alice"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateRepo("acme", "api", "", true); err != nil {
		t.Fatal(err)
	}
	team, _ := s.CreateOrgTeam("acme", "devs")
	_ = s.AddOrgTeamMember("acme", team.ID, "bob")
	_ = s.GrantRepoTeam("acme", "api", team.ID, RoleWrite)

	repos, err := s.AccessibleRepos("bob", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 1 || repos[0].Name != "api" || repos[0].Role != RoleWrite {
		t.Fatalf("accessible via team = %+v", repos)
	}
}

func TestAccessEntriesMergesSources(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range []string{"alice", "bob"} {
		if _, err := s.CreateUser(u, u+"-pass-123456"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.CreateOrg("acme", "ACME", "alice"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetOrgInfo("acme", "", "", RoleRead); err != nil {
		t.Fatal(err)
	}
	if err := s.AddOrgMember("acme", "bob", "member"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateRepo("acme", "api", "", true); err != nil {
		t.Fatal(err)
	}
	// 协作者 triage、团队 write：有效角色应取三者最高的 write。
	if err := s.UpsertCollab("acme", "api", "bob", RoleTriage); err != nil {
		t.Fatal(err)
	}
	team, _ := s.CreateOrgTeam("acme", "devs")
	_ = s.AddOrgTeamMember("acme", team.ID, "bob")
	_ = s.GrantRepoTeam("acme", "api", team.ID, RoleWrite)

	entries, err := s.AccessEntries("acme", "api")
	if err != nil {
		t.Fatal(err)
	}
	bobCount := 0
	var bob AccessEntry
	for _, e := range entries {
		if e.Subject == "bob" {
			bob = e
			bobCount++
		}
	}
	if bobCount != 1 {
		t.Fatalf("bob entries = %d, want 1 (merged): %+v", bobCount, entries)
	}
	if bob.Role != RoleWrite {
		t.Fatalf("effective role = %q, want write", bob.Role)
	}
	want := map[string]bool{"org_member": false, "collaborator": false, "team:devs": false}
	for _, src := range bob.Sources {
		want[src] = true
	}
	for k, ok := range want {
		if !ok {
			t.Fatalf("missing source %q in %+v", k, bob.Sources)
		}
	}
}

// 团队授权应同时出现在「可访问仓库」的各个派生查询里：模板列表、代码搜索候选。
// 回归：accessibleReposSubquery 增加团队分支后，paged.go / search.go 的占位符数量曾失配。
func TestAccessibleListsCoverTeams(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range []string{"alice", "bob"} {
		if _, err := s.CreateUser(u, u+"-pass-123456"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.CreateOrg("acme", "ACME", "alice"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateRepo("acme", "api", "", true); err != nil {
		t.Fatal(err)
	}
	if err := s.SetRepoTemplate("acme", "api", true); err != nil {
		t.Fatal(err)
	}
	team, _ := s.CreateOrgTeam("acme", "devs")
	_ = s.AddOrgTeamMember("acme", team.ID, "bob")
	_ = s.GrantRepoTeam("acme", "api", team.ID, RoleWrite)

	templates, err := s.ListAccessibleTemplateReposPaged("bob", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(templates) != 1 || templates[0].Name != "api" || templates[0].Role != RoleWrite {
		t.Fatalf("templates via team = %+v", templates)
	}
	if n, err := s.CountAccessibleTemplateRepos("bob"); err != nil || n != 1 {
		t.Fatalf("count templates = %d, err %v", n, err)
	}

	candidates, err := s.CodeSearchRepos("bob", 10)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range candidates {
		if r.Owner == "acme" && r.Name == "api" {
			found = true
		}
	}
	if !found {
		t.Fatalf("code search candidates missing team repo: %+v", candidates)
	}
}
