package store

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// resetSecretKey 清空密钥缓存，便于测试通过 GITDASH_SECRET_KEY 覆盖（生产环境
// 密钥在进程生命周期内不变，缓存只解析一次）。
func resetSecretKey() {
	secretKeyOnce = sync.Once{}
	secretAEADVal = nil
}

func TestRepoSecretEncryptedAtRest(t *testing.T) {
	t.Setenv("GITDASH_SECRET_KEY", strings.Repeat("0", 64)) // 32 字节全零密钥
	resetSecretKey()
	defer resetSecretKey()
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}

	if err := s.SetRepoSecret("alice", "r", "TOKEN", "s3cr3t-value"); err != nil {
		t.Fatalf("set: %v", err)
	}
	// 明文不得落库
	var row repoSecretRow
	if err := s.db.Where("owner = ? AND repo = ? AND name = ?", "alice", "r", "TOKEN").First(&row).Error; err != nil {
		t.Fatal(err)
	}
	if row.Value == "s3cr3t-value" || !strings.HasPrefix(row.Value, "v1:") {
		t.Fatalf("secret not encrypted at rest: %q", row.Value)
	}

	// 按白名单解析回明文；未命中的名字忽略
	vals, err := s.RepoSecretValues("alice", "r", []string{"TOKEN", "MISSING"})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if vals["TOKEN"] != "s3cr3t-value" {
		t.Fatalf("value = %q", vals["TOKEN"])
	}
	if _, ok := vals["MISSING"]; ok {
		t.Fatal("missing secret should be absent")
	}

	// upsert 覆盖
	if err := s.SetRepoSecret("alice", "r", "TOKEN", "new-value"); err != nil {
		t.Fatal(err)
	}
	vals, _ = s.RepoSecretValues("alice", "r", []string{"TOKEN"})
	if vals["TOKEN"] != "new-value" {
		t.Fatalf("overwrite value = %q", vals["TOKEN"])
	}

	// 列表只含元信息
	list, err := s.ListRepoSecrets("alice", "r")
	if err != nil || len(list) != 1 || list[0].Name != "TOKEN" {
		t.Fatalf("list = %+v err=%v", list, err)
	}

	if err := s.DeleteRepoSecret("alice", "r", "TOKEN"); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteRepoSecret("alice", "r", "TOKEN"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete err = %v, want ErrNotFound", err)
	}
}

func TestRepoSecretValidation(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetRepoSecret("a", "b", "bad-name", "v"); err == nil {
		t.Fatal("invalid name should fail")
	}
	if err := s.SetRepoSecret("a", "b", "OK", strings.Repeat("x", MaxRepoSecretValueLen+1)); err == nil {
		t.Fatal("oversized value should fail")
	}
	for i := 0; i < MaxRepoSecrets; i++ {
		if err := s.SetRepoSecret("a", "b", fmt.Sprintf("S%d", i), "v"); err != nil {
			t.Fatalf("set %d: %v", i, err)
		}
	}
	if err := s.SetRepoSecret("a", "b", "OVER_LIMIT", "v"); err == nil {
		t.Fatal("over-limit should fail")
	}
}

// TestSensitiveFieldsEncryptedAtRest 覆盖安全评审 §3.1：BYOK API key、镜像 SSH
// 私钥、webhook 签名密钥、仓库环境变量都经统一信封加密落库，并可按需解密回明文。
func TestSensitiveFieldsEncryptedAtRest(t *testing.T) {
	t.Setenv("GITDASH_SECRET_KEY", strings.Repeat("ab", 32))
	resetSecretKey()
	defer resetSecretKey()
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}

	// BYOK
	if _, err := s.CreateByokKey("alice", "k", "anthropic", "sk-secret", "", "m"); err != nil {
		t.Fatal(err)
	}
	var bk byokKeyRow
	if err := s.db.Where("username = ?", "alice").First(&bk).Error; err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(bk.APIKey, "v1:") {
		t.Fatalf("byok api_key not encrypted: %q", bk.APIKey)
	}
	sec, err := s.GetByokSecret("alice", bk.ID)
	if err != nil || sec.APIKey != "sk-secret" {
		t.Fatalf("byok round-trip = %+v err=%v", sec, err)
	}

	// mirror private key
	if err := s.SetMirror("alice", "r", "git@example.com:x.git", "PRIVATE-KEY-DATA"); err != nil {
		t.Fatal(err)
	}
	var mr mirrorRow
	if err := s.db.Where("owner = ? AND repo = ?", "alice", "r").First(&mr).Error; err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(mr.PrivateKey, "v1:") {
		t.Fatalf("mirror private_key not encrypted: %q", mr.PrivateKey)
	}
	m, err := s.GetMirror("alice", "r")
	if err != nil || m.PrivateKey != "PRIVATE-KEY-DATA" {
		t.Fatalf("mirror round-trip = %+v err=%v", m, err)
	}

	// webhook secret
	if _, err := s.CreateWebhook("alice", "r", "https://example.com/h", "hook-secret"); err != nil {
		t.Fatal(err)
	}
	var wr webhookRow
	if err := s.db.Where("owner = ? AND repo = ?", "alice", "r").First(&wr).Error; err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(wr.Secret, "v1:") {
		t.Fatalf("webhook secret not encrypted: %q", wr.Secret)
	}
	ws, err := s.ListWebhooks("alice", "r")
	if err != nil || len(ws) != 1 || ws[0].Secret != "hook-secret" {
		t.Fatalf("webhook round-trip = %+v err=%v", ws, err)
	}

	// env var
	if err := s.SetRepoEnvVar("alice", "r", "TOKEN", "env-secret"); err != nil {
		t.Fatal(err)
	}
	var er repoEnvVarRow
	if err := s.db.Where("owner = ? AND repo = ? AND key = ?", "alice", "r", "TOKEN").First(&er).Error; err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(er.Value, "v1:") {
		t.Fatalf("env value not encrypted: %q", er.Value)
	}
	vars, err := s.ListRepoEnvVars("alice", "r")
	if err != nil || len(vars) != 1 || vars[0].Value != "env-secret" {
		t.Fatalf("env round-trip = %+v err=%v", vars, err)
	}
}
