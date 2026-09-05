package tests

import (
	"net/http"
	"net/http/cookiejar"
	"strings"
	"testing"
	"time"

	"gitdash/backend/internal/totp"
)

func mfaCode(t *testing.T, secret string) string {
	t.Helper()
	// 用未来 1 步内的当前时间窗口；直接取当前时间（与服务端同机）
	c, err := totp.Code(secret, time.Now().Add(-2*time.Second))
	if err != nil {
		t.Fatalf("totp code: %v", err)
	}
	return c
}

func TestProfileInfoAndPasswordChange(t *testing.T) {
	env := start(t)
	alice := register(t, env, "alice", "alice-pass-123")

	// me 返回资料字段
	m := alice.mustStatus("GET", "/me", nil, 200)
	if m["username"] != "alice" || m["created_at"] == nil || m["mfa_enabled"] != false {
		t.Fatalf("me = %v", m)
	}

	// 错误旧密码
	alice.mustFail("POST", "/me/password",
		map[string]string{"current_password": "wrong-old", "new_password": "brand-new-pass-1"}, 401)
	// 新密码太短
	alice.mustFail("POST", "/me/password",
		map[string]string{"current_password": "alice-pass-123", "new_password": "short"}, 400)

	// 改密成功
	alice.mustStatus("POST", "/me/password",
		map[string]string{"current_password": "alice-pass-123", "new_password": "brand-new-pass-1"}, 204)

	// 旧密码登录失败、新密码成功
	alice.mustFail("POST", "/auth/login", map[string]string{"username": "alice", "password": "alice-pass-123"}, 401)
	login := alice.mustStatus("POST", "/auth/login",
		map[string]string{"username": "alice", "password": "brand-new-pass-1"}, 200)
	if login["token"] == nil {
		t.Fatalf("login after password change = %v", login)
	}

	// 未认证访问资料端点
	(&Client{env: env}).mustFail("GET", "/me/mfa", nil, 401)
}

func TestMFAEnrollLoginDisable(t *testing.T) {
	env := start(t)
	alice := register(t, env, "alice", "alice-pass-123")

	// 初始未启用
	if m := alice.mustStatus("GET", "/me/mfa", nil, 200); m["enabled"] != false {
		t.Fatalf("mfa status = %v", m)
	}

	// 注册：拿到 secret 与 otpauth URL
	enroll := alice.mustStatus("POST", "/me/mfa/enroll", nil, 200)
	secret, _ := enroll["secret"].(string)
	if secret == "" || enroll["otpauth_url"] == nil {
		t.Fatalf("enroll = %v", enroll)
	}
	// 再次 enroll 复用同一 secret
	enroll2 := alice.mustStatus("POST", "/me/mfa/enroll", nil, 200)
	if enroll2["secret"] != secret {
		t.Fatalf("enroll should be idempotent: %v vs %v", enroll2, enroll)
	}

	// 错误验证码无法激活
	alice.mustFail("POST", "/me/mfa/activate", map[string]string{"code": "000000"}, 400)
	// 正确验证码激活
	alice.mustStatus("POST", "/me/mfa/activate", map[string]string{"code": mfaCode(t, secret)}, 204)
	if m := alice.mustStatus("GET", "/me/mfa", nil, 200); m["enabled"] != true {
		t.Fatalf("mfa after activate = %v", m)
	}
	// 已启用不可再 enroll
	alice.mustFail("POST", "/me/mfa/enroll", nil, 409)

	// 登录需要二次验证
	m := alice.mustStatus("POST", "/auth/login",
		map[string]string{"username": "alice", "password": "alice-pass-123"}, 200)
	if m["mfa_required"] != true || m["token"] != nil {
		t.Fatalf("login should require mfa = %v", m)
	}
	mfaToken, _ := m["mfa_token"].(string)

	// 错误验证码拒绝；正确验证码签发会话
	alice.mustFail("POST", "/auth/mfa-verify",
		map[string]string{"mfa_token": mfaToken, "code": "000000"}, 401)
	ok := alice.mustStatus("POST", "/auth/mfa-verify",
		map[string]string{"mfa_token": mfaToken, "code": mfaCode(t, secret)}, 200)
	if ok["token"] == nil {
		t.Fatalf("mfa verify = %v", ok)
	}
	// mfa_token 一次性
	alice.mustFail("POST", "/auth/mfa-verify",
		map[string]string{"mfa_token": mfaToken, "code": mfaCode(t, secret)}, 401)

	// 禁用：需要密码 + 验证码；错误密码拒绝
	alice.mustFail("POST", "/me/mfa/disable",
		map[string]string{"password": "wrong", "code": mfaCode(t, secret)}, 401)
	alice.mustStatus("POST", "/me/mfa/disable",
		map[string]string{"password": "alice-pass-123", "code": mfaCode(t, secret)}, 204)
	if m := alice.mustStatus("GET", "/me/mfa", nil, 200); m["enabled"] != false {
		t.Fatalf("mfa after disable = %v", m)
	}
	// 禁用后登录无需二次验证
	alice.mustFail("POST", "/me/mfa/disable",
		map[string]string{"password": "alice-pass-123", "code": "000000"}, 409)
	login := alice.mustStatus("POST", "/auth/login",
		map[string]string{"username": "alice", "password": "alice-pass-123"}, 200)
	if login["token"] == nil || login["mfa_required"] != nil {
		t.Fatalf("login after disable = %v", login)
	}
}

