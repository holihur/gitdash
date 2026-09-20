package notify

import (
	"path/filepath"
	"testing"

	"gitdash/backend/internal/store"
)

// TestActiveConfigPrecedence 校验管理端 SMTP 配置优先于环境变量，且关闭后回退环境变量。
func TestActiveConfigPrecedence(t *testing.T) {
	t.Setenv("GITDASH_SMTP_HOST", "env.example.com")
	t.Setenv("GITDASH_SMTP_PORT", "")

	s, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}

	// 未启用管理端配置：回退环境变量。
	cfg, ok := ActiveConfig(s)
	if !ok || cfg.Host != "env.example.com" || cfg.Port != "587" {
		t.Fatalf("env fallback = %+v, ok=%v", cfg, ok)
	}

	// 启用管理端配置：覆盖环境变量。
	if err := s.SetSetting(SettingEnabled, "1"); err != nil {
		t.Fatal(err)
	}
	_ = s.SetSetting(SettingHost, "store.example.com")
	_ = s.SetSetting(SettingPort, "2525")
	_ = s.SetSetting(SettingFrom, "noreply@example.com")
	cfg, ok = ActiveConfig(s)
	if !ok || cfg.Host != "store.example.com" || cfg.Port != "2525" || cfg.From != "noreply@example.com" {
		t.Fatalf("stored config = %+v, ok=%v", cfg, ok)
	}

	// 关闭后再次回退环境变量。
	if err := s.SetSetting(SettingEnabled, "0"); err != nil {
		t.Fatal(err)
	}
	cfg, ok = ActiveConfig(s)
	if !ok || cfg.Host != "env.example.com" {
		t.Fatalf("fallback after disable = %+v, ok=%v", cfg, ok)
	}
}

// TestStoreSenderUnconfigured 未配置时 Configured=false 且 Send 返回错误。
func TestStoreSenderUnconfigured(t *testing.T) {
	t.Setenv("GITDASH_SMTP_HOST", "")
	s, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	sender := NewStoreSender(s)
	if sender.Configured() {
		t.Fatal("Configured should be false without SMTP settings")
	}
	if err := sender.Send("a@b.c", "subject", "body"); err == nil {
		t.Fatal("Send should fail when SMTP is not configured")
	}
}
