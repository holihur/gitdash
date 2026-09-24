package store

import (
	"path/filepath"
	"testing"
)

func TestOrgDefaultMemberRole(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range []string{"alice", "carol"} {
		if _, err := s.CreateUser(u, u+"-pass-123456"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.CreateOrg("acme", "ACME", "alice"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateRepo("acme", "r", "", true); err != nil {
		t.Fatal(err)
	}
	if err := s.AddOrgMember("acme", "carol", "member"); err != nil {
		t.Fatal(err)
	}
	// 默认成员角色 = write。
	if got := s.RepoRole("acme", "r", "carol"); got != RoleWrite {
		t.Fatalf("default member role = %q, want write", got)
	}
	// 改为 triage 后，成员仅能管理议题、不能推送。
	if err := s.SetOrgInfo("acme", "ACME", "", RoleTriage); err != nil {
		t.Fatal(err)
	}
	if got := s.OrgDefaultMemberRole("acme"); got != RoleTriage {
		t.Fatalf("org default = %q, want triage", got)
	}
	if got := s.RepoRole("acme", "r", "carol"); got != RoleTriage {
		t.Fatalf("member role = %q, want triage", got)
	}
	if s.CanWrite("acme", "r", "carol") || !s.CanDo("acme", "r", "carol", RoleTriage) {
		t.Fatal("triage member capability mismatch")
	}
	// 新建的仓库同样适用。
	if _, err := s.CreateRepo("acme", "r2", "", true); err != nil {
		t.Fatal(err)
	}
	if got := s.RepoRole("acme", "r2", "carol"); got != RoleTriage {
		t.Fatalf("new repo member role = %q, want triage", got)
	}
	// owner 不受影响。
	if got := s.RepoRole("acme", "r", "alice"); got != RoleOwner {
		t.Fatalf("owner role = %q", got)
	}
}

func TestRepoRoles(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range []string{"alice", "bob", "carol"} {
		if _, err := s.CreateUser(u, u+"-pass-123456"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.CreateRepo("alice", "priv", "", true); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateRepo("alice", "pub", "", false); err != nil {
		t.Fatal(err)
	}

	// 公开仓库：登录用户与匿名都可读。
	if got := s.RepoRole("alice", "pub", "bob"); got != RoleRead {
		t.Fatalf("public role = %q, want read", got)
	}
	if got := s.RepoRole("alice", "pub", ""); got != RoleRead {
		t.Fatalf("anon public role = %q, want read", got)
	}
	// 私有仓库：无授权为空。
	if got := s.RepoRole("alice", "priv", "bob"); got != "" {
		t.Fatalf("private role = %q, want empty", got)
	}
	// owner 天然最高。
	if got := s.RepoRole("alice", "priv", "alice"); got != RoleOwner {
		t.Fatalf("owner role = %q", got)
	}

	// 协作者五档角色。
	for _, role := range []string{RoleRead, RoleTriage, RoleWrite, RoleMaintain, RoleAdmin} {
		if err := s.UpsertCollab("alice", "priv", "bob", role); err != nil {
			t.Fatal(err)
		}
		if got := s.RepoRole("alice", "priv", "bob"); got != role {
			t.Fatalf("collab %s -> role %q", role, got)
		}
	}

	// 能力阈值：triage 满足 triage，但不满足 write。
	if err := s.UpsertCollab("alice", "priv", "bob", RoleTriage); err != nil {
		t.Fatal(err)
	}
	if !s.CanDo("alice", "priv", "bob", RoleTriage) {
		t.Fatal("triage should satisfy triage")
	}
	if s.CanDo("alice", "priv", "bob", RoleWrite) || s.CanWrite("alice", "priv", "bob") {
		t.Fatal("triage must not satisfy write")
	}
	if !s.CanRead("alice", "priv", "bob") {
		t.Fatal("triage can read")
	}
	// admin 满足 maintain，但不等于 owner。
	if err := s.UpsertCollab("alice", "priv", "bob", RoleAdmin); err != nil {
		t.Fatal(err)
	}
	if !s.CanDo("alice", "priv", "bob", RoleMaintain) || s.CanDo("alice", "priv", "bob", RoleOwner) {
		t.Fatal("admin should satisfy maintain but not owner")
	}

	// 组织：成员 = write，owner = owner。
	if _, err := s.CreateOrg("acme", "ACME", "alice"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateRepo("acme", "r", "", true); err != nil {
		t.Fatal(err)
	}
	if err := s.AddOrgMember("acme", "carol", "member"); err != nil {
		t.Fatal(err)
	}
	if err := s.AddOrgMember("acme", "bob", "owner"); err != nil {
		t.Fatal(err)
	}
	if got := s.RepoRole("acme", "r", "carol"); got != RoleWrite {
		t.Fatalf("org member role = %q, want write", got)
	}
	if got := s.RepoRole("acme", "r", "bob"); got != RoleOwner {
		t.Fatalf("org owner role = %q, want owner", got)
	}

	// 角色等级与可授予校验。
	if !ValidCollabRole(RoleAdmin) || ValidCollabRole(RoleOwner) || ValidCollabRole("bogus") {
		t.Fatal("ValidCollabRole mismatch")
	}
	if !RoleAtLeast(RoleMaintain, RoleWrite) || RoleAtLeast(RoleTriage, RoleWrite) {
		t.Fatal("RoleAtLeast mismatch")
	}
}
