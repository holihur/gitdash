package tests

// adminban_test.go 管理端封禁（用户 / 仓库 / 组织）与 template 用户黑盒测试。

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

// adminBaseDo 直接对 env.BaseURL 发起管理端请求（带 admin cookie）。
func adminBaseDo(t *testing.T, env *Env, cookie, method, path, body string) *http.Response {
	t.Helper()
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, env.BaseURL+"/api"+path, rd)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if cookie != "" {
		req.Header.Set("Cookie", "gitdash_admin="+cookie)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func adminBaseLogin(t *testing.T, env *Env) string {
	t.Helper()
	res := adminBaseDo(t, env, "", "POST", "/admin/login",
		`{"username":"admin","password":"admin-pass-123"}`)
	defer func() { _ = res.Body.Close() }()
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

func adminBaseCall(t *testing.T, env *Env, cookie, method, path, body string) (int, any) {
	t.Helper()
	res := adminBaseDo(t, env, cookie, method, path, body)
	defer func() { _ = res.Body.Close() }()
	var v any
	_ = json.NewDecoder(res.Body).Decode(&v)
	return res.StatusCode, v
}

func bootstrapAdmin(t *testing.T, env *Env) string {
	t.Helper()
	if err := env.Store.EnsureTemplateUser("template-hash"); err != nil {
		t.Fatalf("ensure template user: %v", err)
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte("admin-pass-123"), bcrypt.DefaultCost)
	if err := env.Store.CreateAdminUser("admin", string(hash)); err != nil {
		t.Fatalf("create admin: %v", err)
	}
	return adminBaseLogin(t, env)
}

func TestAdminBanUserAndRepo(t *testing.T) {
	env := start(t)
	tok := bootstrapAdmin(t, env)

	alice := register(t, env, "alice-ban", "password-1")
	bob := register(t, env, "bob-ban", "password-1")
	alice.mustStatus("POST", "/repos", map[string]any{"name": "proj", "private": false}, 201)

	// 封禁用户 → 登录 403，现有会话失效
	if code, _ := adminBaseCall(t, env, tok, "POST", "/admin/users/alice-ban/ban", `{"banned":true}`); code != 204 {
		t.Fatalf("ban user = %d, want 204", code)
	}
	anon := &Client{env: env}
	if code, _ := anon.do("POST", "/auth/login", map[string]string{"username": "alice-ban", "password": "password-1"}); code != 403 {
		t.Fatalf("banned login = %d, want 403", code)
	}
	if code, _ := alice.do("GET", "/me", nil); code != 401 {
		t.Fatalf("banned session = %d, want 401", code)
	}
	// 收件箱收到封禁通知
	notes, err := env.Store.ListNotifications("alice-ban", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, n := range notes {
		if n.Kind == "system" && n.Action == "banned_user" && n.Actor == "admin" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected banned_user inbox notification, got %+v", notes)
	}

	// 解封后可登录，且解封不发消息
	if code, _ := adminBaseCall(t, env, tok, "POST", "/admin/users/alice-ban/ban", `{"banned":false}`); code != 204 {
		t.Fatalf("unban user = %d, want 204", code)
	}
	after, _ := env.Store.ListNotifications("alice-ban", 10, 0)
	if len(after) != len(notes) {
		t.Fatalf("unban should not create notification: before %d after %d", len(notes), len(after))
	}
	fresh := &Client{env: env}
	fresh.mustStatus("POST", "/auth/login", map[string]string{"username": "alice-ban", "password": "password-1"}, 200)

	// 封禁仓库 → 他人无法访问（404）
	if code, _ := adminBaseCall(t, env, tok, "POST", "/admin/repos/alice-ban/proj/ban", `{"banned":true}`); code != 204 {
		t.Fatalf("ban repo = %d, want 204", code)
	}
	bob.mustFail("GET", "/users/alice-ban/repos/proj", nil, 404)

	// 管理端仓库列表可查到并带 banned 标记
	res := adminBaseDo(t, env, tok, "GET", "/admin/repos?q=proj", "")
	if res.StatusCode != 200 {
		t.Fatalf("admin list repos = %d", res.StatusCode)
	}
	var repos []map[string]any
	_ = json.NewDecoder(res.Body).Decode(&repos)
	_ = res.Body.Close()
	if len(repos) != 1 || repos[0]["banned"] != true {
		t.Fatalf("admin repos = %+v", repos)
	}

	// 解封仓库 → 恢复可读
	if code, _ := adminBaseCall(t, env, tok, "POST", "/admin/repos/alice-ban/proj/ban", `{"banned":false}`); code != 204 {
		t.Fatalf("unban repo = %d, want 204", code)
	}
	if code, _ := bob.do("GET", "/users/alice-ban/repos/proj", nil); code != 200 {
		t.Fatalf("repo after unban = %d, want 200", code)
	}
}

func TestAdminBanOrgAndTemplateProtection(t *testing.T) {
	env := start(t)
	tok := bootstrapAdmin(t, env)

	alice := register(t, env, "alice-org-ban", "password-1")
	bob := register(t, env, "bob-org-ban", "password-1")
	alice.mustStatus("POST", "/orgs", map[string]string{"name": "teamban", "display": "Team"}, 201)
	alice.mustStatus("POST", "/repos", map[string]any{"name": "svc", "namespace": "teamban", "private": false}, 201)
	if code, _ := bob.do("GET", "/users/teamban/repos/svc", nil); code != 200 {
		t.Fatalf("org repo before ban = %d, want 200", code)
	}

	// 封禁组织 → 其下仓库不可访问；org owner 收到系统通知
	if code, _ := adminBaseCall(t, env, tok, "POST", "/admin/orgs/teamban/ban", `{"banned":true}`); code != 204 {
		t.Fatalf("ban org = %d, want 204", code)
	}
	if code, _ := bob.do("GET", "/users/teamban/repos/svc", nil); code != 404 {
		t.Fatalf("org repo after ban = %d, want 404", code)
	}
	notes, _ := env.Store.ListNotifications("alice-org-ban", 10, 0)
	found := false
	for _, n := range notes {
		if n.Kind == "system" && n.Action == "banned_org" && n.Owner == "teamban" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected banned_org notification, got %+v", notes)
	}
	// 组织列表隐藏
	res := adminBaseDo(t, env, tok, "GET", "/admin/orgs?q=teamban", "")
	var orgs []map[string]any
	_ = json.NewDecoder(res.Body).Decode(&orgs)
	_ = res.Body.Close()
	if len(orgs) != 1 || orgs[0]["banned"] != true {
		t.Fatalf("admin orgs = %+v", orgs)
	}

	// template 用户受保护：不可解封、不可删除
	if code, _ := adminBaseCall(t, env, tok, "POST", "/admin/users/template/ban", `{"banned":false}`); code != 403 {
		t.Fatalf("unban template = %d, want 403", code)
	}
	if code, _ := adminBaseCall(t, env, tok, "DELETE", "/admin/users/template", ""); code != 403 {
		t.Fatalf("delete template = %d, want 403", code)
	}
	if !env.Store.IsUserBanned("template") {
		t.Fatal("template user must stay banned")
	}
}