func TestMFAIsolationAndCodes(t *testing.T) {
	env := start(t)
	alice := register(t, env, "alice", "alice-pass-123")
	bob := register(t, env, "bob", "bob-pass-123456")

	// bob 启用 MFA 不影响 alice
	enroll := bob.mustStatus("POST", "/me/mfa/enroll", nil, 200)
	secret, _ := enroll["secret"].(string)
	bob.mustStatus("POST", "/me/mfa/activate", map[string]string{"code": mfaCode(t, secret)}, 204)

	aliceLogin := alice.mustStatus("POST", "/auth/login",
		map[string]string{"username": "alice", "password": "alice-pass-123"}, 200)
	if aliceLogin["token"] == nil {
		t.Fatalf("alice should not require mfa: %v", aliceLogin)
	}

	// 无效 mfa_token / 过期
	alice.mustFail("POST", "/auth/mfa-verify",
		map[string]string{"mfa_token": "bogus", "code": mfaCode(t, secret)}, 401)
	// bob 仍可用 MFA 登录
	bobLogin := bob.mustStatus("POST", "/auth/login",
		map[string]string{"username": "bob", "password": "bob-pass-123456"}, 200)
	tok, _ := bobLogin["mfa_token"].(string)
	bob.mustStatus("POST", "/auth/mfa-verify", map[string]string{"mfa_token": tok, "code": mfaCode(t, secret)}, 200)
}

func TestPasswordChangeRevokesOtherSessions(t *testing.T) {
	env := start(t)
	alice := register(t, env, "alice", "alice-pass-123")

	// 另一台设备登录获得独立会话
	login := alice.mustStatus("POST", "/auth/login",
		map[string]string{"username": "alice", "password": "alice-pass-123"}, 200)
	otherToken, _ := login["token"].(string)
	other := &Client{env: env, token: otherToken}
	other.mustStatus("GET", "/me", nil, 200)

	// 改密：当前会话保留，其它会话被撤销
	alice.mustStatus("POST", "/me/password",
		map[string]string{"current_password": "alice-pass-123", "new_password": "brand-new-pass-1"}, 204)
	alice.mustStatus("GET", "/me", nil, 200) // 当前 token 仍有效
	other.mustFail("GET", "/me", nil, 401)   // 其它会话已失效
}

