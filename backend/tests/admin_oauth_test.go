package tests

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"gitdash/backend/internal/api"
	"gitdash/backend/internal/store"
)

// startAPISeed 启动仅 HTTP 的实例并对 store 预置数据。
func startAPISeed(t *testing.T, seed func(*store.Store)) (*httptest.Server, *store.Store) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	if seed != nil {
		seed(st)
	}
	hs := httptest.NewServer(api.New(st, "test").Handler(""))
	t.Cleanup(hs.Close)
	return hs, st
}

func noRedirectClient() *http.Client {
	return &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}
}

func jarNoRedirectClient() (*http.Client, *cookiejar.Jar) {
	j, _ := cookiejar.New(nil)
	return &http.Client{Jar: j, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}, j
}

func TestAdminDisabledByDefault(t *testing.T) {
	hs, _ := startAPISeed(t, nil)
	want := func(method, path, body string, code int) {
		t.Helper()
		var rd *strings.Reader
		if body != "" {
			rd = strings.NewReader(body)
		} else {
			rd = strings.NewReader("")
		}
		req, _ := http.NewRequest(method, hs.URL+"/api"+path, rd)
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		if res.StatusCode != code {
			t.Fatalf("%s %s = %d, want %d", method, path, res.StatusCode, code)
		}
	}
	// 未引导 admin：一律 404（默认不启用）
	want("GET", "/admin/me", "", 404)
	want("POST", "/admin/login", `{"username":"admin","password":"x"}`, 404)
	want("GET", "/auth/github", "", 404)
	want("GET", "/auth/oidc/start", "", 404)

	// providers 默认全关
	req, _ := http.NewRequest("GET", hs.URL+"/api/auth/providers", nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var m struct {
		Github struct{ Enabled bool } `json:"github"`
		OIDC   struct{ Enabled bool } `json:"oidc"`
	}
	if err := json.NewDecoder(res.Body).Decode(&m); err != nil {
		t.Fatal(err)
	}
	if m.Github.Enabled || m.OIDC.Enabled {
		t.Fatalf("providers should be disabled: %+v", m)
	}
}

// TestAdminConfigOIDCLogin 管理面板配置 OIDC 后，用假 IdP 走完整登录。
func TestAdminConfigOIDCLogin(t *testing.T) {
	// 假 IdP
	var idp *httptest.Server
	var fake struct {
		tokenReq int
	}
	idp = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			writeJSONH(w, map[string]string{
				"issuer":                 idp.URL,
				"authorization_endpoint": idp.URL + "/authorize",
				"token_endpoint":         idp.URL + "/token",
				"userinfo_endpoint":      idp.URL + "/userinfo",
			})
		case "/token":
			fake.tokenReq++
			writeJSONH(w, map[string]string{"access_token": "fake-token"})
		case "/userinfo":
			if r.Header.Get("Authorization") != "Bearer fake-token" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			writeJSONH(w, map[string]string{
				"sub": "u-123", "preferred_username": "keycloak-alice", "email": "alice@example.com",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer idp.Close()

	hs, _ := startAPISeed(t, func(st *store.Store) {
		hash, _ := bcrypt.GenerateFromPassword([]byte("admin-pass-123"), bcrypt.DefaultCost)
		if err := st.CreateAdminUser("admin", string(hash)); err != nil {
			t.Fatalf("seed admin: %v", err)
		}
	})
	_ = fake.tokenReq

	jar, _ := cookiejar.New(nil)
	admin := &http.Client{Jar: jar}

	// admin 登录（cookie）
	req, _ := http.NewRequest("POST", hs.URL+"/api/admin/login", strings.NewReader(`{"username":"admin","password":"admin-pass-123"}`))
	req.Header.Set("Content-Type", "application/json")
	res, err := admin.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("admin login = %d", res.StatusCode)
	}

	// 配置 OIDC
	save, _ := http.NewRequest("POST", hs.URL+"/api/admin/settings", strings.NewReader(fmt.Sprintf(
		`{"oidc_enabled":true,"oidc_name":"Keycloak","oidc_issuer":%q,"oidc_client_id":"cid","oidc_client_secret":"csecret-very-long"}`,
		idp.URL)))
	save.Header.Set("Content-Type", "application/json")
	r2, err := admin.Do(save)
	if err != nil {
		t.Fatal(err)
	}
	_ = r2.Body.Close()
	if r2.StatusCode != 200 {
		t.Fatalf("save settings = %d", r2.StatusCode)
	}

	// admin settings 确认已保存
	gs, _ := http.NewRequest("GET", hs.URL+"/api/admin/settings", nil)
	rs, _ := admin.Do(gs)
	bs, _ := io.ReadAll(rs.Body)
	rs.Body.Close()
	t.Logf("admin settings raw = %s", bs)

	// providers 公开可见（无 cookie）
	pub, _ := http.NewRequest("GET", hs.URL+"/api/auth/providers", nil)
	r3, err := noRedirectClient().Do(pub)
	if err != nil {
		t.Fatal(err)
	}
	defer r3.Body.Close()
	var prov struct {
		OIDC struct {
			Enabled bool   `json:"enabled"`
			Name    string `json:"name"`
		} `json:"oidc"`
	}
	if err := json.NewDecoder(r3.Body).Decode(&prov); err != nil {
		t.Fatal(err)
	}
	if !prov.OIDC.Enabled || prov.OIDC.Name != "Keycloak" {
		t.Fatalf("providers = %+v", prov)
	}

	// OIDC 登录全流程（模拟 IdP 回调）
	start, _ := http.NewRequest("GET", hs.URL+"/api/auth/oidc/start", nil)
	r4, err := noRedirectClient().Do(start)
	if err != nil {
		t.Fatal(err)
	}
	_ = r4.Body.Close()
	loc := r4.Header.Get("Location")
	if r4.StatusCode != 302 || !strings.Contains(loc, "/authorize?") {
		t.Fatalf("start = %d %s", r4.StatusCode, loc)
	}
	u, _ := url.Parse(loc)
	state := u.Query().Get("state")

	cb, _ := http.NewRequest("GET", hs.URL+"/api/auth/oidc/callback?state="+url.QueryEscape(state)+"&code=xyz", nil)
	user, _ := jarNoRedirectClient()
	r5, err := user.Do(cb)
	if err != nil {
		t.Fatal(err)
	}
	_ = r5.Body.Close()
	if r5.StatusCode != 302 {
		t.Fatalf("callback = %d", r5.StatusCode)
	}
	// 用户已建立并登录
	me, _ := http.NewRequest("GET", hs.URL+"/api/me", nil)
	r6, err := user.Do(me)
	if err != nil {
		t.Fatal(err)
	}
	defer r6.Body.Close()
	var who struct {
		Username string `json:"username"`
	}
	if err := json.NewDecoder(r6.Body).Decode(&who); err != nil {
		t.Fatal(err)
	}
	if who.Username != "keycloak-alice" {
		t.Fatalf("me = %+v", who)
	}

	// 再次登录同 sub：绑定同一账号（不新建）
	start2, _ := http.NewRequest("GET", hs.URL+"/api/auth/oidc/start", nil)
	r7, err := noRedirectClient().Do(start2)
	if err != nil {
		t.Fatal(err)
	}
	_ = r7.Body.Close()
	u2, _ := url.Parse(r7.Header.Get("Location"))
	cb2, _ := http.NewRequest("GET", hs.URL+"/api/auth/oidc/callback?state="+url.QueryEscape(u2.Query().Get("state"))+"&code=abc", nil)
	user2, _ := jarNoRedirectClient()
	r8, err := user2.Do(cb2)
	if err != nil {
		t.Fatal(err)
	}
	_ = r8.Body.Close()
	me2, _ := http.NewRequest("GET", hs.URL+"/api/me", nil)
	r9, err := user2.Do(me2)
	if err != nil {
		t.Fatal(err)
	}
	defer r9.Body.Close()
	if err := json.NewDecoder(r9.Body).Decode(&who); err != nil {
		t.Fatal(err)
	}
	if who.Username != "keycloak-alice" {
		t.Fatalf("relogin me = %+v", who)
	}
}

func writeJSONH(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
