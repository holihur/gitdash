package store

import "testing"

func containsStr(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

// TestNotifyRecipientsFiltersRevokedAccess 覆盖安全审计 M1.4：撤销协作者/成员后，
// 不再收到私有仓库通知（即使 watch/订阅行仍在）。
func TestNotifyRecipientsFiltersRevokedAccess(t *testing.T) {
	s := openBanStore(t)
	for _, u := range []string{"alice", "bob"} {
		if _, err := s.CreateUser(u, "hash"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.CreateRepo("alice", "secret", "", true); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertCollab("alice", "secret", "bob", "read"); err != nil {
		t.Fatal(err)
	}
	if err := s.WatchRepo("bob", "alice", "secret"); err != nil {
		t.Fatal(err)
	}
	if got := s.NotifyRecipients("alice", "secret", "alice"); !containsStr(got, "bob") {
		t.Fatalf("bob should be notified while collaborator: %v", got)
	}
	if err := s.RemoveCollab("alice", "secret", "bob"); err != nil {
		t.Fatal(err)
	}
	if got := s.NotifyRecipients("alice", "secret", "alice"); containsStr(got, "bob") {
		t.Fatalf("bob must not be notified after access revoked: %v", got)
	}
}

// TestDeleteUserClearsTeamMembershipAndDeployKey 覆盖安全审计 M1.5：删号清理团队
// 成员行与作为 deploy key 的公钥，防止用户名复用继承仓库权限。
func TestDeleteUserClearsTeamMembershipAndDeployKey(t *testing.T) {
	s := openBanStore(t)
	if _, err := s.CreateUser("alice", "hash"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateUser("bobby", "hash"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateOrg("acme", "Acme", "bobby"); err != nil {
		t.Fatal(err)
	}
	team, err := s.CreateOrgTeam("acme", "devs")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AddOrgTeamMember("acme", team.ID, "alice"); err != nil {
		t.Fatal(err)
	}
	aid, err := s.UserID("alice")
	if err != nil {
		t.Fatal(err)
	}
	mustCreate(t, s,
		&sshKeyRow{UserID: aid, Name: "k", PublicKey: "pub", Fingerprint: "SHA256:deadbeef", CreatedAt: now()},
		&deployKeyRow{Owner: "bobby", Repo: "r2", Name: "d", PublicKey: "pub", Fingerprint: "SHA256:deadbeef", Permission: "read", CreatedAt: now()},
		&orgFollowRow{Follower: "alice", Org: "acme", CreatedAt: now()},
		&issueSubscriberRow{Owner: "bobby", Repo: "r2", Kind: "issue", Number: 1, Username: "alice", CreatedAt: now()},
		&mergeQueueRow{Owner: "bobby", Repo: "r2", Branch: "main", Number: 1, EnqueuedBy: "alice", EnqueuedAt: now()},
	)

	if err := s.DeleteUserAccount("alice"); err != nil {
		t.Fatal(err)
	}
	checks := []struct {
		name  string
		model any
		cond  string
		arg   any
	}{
		{"org_team_members", &orgTeamMemberRow{}, "username = ?", "alice"},
		{"deploy_keys", &deployKeyRow{}, "fingerprint = ?", "SHA256:deadbeef"},
		{"org_follows", &orgFollowRow{}, "follower = ?", "alice"},
		{"issue_subscribers", &issueSubscriberRow{}, "username = ?", "alice"},
		{"merge_queue", &mergeQueueRow{}, "enqueued_by = ?", "alice"},
	}
	for _, c := range checks {
		var n int64
		if err := s.db.Model(c.model).Where(c.cond, c.arg).Count(&n).Error; err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Fatalf("%s still has %d row(s) after delete", c.name, n)
		}
	}
}

// TestRemoveOrgMemberClearsTeamRows 覆盖 M1.5：移除组织成员必须同时清理团队行。
func TestRemoveOrgMemberClearsTeamRows(t *testing.T) {
	s := openBanStore(t)
	if _, err := s.CreateUser("alice", "hash"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateUser("bobby", "hash"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateOrg("acme", "Acme", "bobby"); err != nil {
		t.Fatal(err)
	}
	if err := s.AddOrgMember("acme", "alice", "member"); err != nil {
		t.Fatal(err)
	}
	team, err := s.CreateOrgTeam("acme", "devs")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AddOrgTeamMember("acme", team.ID, "alice"); err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveOrgMember("acme", "alice"); err != nil {
		t.Fatal(err)
	}
	var n int64
	if err := s.db.Model(&orgTeamMemberRow{}).Where("username = ?", "alice").Count(&n).Error; err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("team membership still present after RemoveOrgMember: %d", n)
	}
}
