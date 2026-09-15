package store

// deleteuser_test.go 用户账号彻底删除的存储层测试：覆盖各类归属数据的清理与匿名化。

import (
	"errors"
	"strings"
	"testing"
)

func TestDeleteUserAccountPurgesEverything(t *testing.T) {
	s := openBanStore(t)
	alice, err := s.CreateUser("alice", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateUser("bobby", "hash"); err != nil {
		t.Fatal(err)
	}

	// alice 名下仓库 + 各类关联行
	aliceRepo := repoRow{Owner: "alice", Name: "r1", DefaultBranch: "main", HasIssues: true, CreatedAt: now()}
	bobRepo := repoRow{Owner: "bobby", Name: "r2", DefaultBranch: "main", HasIssues: true, CreatedAt: now()}
	mustCreate(t, s, &aliceRepo, &bobRepo)

	aliceIssue := issueRow{Owner: "alice", Repo: "r1", Number: 1, Title: "x", Author: "alice", CreatedAt: now(), UpdatedAt: now()}
	bobIssue := issueRow{Owner: "bobby", Repo: "r2", Number: 1, Title: "guest", Author: "alice", CreatedAt: now(), UpdatedAt: now()}
	label := repoLabelRow{Owner: "alice", Repo: "r1", Name: "bug", Color: "fff", CreatedAt: now()}
	mustCreate(t, s, &aliceIssue, &bobIssue, &label)
	il := issueLabelRow{IssueID: aliceIssue.ID, LabelID: label.ID}
	mustCreate(t, s, &il)
	milestone := milestoneRow{Owner: "alice", Repo: "r1", Title: "m", CreatedAt: now()}
	mustCreate(t, s, &milestone)

	proj := projectRow{Owner: "alice", Repo: "r1", Name: "p", CreatedAt: now()}
	mustCreate(t, s, &proj)
	col := projectColumnRow{ProjectID: proj.ID, Name: "c"}
	swim := projectSwimlaneRow{ProjectID: proj.ID, Name: "s"}
	mustCreate(t, s, &col, &swim)
	card := projectCardRow{ProjectID: proj.ID, ColumnID: col.ID, CreatedAt: now()}
	mustCreate(t, s, &card)

	mustCreate(t, s,
		&collabRow{Owner: "alice", Repo: "r1", Username: "bobby", Permission: "read", CreatedAt: now()},
		&collabRow{Owner: "bobby", Repo: "r2", Username: "alice", Permission: "read", CreatedAt: now()},
		&repoTopicRow{Owner: "alice", Repo: "r1", Topic: "topic"},
		&starRow{Username: "bobby", Owner: "alice", Repo: "r1", CreatedAt: now()},
		&starRow{Username: "alice", Owner: "bobby", Repo: "r2", CreatedAt: now()},
		&watchRow{Username: "alice", Owner: "bobby", Repo: "r2", CreatedAt: now()},
		&followRow{Follower: "alice", Followee: "bobby", CreatedAt: now()},
		&importRow{Owner: "alice", Repo: "r1", SourceURL: "x", CreatedAt: now()},
		&mirrorRow{Owner: "alice", Repo: "r1", URL: "x", CreatedAt: now()},
		&branchProtectionRow{Owner: "alice", Repo: "r1", Branch: "main", CreatedAt: now()},
		&pipelineCfgRow{Owner: "alice", Repo: "r1", CreatedAt: now()},
		&pipelineRunRow{Owner: "alice", Repo: "r1", TriggerBy: "alice", CreatedAt: now()},
		&pipelineScheduleRow{Owner: "alice", Repo: "r1", Expr: "* * * * *"},
		&repoEnvVarRow{Owner: "alice", Repo: "r1", Key: "K", CreatedAt: now()},
		&releaseRow{Owner: "alice", Repo: "r1", TagName: "v1", Author: "alice", CreatedAt: now()},
		&releaseAssetRow{Owner: "alice", Repo: "r1", Filename: "a", Content: []byte("x"), CreatedAt: now()},
		&incomingWebhookRow{Owner: "alice", Repo: "r1", TokenHash: "h", CreatedAt: now()},
		&copilotSessionRow{Owner: "alice", Repo: "r1", CreatedBy: "alice", CreatedAt: now(), UpdatedAt: now()},
	)

	pr := pullRequestRow{Owner: "alice", Repo: "r1", Number: 1, Title: "pr", Author: "alice", MergedBy: "alice", CreatedAt: now(), UpdatedAt: now()}
	review := pullReviewRow{Owner: "alice", Repo: "r1", Number: 1, Reviewer: "alice", State: "approved", CreatedAt: now()}
	comment := commentRow{Owner: "alice", Repo: "r1", Kind: "issue", Number: 1, Author: "alice", Body: "c", CreatedAt: now(), UpdatedAt: now()}
	mustCreate(t, s, &pr, &review, &comment)

	hook := webhookRow{Owner: "alice", Repo: "r1", URL: "http://x", CreatedAt: now()}
	mustCreate(t, s, &hook)
	mustCreate(t, s, &webhookDeliveryRow{HookID: hook.ID, Event: "push", Payload: "{}", Status: "success", CreatedAt: now()})

	// 通知：收件人/动作两类匿名化路径
	mustCreate(t, s,
		&notificationRow{Username: "bobby", Kind: "issue", Action: "opened", Owner: "alice", Repo: "r1", CreatedAt: now()},
		&notificationRow{Username: "alice", Kind: "issue", Action: "opened", Owner: "bobby", Repo: "r2", CreatedAt: now()},
		&notificationRow{Username: "bobby", Kind: "issue", Action: "opened", Owner: "bobby", Repo: "r2", Actor: "alice", CreatedAt: now()},
	)

	// 用户凭据 / 配置
	mustCreate(t, s,
		&sessionRow{Token: "t", UserID: alice.ID, CreatedAt: now(), ExpiresAt: now()},
		&sshKeyRow{UserID: alice.ID, Name: "k", PublicKey: "p", Fingerprint: "fp", CreatedAt: now()},
		&gpgKeyRow{UserID: alice.ID, Fingerprint: "gfp", Armor: "a", CreatedAt: now()},
		&patRow{UserID: alice.ID, Name: "pat", TokenHash: "th", CreatedAt: now()},
		&userOAuthRow{Provider: "github", ExternalID: "1", UserID: alice.ID, CreatedAt: now()},
		&byokKeyRow{Username: "alice", Name: "b", Provider: "compatible", APIKey: "k", CreatedAt: now(), UpdatedAt: now()},
		&userAvatarRow{Username: "alice", Data: []byte("x"), UpdatedAt: now()},
		&orgMemberRow{Org: "team", Username: "alice", Role: "member", CreatedAt: now()},
		&loginFailRow{Key: "alice|127.0.0.1", Count: 1, Until: now()},
	)
	app := oauthAppRow{UserID: alice.ID, Name: "app", ClientID: "cid", ClientSecretHash: "csh", CreatedAt: now()}
	mustCreate(t, s, &app)
	mustCreate(t, s,
		&oauthGrantRow{CodeHash: "ch", AppID: app.ID, UserID: alice.ID, ExpiresAt: now(), CreatedAt: now()},
		&oauthDeviceGrantRow{DeviceCodeHash: "dch", UserCode: "uc", ClientID: "cid", UserID: alice.ID, ExpiresAt: now(), CreatedAt: now()},
		&runnerRow{Name: "rn", SecretHash: "rh", Scope: "user:alice", CreatedAt: now()},
		&runnerTokenRow{TokenHash: "rth", Scope: "user:alice", ExpiresAt: now(), CreatedAt: now()},
		&settingRow{Key: quotaUserPrefix + "alice", Value: "{}"},
		&settingRow{Key: "email_mfa:disable:alice", Value: "{}"},
	)

	// 包 / 注册表
	mustCreate(t, s,
		&packageRow{Owner: "alice", Type: "npm", Name: "pkg", Version: "1", Filename: "f", CreatedAt: now()},
		&packageTagRow{Owner: "alice", Type: "npm", Name: "pkg", Tag: "latest", Version: "1"},
		&packageAuditRow{Owner: "alice", Type: "npm", Name: "pkg", Action: "publish", Actor: "alice", CreatedAt: now()},
		&registryManifestRow{Owner: "alice", Image: "img", Reference: "latest", Digest: "sha256:" + strings.Repeat("a", 64), Content: []byte("{}"), CreatedAt: now()},
	)
	// 他人仓库中 alice 的内容（应保留但匿名化）
	mustCreate(t, s,
		&commentRow{Owner: "bobby", Repo: "r2", Kind: "issue", Number: 1, Author: "alice", Body: "guest", CreatedAt: now(), UpdatedAt: now()},
		&releaseRow{Owner: "bobby", Repo: "r2", TagName: "v1", Author: "alice", CreatedAt: now()},
		&pipelineRunRow{Owner: "bobby", Repo: "r2", TriggerBy: "alice", CreatedAt: now()},
		&packageRow{Owner: "bobby", Type: "npm", Name: "other", Version: "1", Filename: "f", Uploader: "alice", CreatedAt: now()},
		&packageAuditRow{Owner: "bobby", Type: "npm", Name: "other", Action: "publish", Actor: "alice", CreatedAt: now()},
	)

	if err := s.DeleteUserAccount("alice"); err != nil {
		t.Fatalf("DeleteUserAccount: %v", err)
	}

	// 用户与自有数据全部消失
	if _, err := s.GetByUsername("alice"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("user still present: %v", err)
	}
	// 按 owner 归属的仓库关联数据全部清空（不影响 bobby 的仓库）
	ownerScoped := []any{
		&repoRow{}, &issueRow{}, &commentRow{}, &repoLabelRow{}, &milestoneRow{}, &repoTopicRow{},
		&webhookRow{}, &pipelineCfgRow{}, &pipelineRunRow{}, &pipelineScheduleRow{}, &repoEnvVarRow{},
		&releaseRow{}, &releaseAssetRow{}, &incomingWebhookRow{}, &copilotSessionRow{},
		&packageRow{}, &packageTagRow{}, &packageAuditRow{}, &registryManifestRow{},
	}
	for _, m := range ownerScoped {
		if n := countRows(t, s, m, "owner = ?", "alice"); n != 0 {
			t.Fatalf("%T still has %d alice rows", m, n)
		}
	}
	// 按父表 ID 关联的子表 / 项目结构全部清空
	for _, m := range []any{
		&issueLabelRow{}, &projectRow{}, &projectColumnRow{}, &projectSwimlaneRow{},
		&projectCardRow{}, &webhookDeliveryRow{},
	} {
		if n := countRows(t, s, m, ""); n != 0 {
			t.Fatalf("%T still has %d rows", m, n)
		}
	}
	// 用户凭据 / 配置全部清空
	for _, m := range []any{
		&sessionRow{}, &sshKeyRow{}, &gpgKeyRow{}, &patRow{}, &userOAuthRow{},
		&oauthGrantRow{}, &oauthDeviceGrantRow{}, &runnerRow{}, &runnerTokenRow{},
		&byokKeyRow{}, &userAvatarRow{},
	} {
		if n := countRows(t, s, m, ""); n != 0 {
			t.Fatalf("%T still has %d rows", m, n)
		}
	}
	// 通知：收件人 / 仓库归属的通知全部删除
	if n := countRows(t, s, &notificationRow{}, "username = ? OR owner = ?", "alice", "alice"); n != 0 {
		t.Fatalf("notifications still has %d alice rows", n)
	}
	// 用户注册的 OAuth 应用（内置第一方应用 user_id=0 不受影响）
	if n := countRows(t, s, &oauthAppRow{}, "user_id = ?", alice.ID); n != 0 {
		t.Fatalf("oauth_apps still has %d rows for alice", n)
	}
	// 配额覆置与一次性验证码（内置设置不受影响）
	if n := countRows(t, s, &settingRow{}, "\"key\" IN ?",
		[]string{quotaUserPrefix + "alice", "email_mfa:disable:alice"}); n != 0 {
		t.Fatalf("settings still has %d alice rows", n)
	}
	// 跨用户关联的 alice 身份数据（协作者/star/watch/关注/组织成员/限速）
	for _, q := range []struct {
		model any
		where string
	}{
		{&collabRow{}, "username = ?"},
		{&starRow{}, "username = ?"},
		{&watchRow{}, "username = ?"},
		{&followRow{}, "follower = ?"},
		{&orgMemberRow{}, "username = ?"},
		{&loginFailRow{}, "\"key\" LIKE ?"},
	} {
		arg := any("alice")
		if q.where == "\"key\" LIKE ?" {
			arg = "alice|%"
		}
		if n := countRows(t, s, q.model, q.where, arg); n != 0 {
			t.Fatalf("%T (username=alice) still has %d rows", q.model, n)
		}
	}

	// 他人数据保留
	if n := countRows(t, s, &repoRow{}, "owner = ?", "bobby"); n != 1 {
		t.Fatalf("bobby repo rows = %d, want 1", n)
	}
	if n := countRows(t, s, &issueRow{}, "owner = ?", "bobby"); n != 1 {
		t.Fatalf("bobby issue rows = %d, want 1", n)
	}
	// 内容匿名化
	var guestIssue issueRow
	if err := s.db.Where("owner = ? AND repo = ? AND number = ?", "bobby", "r2", 1).First(&guestIssue).Error; err != nil {
		t.Fatal(err)
	}
	if guestIssue.Author != "deleted-user" {
		t.Fatalf("guest issue author = %q, want deleted-user", guestIssue.Author)
	}
	var guestComment commentRow
	if err := s.db.Where("owner = ? AND repo = ?", "bobby", "r2").First(&guestComment).Error; err != nil {
		t.Fatal(err)
	}
	if guestComment.Author != "deleted-user" {
		t.Fatalf("guest comment author = %q, want deleted-user", guestComment.Author)
	}
	var guestRelease releaseRow
	if err := s.db.Where("owner = ? AND repo = ?", "bobby", "r2").First(&guestRelease).Error; err != nil {
		t.Fatal(err)
	}
	if guestRelease.Author != "deleted-user" {
		t.Fatalf("guest release author = %q, want deleted-user", guestRelease.Author)
	}
	var guestRun pipelineRunRow
	if err := s.db.Where("owner = ? AND repo = ?", "bobby", "r2").First(&guestRun).Error; err != nil {
		t.Fatal(err)
	}
	if guestRun.TriggerBy != "deleted-user" {
		t.Fatalf("guest pipeline trigger_by = %q, want deleted-user", guestRun.TriggerBy)
	}
	var guestPkg packageRow
	if err := s.db.Where("owner = ? AND name = ?", "bobby", "other").First(&guestPkg).Error; err != nil {
		t.Fatal(err)
	}
	if guestPkg.Uploader != "deleted-user" {
		t.Fatalf("guest package uploader = %q, want deleted-user", guestPkg.Uploader)
	}
	var guestAudit packageAuditRow
	if err := s.db.Where("owner = ? AND name = ?", "bobby", "other").First(&guestAudit).Error; err != nil {
		t.Fatal(err)
	}
	if guestAudit.Actor != "deleted-user" {
		t.Fatalf("guest package audit actor = %q, want deleted-user", guestAudit.Actor)
	}
	var guestNote notificationRow
	if err := s.db.Where("owner = ? AND repo = ?", "bobby", "r2").First(&guestNote).Error; err != nil {
		t.Fatal(err)
	}
	if guestNote.Actor != "deleted-user" {
		t.Fatalf("guest notification actor = %q, want deleted-user", guestNote.Actor)
	}

	// 删除不存在的用户返回 ErrNotFound
	if err := s.DeleteUserAccount("alice"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete missing = %v, want ErrNotFound", err)
	}
}

func mustCreate(t *testing.T, s *Store, rows ...any) {
	t.Helper()
	for _, r := range rows {
		if err := s.db.Create(r).Error; err != nil {
			t.Fatalf("create %T: %v", r, err)
		}
	}
}

func countRows(t *testing.T, s *Store, model any, where string, args ...any) int64 {
	t.Helper()
	var n int64
	q := s.db.Model(model)
	if where != "" {
		q = q.Where(where, args...)
	}
	if err := q.Count(&n).Error; err != nil {
		t.Fatalf("count %T: %v", model, err)
	}
	return n
}
