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
	c.mustFail("POST", "/auth/register", map[string]string{"username": "bob", "password": "short"}, 400)
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
