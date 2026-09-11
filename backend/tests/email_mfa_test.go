package tests

import (
	"net/http/httptest"
	"path/filepath"
	"testing"

	"gitdash/backend/internal/api"
	"gitdash/backend/internal/gitsvc"
	"gitdash/backend/internal/notify"
	"gitdash/backend/internal/store"
)

// startSMTP 启动带 SMTP 发送器（投递会失败但仅记日志）的 HTTP 实例，
// 用于 email MFA 全流程测试；验证码可通过 env.Store.GetEmailMFACode 读取。
func startSMTP(t *testing.T) *Env {
	t.Helper()
	t.Setenv("GITDASH_SMTP_HOST", "127.0.0.1")
	t.Setenv("GITDASH_SMTP_PORT", "1")
	t.Setenv("GITDASH_DISABLE_RATE_LIMIT", "1")
	dir := t.TempDir()
	t.Setenv("GITDASH_DATA", dir)
	if err := gitsvc.Init(dir); err != nil {
		t.Fatalf("gitsvc init: %v", err)
	}
	st, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	ap := api.New(st, "test")
	ap.SetEmailSender(notify.NewSender())
	hs := httptest.NewServer(ap.Handler(""))
	t.Cleanup(hs.Close)
	return &Env{t: t, BaseURL: hs.URL, ReposDir: gitsvc.ReposDir(), DataDir: dir, Store: st}
}

// verifiedEmail 设置并"验证"用户邮箱（绕过邮件链接：直接标记已验证）。
func verifiedEmail(t *testing.T, env *Env, c *Client, username, email string) {
	t.Helper()
	c.mustStatus("POST", "/me/profile", map[string]string{"email": email}, 200)
	if err := env.Store.MarkEmailVerifiedByUsername(username); err != nil {
		t.Fatalf("mark verified: %v", err)
	}
}

func emailCode(t *testing.T, env *Env, key string) string {
	t.Helper()
	code, expires, err := env.Store.GetEmailMFACode(key)
	if err != nil || code == "" || expires == "" {
		t.Fatalf("read email mfa code %q: %v", key, err)
	}
	return code
}

func TestEmailMFAFlow(t *testing.T) {
	env := startSMTP(t)
	alice := register(t, env, "alice", "alice-pass-123")
	verifiedEmail(t, env, alice, "alice", "alice@example.com")

	// 未配置 SMTP 的实例不能绑定 email MFA
	plain := start(t)
	pa := register(t, plain, "bobby", "bob-pass-1234")
	pa.mustFail("POST", "/me/mfa/email/enroll", nil, 400)

	// enroll：发送激活码
	if m := alice.mustStatus("POST", "/me/mfa/email/enroll", nil, 200); m["sent"] != true {
		t.Fatalf("enroll = %v", m)
	}
	code := emailCode(t, env, "enroll:alice")
	alice.mustFail("POST", "/me/mfa/email/activate", map[string]string{"code": "000000"}, 400)
	alice.mustStatus("POST", "/me/mfa/email/activate", map[string]string{"code": code}, 204)
	alice.mustFail("POST", "/me/mfa/email/enroll", nil, 409)

	m := alice.mustStatus("GET", "/me/mfa", nil, 200)
	if m["enabled"] != true || m["method"] != "email" {
		t.Fatalf("mfa status = %v", m)
	}

	// 登录：进入 email 验证码二次验证
	login := alice.mustStatus("POST", "/auth/login",
		map[string]string{"username": "alice", "password": "alice-pass-123"}, 200)
	if login["mfa_required"] != true || login["mfa_method"] != "email" {
		t.Fatalf("login = %v", login)
	}
	tok, _ := login["mfa_token"].(string)

	// 错误验证码 401；重发后旧验证码失效
	alice.mustFail("POST", "/auth/mfa-verify",
		map[string]string{"mfa_token": tok, "code": "000000"}, 401)
	if m := alice.mustStatus("POST", "/auth/mfa-email/resend",
		map[string]string{"mfa_token": tok}, 200); m["sent"] != true {
		t.Fatalf("resend = %v", m)
	}
	oldCode := code
	code2 := emailCode(t, env, tok)
	alice.mustFail("POST", "/auth/mfa-verify",
		map[string]string{"mfa_token": tok, "code": oldCode}, 401)
	ok := alice.mustStatus("POST", "/auth/mfa-verify",
		map[string]string{"mfa_token": tok, "code": code2}, 200)
	if ok["token"] == nil {
		t.Fatalf("verify = %v", ok)
	}
	// mfa_token 一次性
	alice.mustFail("POST", "/auth/mfa-verify",
		map[string]string{"mfa_token": tok, "code": code2}, 401)

	// 关闭：需要密码 + 通过 /me/mfa/email/send 下发的验证码
	alice.mustStatus("POST", "/me/mfa/email/send", nil, 204)
	dcode := emailCode(t, env, "disable:alice")
	alice.mustFail("POST", "/me/mfa/disable",
		map[string]string{"password": "wrong", "code": dcode}, 401)
	alice.mustFail("POST", "/me/mfa/disable",
		map[string]string{"password": "alice-pass-123", "code": "000000"}, 400)
	alice.mustStatus("POST", "/me/mfa/disable",
		map[string]string{"password": "alice-pass-123", "code": dcode}, 204)
	if m := alice.mustStatus("GET", "/me/mfa", nil, 200); m["enabled"] != false {
		t.Fatalf("mfa after disable = %v", m)
	}
	// 关闭后登录直接成功
	l2 := alice.mustStatus("POST", "/auth/login",
		map[string]string{"username": "alice", "password": "alice-pass-123"}, 200)
	if l2["token"] == nil || l2["mfa_required"] != nil {
		t.Fatalf("login after disable = %v", l2)
	}
}

func TestEmailMFARequiresVerifiedEmail(t *testing.T) {
	env := startSMTP(t)
	carol := register(t, env, "carol", "carol-pass-123")
	// 未设置邮箱
	carol.mustFail("POST", "/me/mfa/email/enroll", nil, 400)
	// 邮箱未验证
	carol.mustStatus("POST", "/me/profile", map[string]string{"email": "carol@example.com"}, 200)
	carol.mustFail("POST", "/me/mfa/email/enroll", nil, 400)
}
