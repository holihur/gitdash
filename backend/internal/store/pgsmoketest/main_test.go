package pgsmoketest

import (
	"errors"
	"os"
	"testing"

	"gitdash/backend/internal/store"
)

// TestPGSmoke 在真实 PostgreSQL 上跑一遍跨后端一致性清单里的关键路径：
// 布尔零值、唯一冲突映射、部分唯一索引（空邮箱）、ON CONFLICT upsert、
// 大小写不敏感的过滤，以及基础会话/仓库 CRUD。
func TestPGSmoke(t *testing.T) {
	if os.Getenv("GITDASH_DB") == "" {
		t.Skip("set GITDASH_DB to run")
	}
	s, err := store.OpenDSN(os.Getenv("GITDASH_DB"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	// 幂等：清空相关表
	for _, tbl := range []string{
		"registry_manifests", "packages", "login_fails", "settings",
		"repo_counters", "issues", "pull_requests",
		"repo_stars", "repo_watches", "sessions", "repos", "org_members", "orgs", "users",
	} {
		if derr := s.DB().Exec("DELETE FROM " + tbl).Error; derr != nil {
			t.Fatalf("clean %s: %v", tbl, derr)
		}
	}

	u, err := s.CreateUser("alice", "h")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	// 布尔零值：private=false 必须显式落库
	if _, err := s.CreateRepo("alice", "ci", "", false); err != nil {
		t.Fatalf("create repo: %v", err)
	}
	rr, err := s.GetRepo("alice", "ci")
	if err != nil {
		t.Fatalf("get repo: %v", err)
	}
	if rr.Private {
		t.Fatal("private should be false")
	}
	// 唯一冲突 → ErrExists
	if _, err := s.CreateRepo("alice", "ci", "", false); !errors.Is(err, store.ErrExists) {
		t.Fatalf("duplicate repo err = %v, want ErrExists", err)
	}

	// 仓库级编号计数器：INSERT ... ON CONFLICT ... RETURNING 在 PG 上原子自增，删空后不复用
	var lastIssue int64
	for i := int64(1); i <= 3; i++ {
		it, err := s.CreateIssue("alice", "ci", "alice", "t", "b")
		if err != nil {
			t.Fatalf("create issue %d: %v", i, err)
		}
		if it.Number != i {
			t.Fatalf("issue number = %d, want %d", it.Number, i)
		}
		lastIssue = it.Number
	}
	for n := int64(1); n <= lastIssue; n++ {
		if err := s.DeleteIssue("alice", "ci", n); err != nil {
			t.Fatalf("delete issue %d: %v", n, err)
		}
	}
	it, err := s.CreateIssue("alice", "ci", "alice", "after", "b")
	if err != nil {
		t.Fatalf("create issue after delete: %v", err)
	}
	if it.Number != lastIssue+1 {
		t.Fatalf("issue number reset on PG: got %d, want %d", it.Number, lastIssue+1)
	}
	pr, err := s.CreatePull("alice", "ci", "alice", "t", "b", "feat", "main", "", "", false)
	if err != nil {
		t.Fatalf("create pull: %v", err)
	}
	if pr.Number != 1 {
		t.Fatalf("pr number = %d, want 1", pr.Number)
	}

	// 会话
	if err := s.CreateSession("t1", u.ID); err != nil {
		t.Fatalf("create session: %v", err)
	}
	if name, err := s.GetSession("t1"); err != nil || name != "alice" {
		t.Fatalf("get session = %q, %v", name, err)
	}

	// 邮箱部分唯一索引：空邮箱可重复，非空唯一
	for _, n := range []string{"bob", "carol"} {
		if _, err := s.CreateUser(n, "h"); err != nil {
			t.Fatalf("create %s: %v", n, err)
		}
	}
	if err := s.SetUserEmail("bob", "bob@example.com"); err != nil {
		t.Fatalf("set bob email: %v", err)
	}
	if err := s.SetUserEmail("carol", "bob@example.com"); err == nil {
		t.Fatal("duplicate non-empty email should fail")
	}

	// settings upsert（ON CONFLICT DO UPDATE）
	if err := s.SetSetting("k", "v1"); err != nil {
		t.Fatalf("set setting: %v", err)
	}
	if err := s.SetSetting("k", "v2"); err != nil {
		t.Fatalf("update setting: %v", err)
	}
	if got := s.GetSetting("k"); got != "v2" {
		t.Fatalf("setting = %q, want v2", got)
	}

	// 组织 + 成员 upsert
	if _, err := s.CreateOrg("team", "Team", "alice"); err != nil {
		t.Fatalf("create org: %v", err)
	}
	if err := s.AddOrgMember("team", "bob", "member"); err != nil {
		t.Fatalf("add member: %v", err)
	}
	if err := s.AddOrgMember("team", "bob", "owner"); err != nil {
		t.Fatalf("upsert member: %v", err)
	}
	if role := s.OrgRole("team", "bob"); role != "owner" {
		t.Fatalf("member role = %q, want owner", role)
	}

	// LIKE 大小写一致性：大写查询要能命中（显式 LOWER）
	users, total, err := s.AdminListUsers("ALICE", 10, 0)
	if err != nil || total != 1 || len(users) != 1 || users[0].Username != "alice" {
		t.Fatalf("case-insensitive admin search: users=%v total=%d err=%v", users, total, err)
	}

	// 配额设置 JSON 往返
	if err := s.SetQuotaDefault(store.Quota{MaxReposPerUser: 2}); err != nil {
		t.Fatalf("set quota: %v", err)
	}
	if q := s.QuotaDefault(); q.MaxReposPerUser != 2 {
		t.Fatalf("quota round-trip = %+v", q)
	}

	t.Log("PG SMOKE OK")
}
