package tests

import (
	"testing"
)

func TestRegisterLoginSession(t *testing.T) {
	env := start(t)

	// 注册成功并自动登录
	c := register(t, env, "alice", "alice-pass-123")
	m := c.mustStatus("GET", "/me", nil, 200)
	if m["username"] != "alice" {
		t.Fatalf("me = %v", m)
	}

	// 重复注册被拒绝
	c.mustFail("POST", "/auth/register", map[string]string{"username": "alice", "password": "alice-pass-123"}, 409)

	// 弱密码 / 非法用户名
	c.mustFail("POST", "/auth/register", map[string]string{"username": "bobby", "password": "short"}, 400)
	c.mustFail("POST", "/auth/register", map[string]string{"username": "-bad", "password": "long-enough-pw"}, 400)
	c.mustFail("POST", "/auth/register", map[string]string{"username": "a", "password": "long-enough-pw"}, 400)
	c.mustFail("POST", "/auth/register", map[string]string{"username": "has space", "password": "long-enough-pw"}, 400)

	// 错误密码 / 未知用户
	c.mustFail("POST", "/auth/login", map[string]string{"username": "alice", "password": "wrong-pass-99"}, 401)
	c.mustFail("POST", "/auth/login", map[string]string{"username": "nobody", "password": "wrong-pass-99"}, 401)

	// 重新登录拿到新会话
	c2 := &Client{env: env}
	m = c2.mustStatus("POST", "/auth/login", map[string]string{"username": "alice", "password": "alice-pass-123"}, 200)
	token, _ := m["token"].(string)
	if token == "" {
		t.Fatal("login: empty token")
	}
	c2.token = token
	c2.mustStatus("GET", "/me", nil, 200)

	// 登出后会话失效
	c2.mustStatus("POST", "/auth/logout", nil, 204)
	c2.mustFail("GET", "/me", nil, 401)

	// 无 token 访问
	noAuth := &Client{env: env}
	noAuth.mustFail("GET", "/me", nil, 401)

	// 注册时的会话不受 alice 其他会话登出影响
	c.mustStatus("GET", "/me", nil, 200)
}

func TestRegisterPasswordStrength(t *testing.T) {
	env := start(t)
	c := &Client{env: env}

	// 长度足够但字符类别不足（仅小写字母）
	c.mustFail("POST", "/auth/register", map[string]string{"username": "weakuser", "password": "alllowercaseonly"}, 400)
	// 长度不足
	c.mustFail("POST", "/auth/register", map[string]string{"username": "weakuser", "password": "Ab1!x"}, 400)
	// 用户名过短（<5 位）被拒绝
	c.mustFail("POST", "/auth/register", map[string]string{"username": "abcd", "password": "Passw0rd-123"}, 400)
	// 至少 3 类字符且长度足够
	m := c.mustStatus("POST", "/auth/register", map[string]string{"username": "stronguser", "password": "Passw0rd-123"}, 201)
	if token, _ := m["token"].(string); token == "" {
		t.Fatal("strong password register: empty token")
	}
}

func TestSessionRequired(t *testing.T) {
	env := start(t)

	// 未登录访问受保护端点
	for _, tc := range []struct{ method, path string }{
		{"GET", "/repos"},
		{"POST", "/repos"},
		{"GET", "/keys"},
		{"POST", "/keys"},
	} {
		c := &Client{env: env}
		c.mustFail(tc.method, tc.path, nil, 401)
	}

	// 无效 token
	c := &Client{env: env, token: "not-a-real-token"}
	c.mustFail("GET", "/me", nil, 401)
}
