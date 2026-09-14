package tests

import (
	"testing"

	"gitdash/backend/internal/store"
)

const quotaKey1 = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIHqI1nEJ+VksNOkTg3rWM+gv+7m07ra3reXEa2hAVSar q1@example.com"
const quotaKey2 = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIKMBkoX7Ild9UiPUeUKdp0V8XEYIn3q4LwvCg9r878Kj q2@example.com"

func expectQuota(t *testing.T, c *Client, method, path string, body any) {
	t.Helper()
	code, m := c.do(method, path, body)
	if code != 403 || m["code"] != "quota_exceeded" {
		t.Fatalf("%s %s: got %d %v, want 403 quota_exceeded", method, path, code, m)
	}
}

func TestQuotaReposPerUserAndOverride(t *testing.T) {
	env := start(t)
	if err := env.Store.SetQuotaDefault(store.Quota{MaxReposPerUser: 1}); err != nil {
		t.Fatalf("set default quota: %v", err)
	}
	alice := register(t, env, "alice", "alice-pass-123")

	alice.mustStatus("POST", "/repos", map[string]any{"name": "one"}, 201)
	expectQuota(t, alice, "POST", "/repos", map[string]any{"name": "two"})

	// per-user 覆盖放宽为不限后可继续创建
	if err := env.Store.SetUserQuota("alice", store.Quota{}); err != nil {
		t.Fatalf("set user quota: %v", err)
	}
	alice.mustStatus("POST", "/repos", map[string]any{"name": "two"}, 201)
}

func TestQuotaOrgReposAndMembers(t *testing.T) {
	env := start(t)
	if err := env.Store.SetQuotaDefault(store.Quota{MaxReposPerOrg: 1, MaxOrgMembers: 1}); err != nil {
		t.Fatalf("set default quota: %v", err)
	}
	alice := register(t, env, "alice", "alice-pass-123")
	register(t, env, "bobby", "bobby-pass-1234")

	alice.mustStatus("POST", "/orgs", map[string]any{"name": "team", "display": "Team"}, 201)

	// 组织成员上限：创建者已是 1 人，再加人超限
	expectQuota(t, alice, "POST", "/orgs/team/members", map[string]any{"username": "bobby", "role": "member"})

	// 组织仓库上限（不计入个人配额）
	alice.mustStatus("POST", "/repos", map[string]any{"name": "r1", "namespace": "team"}, 201)
	expectQuota(t, alice, "POST", "/repos", map[string]any{"name": "r2", "namespace": "team"})

	// 组织覆盖放宽成员上限
	if err := env.Store.SetOrgQuota("team", store.Quota{MaxOrgMembers: 5}); err != nil {
		t.Fatalf("set org quota: %v", err)
	}
	alice.mustStatus("POST", "/orgs/team/members", map[string]any{"username": "bobby", "role": "member"}, 200)
}

func TestQuotaUserKeysPATsWebhooks(t *testing.T) {
	env := start(t)
	if err := env.Store.SetQuotaDefault(store.Quota{
		MaxSSHKeysPerUser:  1,
		MaxPATsPerUser:     1,
		MaxWebhooksPerRepo: 1,
	}); err != nil {
		t.Fatalf("set default quota: %v", err)
	}
	alice := register(t, env, "alice", "alice-pass-123")

	// SSH keys 上限
	alice.mustStatus("POST", "/keys", map[string]any{"name": "k1", "public_key": quotaKey1}, 201)
	expectQuota(t, alice, "POST", "/keys", map[string]any{"name": "k2", "public_key": quotaKey2})

	// PAT 上限
	alice.mustStatus("POST", "/tokens", map[string]any{"name": "t1"}, 201)
	expectQuota(t, alice, "POST", "/tokens", map[string]any{"name": "t2"})

	// webhook 上限（每仓库）
	alice.mustStatus("POST", "/repos", map[string]any{"name": "hooks"}, 201)
	alice.mustStatus("POST", "/users/alice/repos/hooks/webhooks", map[string]any{"url": "https://example.com/1"}, 201)
	expectQuota(t, alice, "POST", "/users/alice/repos/hooks/webhooks", map[string]any{"url": "https://example.com/2"})
}