func TestMFAVerifyAttemptLimit(t *testing.T) {
	env := start(t)
	alice := register(t, env, "alice", "alice-pass-123")
	e := alice.mustStatus("POST", "/me/mfa/enroll", nil, 200)
	secret, _ := e["secret"].(string)
	alice.mustStatus("POST", "/me/mfa/activate", map[string]string{"code": mfaCode(t, secret)}, 204)

	login := alice.mustStatus("POST", "/auth/login",
		map[string]string{"username": "alice", "password": "alice-pass-123"}, 200)
	tok, _ := login["mfa_token"].(string)

	// 连续输错 5 次：挑战被销毁（防暴力枚举 6 位验证码）
	for i := 0; i < 4; i++ {
		alice.mustFail("POST", "/auth/mfa-verify",
			map[string]string{"mfa_token": tok, "code": "000000"}, 401)
	}
	alice.mustFail("POST", "/auth/mfa-verify",
		map[string]string{"mfa_token": tok, "code": "000000"}, 429)
	// 令牌已失效：即使验证码正确也被拒
	alice.mustFail("POST", "/auth/mfa-verify",
		map[string]string{"mfa_token": tok, "code": mfaCode(t, secret)}, 401)
}

func TestSecurityHeadersAndWebhookSSRFGuard(t *testing.T) {
	env := start(t)
	req, _ := http.NewRequest("GET", env.BaseURL+"/api/health", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.Header.Get("X-Content-Type-Options") != "nosniff" ||
		resp.Header.Get("X-Frame-Options") != "DENY" ||
		resp.Header.Get("Referrer-Policy") != "no-referrer" {
		t.Fatalf("missing security headers: %v", resp.Header)
	}
	if !strings.Contains(resp.Header.Get("Content-Security-Policy"), "default-src 'self'") {
		t.Fatalf("missing CSP: %v", resp.Header.Get("Content-Security-Policy"))
	}
}

func TestSessionCookieAndCSRFGuard(t *testing.T) {
	jar, _ := cookiejar.New(nil)
	env := start(t)
	hc := &http.Client{Jar: jar}

	// 注册（无 Authorization 头）→ cookie 会话自动建立
	req, _ := http.NewRequest("POST", env.BaseURL+"/api/auth/register", strings.NewReader(`{"username":"cooky","password":"cookie-pass-123"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := hc.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != 201 {
		t.Fatalf("register = %d", resp.StatusCode)
	}
	// 不带 token，仅凭 cookie 访问 /me
	me, _ := http.NewRequest("GET", env.BaseURL+"/api/me", nil)
	r2, err := hc.Do(me)
	if err != nil {
		t.Fatal(err)
	}
	_ = r2.Body.Close()
	if r2.StatusCode != 200 {
		t.Fatalf("cookie session /me = %d", r2.StatusCode)
	}
	// 跨站 Origin 的写请求被拒绝
	post, _ := http.NewRequest("POST", env.BaseURL+"/api/repos", strings.NewReader(`{"name":"x"}`))
	post.Header.Set("Content-Type", "application/json")
	post.Header.Set("Origin", "http://evil.example")
	r3, err := hc.Do(post)
	if err != nil {
		t.Fatal(err)
	}
	_ = r3.Body.Close()
	if r3.StatusCode != 403 {
		t.Fatalf("csrf guard = %d", r3.StatusCode)
	}
	// 登出清 cookie，会话失效
	lo, _ := http.NewRequest("POST", env.BaseURL+"/api/auth/logout", nil)
	r4, err := hc.Do(lo)
	if err != nil {
		t.Fatal(err)
	}
	_ = r4.Body.Close()
	me2, _ := http.NewRequest("GET", env.BaseURL+"/api/me", nil)
	r5, err := hc.Do(me2)
	if err != nil {
		t.Fatal(err)
	}
	_ = r5.Body.Close()
	if r5.StatusCode != 401 {
		t.Fatalf("after logout /me = %d", r5.StatusCode)
	}
}

func TestLoginRateLimit(t *testing.T) {
	env := start(t)
	register(t, env, "alice", "alice-pass-123")
	anon := &Client{env: env}
	// 5 次失败 → 第 6 次即使密码正确也被限速
	for i := 0; i < 5; i++ {
		anon.mustFail("POST", "/auth/login",
			map[string]string{"username": "alice", "password": "wrong-pass-999"}, 401)
	}
	anon.mustFail("POST", "/auth/login",
		map[string]string{"username": "alice", "password": "alice-pass-123"}, 429)
}
