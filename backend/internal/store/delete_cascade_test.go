package store

import "testing"

// TestDeleteRepoCascadesRelatedRows 覆盖安全审计 F-24：删除仓库需级联清理
// 包/标签/审计、secrets、merge queue、团队授权、看板、copilot、流水线调度、
// badge、issue 事件/订阅/指派等关联行。
func TestDeleteRepoCascadesRelatedRows(t *testing.T) {
	s := openBanStore(t)
	if _, err := s.CreateUser("alice", "hash"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateRepo("alice", "r1", "", false); err != nil {
		t.Fatal(err)
	}
	issue, err := s.CreateIssue("alice", "r1", "alice", "t", "b")
	if err != nil {
		t.Fatal(err)
	}
	label, err := s.CreateLabel("alice", "r1", "custom-label", "ffffff")
	if err != nil {
		t.Fatal(err)
	}
	mustCreate(t, s,
		&issueAssigneeRow{IssueID: issue.ID, Username: "bob"},
		&issueLabelRow{IssueID: issue.ID, LabelID: label.ID},
		&issueEventRow{Owner: "alice", Repo: "r1", Kind: "issue", Number: issue.Number, Actor: "alice", Action: "opened", CreatedAt: now()},
		&issueSubscriberRow{Owner: "alice", Repo: "r1", Kind: "issue", Number: issue.Number, Username: "bob", CreatedAt: now()},
		&repoSecretRow{Owner: "alice", Repo: "r1", Name: "K", CreatedAt: now(), UpdatedAt: now()},
		&mergeQueueRow{Owner: "alice", Repo: "r1", Branch: "main", Number: 1, EnqueuedBy: "alice", EnqueuedAt: now()},
		&repoTeamGrantRow{Owner: "alice", Repo: "r1", TeamID: 1, Permission: "read"},
		&copilotSessionRow{Owner: "alice", Repo: "r1", CreatedBy: "alice"},
		&pipelineScheduleRow{Owner: "alice", Repo: "r1", File: ".gitdash.yml", Expr: "* * * * *"},
		&badgeGrantRow{BadgeID: 1, Kind: "repo", Owner: "alice", Repo: "r1", CreatedAt: now()},
	)
	proj := projectRow{Owner: "alice", Repo: "r1", Name: "p", CreatedAt: now()}
	mustCreate(t, s, &proj)
	col := projectColumnRow{ProjectID: proj.ID, Name: "c"}
	swim := projectSwimlaneRow{ProjectID: proj.ID, Name: "s"}
	mustCreate(t, s, &col, &swim)
	card := projectCardRow{ProjectID: proj.ID, ColumnID: col.ID, SwimlaneID: swim.ID, Title: "x", CreatedAt: now()}
	mustCreate(t, s, &card)
	mustCreate(t, s,
		&projectCardAssigneeRow{CardID: card.ID, Username: "bob"},
		&projectCardLabelRow{CardID: card.ID, LabelID: label.ID},
		&packageRow{Owner: "alice", Repo: "r1", Type: "npm", Name: "pkg", Version: "1.0.0", Filename: "f", CreatedAt: now()},
		&packageTagRow{Owner: "alice", Type: "npm", Name: "pkg", Tag: "latest", Version: "1.0.0"},
		&packageAuditRow{Owner: "alice", Type: "npm", Name: "pkg", Version: "1.0.0", Action: "publish", Actor: "alice", CreatedAt: now()},
	)

	if err := s.DeleteRepo("alice", "r1"); err != nil {
		t.Fatal(err)
	}
	checks := []struct {
		name  string
		model any
		cond  string
		args  []any
	}{
		{"issues", &issueRow{}, "owner = ? AND repo = ?", []any{"alice", "r1"}},
		{"issue_assignees", &issueAssigneeRow{}, "username = ?", []any{"bob"}},
		{"issue_labels", &issueLabelRow{}, "1 = 1", nil},
		{"issue_events", &issueEventRow{}, "owner = ? AND repo = ?", []any{"alice", "r1"}},
		{"issue_subscribers", &issueSubscriberRow{}, "owner = ? AND repo = ?", []any{"alice", "r1"}},
		{"repo_secrets", &repoSecretRow{}, "owner = ? AND repo = ?", []any{"alice", "r1"}},
		{"merge_queue", &mergeQueueRow{}, "owner = ? AND repo = ?", []any{"alice", "r1"}},
		{"repo_team_grants", &repoTeamGrantRow{}, "owner = ? AND repo = ?", []any{"alice", "r1"}},
		{"projects", &projectRow{}, "owner = ? AND repo = ?", []any{"alice", "r1"}},
		{"project_columns", &projectColumnRow{}, "1 = 1", nil},
		{"project_swimlanes", &projectSwimlaneRow{}, "1 = 1", nil},
		{"project_cards", &projectCardRow{}, "1 = 1", nil},
		{"project_card_assignees", &projectCardAssigneeRow{}, "1 = 1", nil},
		{"project_card_labels", &projectCardLabelRow{}, "1 = 1", nil},
		{"copilot_sessions", &copilotSessionRow{}, "owner = ? AND repo = ?", []any{"alice", "r1"}},
		{"pipeline_schedules", &pipelineScheduleRow{}, "owner = ? AND repo = ?", []any{"alice", "r1"}},
		{"badge_grants", &badgeGrantRow{}, "owner = ? AND repo = ?", []any{"alice", "r1"}},
		{"packages", &packageRow{}, "owner = ? AND repo = ?", []any{"alice", "r1"}},
		{"package_tags", &packageTagRow{}, "owner = ? AND name = ?", []any{"alice", "pkg"}},
		{"package_audits", &packageAuditRow{}, "owner = ? AND name = ?", []any{"alice", "pkg"}},
	}
	for _, c := range checks {
		var n int64
		q := s.db.Model(c.model)
		if c.cond != "" {
			q = q.Where(c.cond, c.args...)
		}
		if err := q.Count(&n).Error; err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Fatalf("%s still has %d row(s) after DeleteRepo", c.name, n)
		}
	}
}

