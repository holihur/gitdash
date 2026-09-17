package store

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepoSecretEncryptedAtRest(t *testing.T) {
	t.Setenv("GITDASH_SECRET_KEY", strings.Repeat("0", 64)) // 32 字节全零密钥
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
