package store

import (
	"errors"
	"path/filepath"
	"testing"
)

func openBanStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "ban.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	return s
}

func TestUserBanToggle(t *testing.T) {
	s := openBanStore(t)
	if _, err := s.CreateUser("alice", "x"); err != nil {
		t.Fatal(err)
	}
	if s.IsUserBanned("alice") {
		t.Fatal("new user should not be banned")
	}
	if err := s.SetUserBanned("alice", true); err != nil {
		t.Fatal(err)
	}
	if !s.IsUserBanned("alice") {
		t.Fatal("user should be banned")
	}
	if err := s.SetUserBanned("alice", false); err != nil {
		t.Fatal(err)
	}
	if s.IsUserBanned("alice") {
		t.Fatal("user should be unbanned")
	}
	// 不存在的用户返回 ErrNotFound
	if err := s.SetUserBanned("ghost", true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("ban missing user = %v, want ErrNotFound", err)
	}
}

func TestRepoBanBlocksAccess(t *testing.T) {
	s := openBanStore(t)
	if _, err := s.CreateUser("alice", "x"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateUser("bob", "x"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateRepo("alice", "proj", "", false); err != nil {
		t.Fatal(err)
	}
	// 公开仓库 bob 可读
	if !s.CanRead("alice", "proj", "bob") {
		t.Fatal("bob should read public repo")
	}
	if err := s.SetRepoBanned("alice", "proj", true); err != nil {
		t.Fatal(err)
	}
	// 封禁后整体禁止访问（含 owner）
	if s.CanRead("alice", "proj", "bob") {
		t.Fatal("banned repo should not be readable by others")
	}
	if s.CanRead("alice", "proj", "alice") {
		t.Fatal("banned repo should not be readable by owner")
	}
	if s.CanWrite("alice", "proj", "alice") {
		t.Fatal("banned repo should not be writable by owner")
	}
	// 不出现在 Explore
	repos, err := s.ExploreRepos(0, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range repos {
		if r.Owner == "alice" && r.Name == "proj" {
			t.Fatal("banned repo should be hidden from Explore")
		}
	}
}

func TestOrgBanPropagatesToRepos(t *testing.T) {
	s := openBanStore(t)
	if _, err := s.CreateUser("alice", "x"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateOrg("team", "Team", "alice"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateRepo("team", "svc", "", false); err != nil {
		t.Fatal(err)
	}
	if !s.CanRead("team", "svc", "bob") {
		t.Fatal("public org repo should be readable")
	}
	if err := s.SetOrgBanned("team", true); err != nil {
		t.Fatal(err)
	}
	if !s.IsOrgBanned("team") {
		t.Fatal("org should be banned")
	}
	if !s.IsRepoBanned("team", "svc") {
		t.Fatal("org ban should mark its repos banned")
	}
	if s.CanRead("team", "svc", "bob") {
		t.Fatal("org-banned repo should not be readable")
	}
	// 成员列表中隐藏
	orgs, err := s.ListMyOrgs("alice", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range orgs {
		if o.Name == "team" {
			t.Fatal("banned org should be hidden from member list")
		}
	}
	// org owner 通知对象
	owners := s.OrgOwners("team")
	if len(owners) != 1 || owners[0] != "alice" {
		t.Fatalf("OrgOwners = %v, want [alice]", owners)
	}
}

func TestEnsureTemplateUserAndRepos(t *testing.T) {
	s := openBanStore(t)
	if err := s.EnsureTemplateUser("hash"); err != nil {
		t.Fatal(err)
	}
	// 幂等
	if err := s.EnsureTemplateUser("hash2"); err != nil {
		t.Fatal(err)
	}
	if !s.IsUserBanned(TemplateUser) {
		t.Fatal("template user must be banned")
	}
	// 名下仓库自动 is_template + 公开
	repo, err := s.CreateRepo(TemplateUser, "starter", "", true)
	if err != nil {
		t.Fatal(err)
	}
	if !repo.IsTemplate {
		t.Fatal("template user repo must be is_template")
	}
	if repo.Private {
		t.Fatal("template user repo must be public")
	}
	got, err := s.GetRepo(TemplateUser, "starter")
	if err != nil {
		t.Fatal(err)
	}
	if !got.IsTemplate || got.Private {
		t.Fatalf("persisted template repo = %+v", got)
	}
}

func TestAdminListReposAndOrgs(t *testing.T) {
	s := openBanStore(t)
	if _, err := s.CreateUser("alice", "x"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateRepo("alice", "one", "", false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateRepo("alice", "two", "", true); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateOrg("team", "Team", "alice"); err != nil {
		t.Fatal(err)
	}
	repos, total, err := s.AdminListRepos("alice", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 || len(repos) != 2 {
		t.Fatalf("AdminListRepos = %d/%d, want 2/2", len(repos), total)
	}
	filtered, ftotal, err := s.AdminListRepos("two", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if ftotal != 1 || len(filtered) != 1 || filtered[0].Name != "two" {
		t.Fatalf("filtered repos = %+v (total %d)", filtered, ftotal)
	}
	orgs, ototal, err := s.AdminListOrgs("team", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if ototal != 1 || len(orgs) != 1 || orgs[0].Name != "team" {
		t.Fatalf("AdminListOrgs = %+v (total %d)", orgs, ototal)
	}
	if err := s.SetOrgBanned("team", true); err != nil {
		t.Fatal(err)
	}
	orgs, _, _ = s.AdminListOrgs("team", 10, 0)
	if len(orgs) != 1 || !orgs[0].Banned {
		t.Fatalf("admin org list should expose banned flag: %+v", orgs)
	}
}