// TestDeleteOrgClearsTeamState 覆盖安全审计 F-21：删除组织需清理团队/授权，
// 且 RepoTeamRole 在团队行残留时也不再生效（纵深防御）。
func TestDeleteOrgClearsTeamState(t *testing.T) {
	s := openBanStore(t)
	if _, err := s.CreateUser("mallory", "hash"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateUser("owner", "hash"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateOrg("acme", "Acme", "owner"); err != nil {
		t.Fatal(err)
	}
	team, err := s.CreateOrgTeam("acme", "T")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AddOrgTeamMember("acme", team.ID, "mallory"); err != nil {
		t.Fatal(err)
	}
	// grant 要求仓库存在；创建后授权，再删除仓库（会顺带清 grants）。
	if _, err := s.CreateRepo("acme", "api", "", true); err != nil {
		t.Fatal(err)
	}
	if err := s.GrantRepoTeam("acme", "api", team.ID, "write"); err != nil {
		t.Fatal(err)
	}
	if role := s.RepoTeamRole("acme", "api", "mallory"); role != "write" {
		t.Fatalf("before delete role = %q, want write", role)
	}
	if err := s.DeleteRepo("acme", "api"); err != nil {
		t.Fatal(err)
	}
	// 模拟历史缺陷残留的授权行：DeleteOrg 必须清理。
	mustCreate(t, s, &repoTeamGrantRow{Owner: "acme", Repo: "api", TeamID: team.ID, Permission: "write"})
	if err := s.DeleteOrg("acme"); err != nil {
		t.Fatal(err)
	}
	for name, model := range map[string]any{
		"org_teams":        &orgTeamRow{},
		"org_team_members": &orgTeamMemberRow{},
		"repo_team_grants": &repoTeamGrantRow{},
		"org_follows":      &orgFollowRow{},
	} {
		var n int64
		if err := s.db.Model(model).Count(&n).Error; err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Fatalf("%s still has %d row(s) after DeleteOrg", name, n)
		}
	}
	// 纵深防御：即使残留 grant/member 行，缺少 org_teams 行时也不再授权。
	mustCreate(t, s, &repoTeamGrantRow{Owner: "acme", Repo: "api", TeamID: 999, Permission: "write"})
	mustCreate(t, s, &orgTeamMemberRow{TeamID: 999, Username: "mallory"})
	if role := s.RepoTeamRole("acme", "api", "mallory"); role != "" {
		t.Fatalf("stale grant revived a role: %q", role)
	}
}

// TestCountRepoPinsIgnoresStale 覆盖 F-24：已删除仓库的陈旧 pin 不占用置顶配额。
func TestCountRepoPinsIgnoresStale(t *testing.T) {
	s := openBanStore(t)
	if _, err := s.CreateUser("alice", "hash"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateRepo("alice", "live", "", false); err != nil {
		t.Fatal(err)
	}
	uid, err := s.UserID("alice")
	if err != nil {
		t.Fatal(err)
	}
	mustCreate(t, s,
		&repoPinRow{UserID: uid, Owner: "alice", Repo: "live", Position: 0, CreatedAt: now()},
		&repoPinRow{UserID: uid, Owner: "alice", Repo: "gone", Position: 1, CreatedAt: now()},
	)
	n, err := s.CountRepoPins("alice")
	if err != nil || n != 1 {
		t.Fatalf("CountRepoPins = %d err=%v, want 1", n, err)
	}
}
