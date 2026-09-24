package store

import (
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
	if _, err := s.CreateOrgTeam("acme", "backend"); err != ErrExists {
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
	// 审计条目包含团队来源。
	entries, err := s.AccessEntries("acme", "api")
	if err != nil {
		t.Fatal(err)
	}
	foundTeam := false
	for _, e := range entries {
		if e.Subject == "bob" && e.Source == "team:backend" && e.Role == RoleWrite {
			foundTeam = true
		}
	}
	if !foundTeam {
		t.Fatalf("access entries missing team source: %+v", entries)
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
