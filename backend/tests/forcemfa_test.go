package tests

import "testing"

// TestForceMFA 验证实例强制 MFA：未启用 MFA 的交互式会话被拦截，
// 但 /me 与 MFA 注册端点放行；PAT 不受影响。
func TestForceMFA(t *testing.T) {
	env := start(t)
	alice := register(t, env, "fmauser", "alice-pass-123")

	// 先创建一个 PAT（强制 MFA 前），之后验证它不受影响
	out := alice.mustStatus("POST", "/tokens", map[string]any{"name": "ci", "scopes": []string{"repo"}}, 201)
	pat, _ := out["token"].(string)
	if pat == "" {
		t.Fatalf("pat = %v", out)
	}

	// 默认不强制
	if m := alice.mustStatus("GET", "/me", nil, 200); m["mfa_required"] != false {
		t.Fatalf("me before force = %v", m)
	}

	if err := env.Store.SetSetting("force_mfa", "1"); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = env.Store.SetSetting("force_mfa", "0") }()

	me := alice.mustStatus("GET", "/me", nil, 200)
	if me["mfa_required"] != true || me["mfa_enabled"] != false {
		t.Fatalf("me after force = %v", me)
	}
	// 非豁免端点被拦截
	alice.mustFail("GET", "/repos", nil, 403)
	// MFA 注册端放行
	alice.mustStatus("GET", "/me/mfa", nil, 200)

	// PAT 不受强制 MFA 影响（自动化场景）
	patClient := &Client{env: env, token: pat}
	patClient.mustStatus("GET", "/repos", nil, 200)
}
