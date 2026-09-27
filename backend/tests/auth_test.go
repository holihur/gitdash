package tests

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// TestSessionTokenOmittedForBrowser 覆盖安全审计 M2.3：浏览器请求（带
// Sec-Fetch-*）不返回明文会话 token（只下发 HttpOnly cookie）；非浏览器客户端
// 保持兼容。
func TestSessionTokenOmittedForBrowser(t *testing.T) {
	env := start(t)
	do := func(username string, browser bool) map[string]any {
		t.Helper()
		body, _ := json.Marshal(map[string]string{"username": username, "password": "password-123456"})
		req, _ := http.NewRequest("POST", env.BaseURL+"/api/auth/register", strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")
		if browser {
			req.Header.Set("Sec-Fetch-Site", "same-origin")
			req.Header.Set("Sec-Fetch-Mode", "cors")
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = res.Body.Close() }()
		var m map[string]any
		_ = json.NewDecoder(res.Body).Decode(&m)
		return m
	}

	if m := do("browseruser", true); m["token"] != nil {
		t.Fatalf("browser response leaked token: %v", m)
	} else if m["username"] != "browseruser" {
		t.Fatalf("register response = %v", m)
	}
	if m := do("cliuser", false); m["token"] == nil {
		t.Fatalf("non-browser client missing token: %v", m)
	}
}

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
	// 用户名过短（<4 位）被拒绝
	c.mustFail("POST", "/auth/register", map[string]string{"username": "abc", "password": "Passw0rd-123"}, 400)
	// 边界：4 位用户名合法
	m4 := c.mustStatus("POST", "/auth/register", map[string]string{"username": "abcd", "password": "Passw0rd-123"}, 201)
	if token, _ := m4["token"].(string); token == "" {
		t.Fatal("4-char username register: empty token")
	}
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
