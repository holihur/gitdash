package tests

import "testing"

// TestEmailVerificationFlow SMTP 未配置（测试默认）时：
// 设置邮箱 → 直接视为已验证；verify 端点对非法令牌 400；resend 无 SMTP 返回 400。
func TestEmailVerificationFlow(t *testing.T) {
	env := start(t)
	alice := register(t, env, "alice", "alice-pass-123")

	m := alice.mustStatus("POST", "/me/profile",
		map[string]string{"email": "alice@example.com"}, 200)
	if m["email_verified"] != true {
		t.Fatalf("SMTP 未配置时应直接已验证: %v", m)
	}

	// me 返回 email_verified
	me := alice.mustStatus("GET", "/me", nil, 200)
	if me["email_verified"] != true || me["email"] != "alice@example.com" {
		t.Fatalf("me = %v", me)
	}

	// 非法令牌
	alice.mustFail("POST", "/me/email/verify",
		map[string]string{"token": "bogus-token"}, 400)
	// 缺 token
	alice.mustFail("POST", "/me/email/verify", map[string]string{}, 400)

	// 已验证邮箱 resend → 400（nothing_to_verify）
	alice.mustFail("POST", "/me/email/resend", nil, 400)

	// 清空邮箱
	alice.mustStatus("POST", "/me/profile", map[string]string{"email": ""}, 200)
}
