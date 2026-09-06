package tests

// adminusers_test.go 管理端用户管理（列表 / 创建 / 重置密码 / 删除）黑盒测试。

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"gitdash/backend/internal/api"
	"gitdash/backend/internal/store"
)

// adminLogin 用 admin 账号登录并返回 admin 会话 cookie 值。
func adminLogin(t *testing.T, hs *httptest.Server) string {
	t.Helper()
	res := mustAdminDo(t, hs, "", "POST", "/admin/login",
		`{"username":"admin","password":"admin-pass-123"}`)
	if res.StatusCode != 200 {
		t.Fatalf("admin login = %d", res.StatusCode)
	}
	for _, c := range res.Cookies() {
		if c.Name == "gitdash_admin" {
			return c.Value
		}
	}
	t.Fatal("no admin cookie")
	return ""
}

func mustAdminDo(t *testing.T, hs *httptest.Server, cookie, method, path, body string) *http.Response {
	t.Helper()
	var rd io.Reader
	if body != "" {
		rd = bytes.NewReader([]byte(body))
	}
	req, err := http.NewRequest(method, hs.URL+"/api"+path, rd)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if cookie != "" {
		req.Header.Set("Cookie", "gitdash_admin="+cookie)
	}
	res, err := hs.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// adminDo 请求管理端点并返回 (状态码, 解码后的 JSON)。
func adminDo(t *testing.T, hs *httptest.Server, cookie, method, path, body string) (int, any) {
	t.Helper()
	res := mustAdminDo(t, hs, cookie, method, path, body)
	defer func() { _ = res.Body.Close() }()
	var v any
	_ = json.NewDecoder(res.Body).Decode(&v)
	return res.StatusCode, v
}

func TestAdminUsersCRUD(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte("admin-pass-123"), bcrypt.DefaultCost)
	if err := st.CreateAdminUser("admin", string(hash)); err != nil {
		t.Fatal(err)
	}
	hs := httptest.NewServer(api.New(st, "test").Handler(""))
	defer hs.Close()
	tok := adminLogin(t, hs)

	// 未登录访问被拒
	if code, _ := adminDo(t, hs, "", "GET", "/admin/users", ""); code != 401 {
		t.Fatalf("unauthed list = %d, want 401", code)
	}

	// 造两个普通用户
	env := &Env{t: t, BaseURL: hs.URL}
	register(t, env, "alice-admin", "password-1")
	register(t, env, "bob-admin", "password-1")

	// 列表：返回数组 + X-Total-Count
	res := mustAdminDo(t, hs, tok, "GET", "/admin/users", "")
	if res.StatusCode != 200 {
		t.Fatalf("list = %d", res.StatusCode)
	}
	var users []map[string]any
	if err := json.NewDecoder(res.Body).Decode(&users); err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if len(users) != 2 {
		t.Fatalf("list = %v", users)
	}

	// q 过滤
	code, v := adminDo(t, hs, tok, "GET", "/admin/users?q=alice", "")
	if code != 200 {
		t.Fatalf("filter = %d", code)
	}
	if filtered := v.([]any); len(filtered) != 1 {
		t.Fatalf("filter = %v", filtered)
	}

	// 创建用户（含邮箱）
	if code, _ := adminDo(t, hs, tok, "POST", "/admin/users",
		`{"username":"carol-admin","password":"password-1","email":"c@x.io"}`); code != 201 {
		t.Fatalf("create = %d, want 201", code)
	}
	// 重名 / 弱密码被拒
	if code, _ := adminDo(t, hs, tok, "POST", "/admin/users", `{"username":"carol-admin","password":"password-1"}`); code != 409 {
		t.Fatalf("dup create = %d, want 409", code)
	}
	if code, _ := adminDo(t, hs, tok, "POST", "/admin/users", `{"username":"dave-admin","password":"short"}`); code != 400 {
		t.Fatalf("weak create = %d, want 400", code)
	}

	// carol 能用新密码登录
	if code, _ := adminDo(t, hs, "", "POST", "/auth/login", `{"username":"carol-admin","password":"password-1"}`); code != 200 {
		t.Fatalf("carol login = %d", code)
	}

	// 重置密码
	if code, _ := adminDo(t, hs, tok, "POST", "/admin/users/carol-admin/reset_password", `{"password":"newpass-123"}`); code != 204 {
		t.Fatalf("reset = %d, want 204", code)
	}
	if code, _ := adminDo(t, hs, "", "POST", "/auth/login", `{"username":"carol-admin","password":"password-1"}`); code != 401 {
		t.Fatalf("old password still valid = %d", code)
	}
	if code, _ := adminDo(t, hs, "", "POST", "/auth/login", `{"username":"carol-admin","password":"newpass-123"}`); code != 200 {
		t.Fatalf("new password login = %d", code)
	}
	// 重置/删除不存在的用户 → 404
	if code, _ := adminDo(t, hs, tok, "POST", "/admin/users/ghost/reset_password", `{"password":"newpass-123"}`); code != 404 {
		t.Fatalf("reset ghost = %d, want 404", code)
	}

	// 删除用户
	if code, _ := adminDo(t, hs, tok, "DELETE", "/admin/users/carol-admin", ""); code != 204 {
		t.Fatalf("delete = %d, want 204", code)
	}
	if code, _ := adminDo(t, hs, "", "POST", "/auth/login", `{"username":"carol-admin","password":"newpass-123"}`); code != 401 {
		t.Fatalf("deleted user login = %d", code)
	}
	code, v = adminDo(t, hs, tok, "GET", "/admin/users", "")
	if code != 200 {
		t.Fatalf("list after delete = %d", code)
	}
	for _, u := range v.([]any) {
		name := u.(map[string]any)["username"].(string)
		if name == "carol-admin" {
			t.Fatalf("deleted user still listed: %v", v)
		}
	}
	if code, _ := adminDo(t, hs, tok, "DELETE", "/admin/users/carol-admin", ""); code != 404 {
		t.Fatalf("delete ghost = %d, want 404", code)
	}
}
