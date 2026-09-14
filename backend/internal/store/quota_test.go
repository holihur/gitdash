package store

import (
	"errors"
	"path/filepath"
	"testing"
)

func openQuotaStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "quota.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	return s
}

func TestQuotaDefaultsAndOverrides(t *testing.T) {
	s := openQuotaStore(t)
	if got := s.QuotaDefault(); got != (Quota{}) {
		t.Fatalf("default quota = %+v, want all zero (unlimited)", got)
	}

	def := Quota{MaxReposPerUser: 2, MaxOrgMembers: 3}
	if err := s.SetQuotaDefault(def); err != nil {
		t.Fatalf("set default: %v", err)
	}
	if got := s.QuotaDefault(); got != def {
		t.Fatalf("default quota = %+v, want %+v", got, def)
	}

	// 含负数的配置被规范化成 0（不限）
	if err := s.SetQuotaDefault(Quota{MaxReposPerUser: -5}); err != nil {
		t.Fatalf("set negative: %v", err)
	}
	if got := s.QuotaDefault().MaxReposPerUser; got != 0 {
		t.Fatalf("negative normalized = %d, want 0", got)
	}

	// 未覆盖时继承默认
	if got := s.QuotaForUser("alice"); got != (Quota{}) {
		t.Fatalf("user inherits default = %+v", got)
	}

	// per-user 覆盖为整体替换（未设置字段归零）
	if err := s.SetQuotaDefault(Quota{MaxReposPerUser: 2, MaxOrgMembers: 3}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetUserQuota("Alice", Quota{MaxReposPerUser: 7}); err != nil {
		t.Fatalf("set user quota: %v", err)
	}
	gotUser := s.QuotaForUser("alice") // 大小写不敏感
	if gotUser.MaxReposPerUser != 7 || gotUser.MaxOrgMembers != 0 {
		t.Fatalf("user override = %+v, want {MaxReposPerUser:7}", gotUser)
	}

	if err := s.SetOrgQuota("Team", Quota{MaxReposPerOrg: 4}); err != nil {
		t.Fatalf("set org quota: %v", err)
	}
	if got := s.QuotaForOrg("team"); got.MaxReposPerOrg != 4 {
		t.Fatalf("org override = %+v", got)
	}
	if got := s.QuotaForOrg("other"); got != (Quota{MaxReposPerUser: 2, MaxOrgMembers: 3}) {
		t.Fatalf("unset org should inherit default, got %+v", got)
	}

	overrides, err := s.ListQuotaOverrides()
	if err != nil {
		t.Fatalf("list overrides: %v", err)
	}
	if len(overrides) != 2 {
		t.Fatalf("overrides = %+v, want 2", overrides)
	}

	if err := s.DeleteUserQuota("ALICE"); err != nil {
		t.Fatalf("delete user quota: %v", err)
	}
	if got := s.QuotaForUser("alice"); got != (Quota{MaxReposPerUser: 2, MaxOrgMembers: 3}) {
		t.Fatalf("after delete user inherits default, got %+v", got)
	}
	if err := s.DeleteOrgQuota("team"); err != nil {
		t.Fatalf("delete org quota: %v", err)
	}
	if overrides, _ := s.ListQuotaOverrides(); len(overrides) != 0 {
		t.Fatalf("overrides after delete = %+v, want empty", overrides)
	}
}

func TestQuotaEnforcement(t *testing.T) {
	s := openQuotaStore(t)
	for _, u := range []string{"alice", "bobby"} {
		if _, err := s.CreateUser(u, "password-123"); err != nil {
			t.Fatalf("create %s: %v", u, err)
		}
	}

	// 个人仓库：默认上限 1，超限返回 *QuotaError
	if err := s.SetQuotaDefault(Quota{MaxReposPerUser: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateRepo("alice", "one", "", true); err != nil {
		t.Fatalf("first repo: %v", err)
	}
	_, err := s.CreateRepo("alice", "two", "", true)
	if !errors.Is(err, ErrQuotaExceeded) {
		t.Fatalf("second repo err = %v, want ErrQuotaExceeded", err)
	}
	var qe *QuotaError
	if !errors.As(err, &qe) || qe.Scope != "user" || qe.Item != "repos" || qe.Limit != 1 {
		t.Fatalf("quota error = %+v", qe)
	}
	// per-user 放宽后可继续
	if err := s.SetUserQuota("alice", Quota{}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateRepo("alice", "two", "", true); err != nil {
		t.Fatalf("repo after override: %v", err)
	}
	// 移除覆盖，后续用例回到默认配额
	if err := s.DeleteUserQuota("alice"); err != nil {
		t.Fatalf("delete override: %v", err)
	}

	// 组织：建组织上限，成员上限，组织仓库上限
	if err := s.SetQuotaDefault(Quota{MaxOrgsPerUser: 1, MaxOrgMembers: 1, MaxReposPerOrg: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateOrg("team", "Team", "alice"); err != nil {
		t.Fatalf("create org: %v", err)
	}
	if _, err := s.CreateOrg("team2", "Team2", "alice"); !errors.Is(err, ErrQuotaExceeded) {
		t.Fatalf("second org err = %v, want quota", err)
	}
	// team 已有 alice（1 人），加 bobby 超成员上限
	if err := s.AddOrgMember("team", "bobby", "member"); !errors.Is(err, ErrQuotaExceeded) {
		t.Fatalf("add member err = %v, want quota", err)
	}
	// 已存在成员只改角色，不占新名额
	if err := s.AddOrgMember("team", "alice", "owner"); err != nil {
		t.Fatalf("update existing member: %v", err)
	}
	// 组织仓库上限
	if _, err := s.CreateRepo("team", "r1", "", true); err != nil {
		t.Fatalf("org repo 1: %v", err)
	}
	if _, err := s.CreateRepo("team", "r2", "", true); !errors.Is(err, ErrQuotaExceeded) {
		t.Fatalf("org repo 2 err = %v, want quota", err)
	}

	// SSH / GPG / PAT / webhook 上限
	if err := s.SetQuotaDefault(Quota{MaxSSHKeysPerUser: 1, MaxGPGKeysPerUser: 1, MaxPATsPerUser: 1, MaxWebhooksPerRepo: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateKey("alice", "k1", "ssh-ed25519 AAAA k1", "SHA256:fp1"); err != nil {
		t.Fatalf("key1: %v", err)
	}
	if _, err := s.CreateKey("alice", "k2", "ssh-ed25519 BBBB k2", "SHA256:fp2"); !errors.Is(err, ErrQuotaExceeded) {
		t.Fatalf("key2 err = %v, want quota", err)
	}
	if _, err := s.AddGPGKey("alice", "FP1", "armor1"); err != nil {
		t.Fatalf("gpg1: %v", err)
	}
	if _, err := s.AddGPGKey("alice", "FP2", "armor2"); !errors.Is(err, ErrQuotaExceeded) {
		t.Fatalf("gpg2 err = %v, want quota", err)
	}
	uid, err := s.UserID("alice")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.CreatePAT(uid, "t1", "repo", "", ""); err != nil {
		t.Fatalf("pat1: %v", err)
	}
	if _, _, err := s.CreatePAT(uid, "t2", "repo", "", ""); !errors.Is(err, ErrQuotaExceeded) {
		t.Fatalf("pat2 err = %v, want quota", err)
	}
	if _, err := s.CreateRepo("alice", "hooks", "", true); err != nil {
		// alice 个人仓库默认上限此刻为 1，且已放宽过 → 覆盖仍是不限
		t.Fatalf("repo for webhook: %v", err)
	}
	if _, err := s.CreateWebhook("alice", "hooks", "https://example.com/1", ""); err != nil {
		t.Fatalf("webhook1: %v", err)
	}
	if _, err := s.CreateWebhook("alice", "hooks", "https://example.com/2", ""); !errors.Is(err, ErrQuotaExceeded) {
		t.Fatalf("webhook2 err = %v, want quota", err)
	}
}
