package tests

import (
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"gitdash/backend/internal/store"
)

// TestAdminConfigGoogleLogin 管理面板开启 Google 登录后，用假 Google 端点走完整授权码流程。
func TestAdminConfigGoogleLogin(t *testing.T) {
	var tokens int
	google := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth":
			rd := r.URL.Query().Get("redirect_uri")
			if rd == "" {
				http.Error(w, "missing redirect_uri", http.StatusBadRequest)
				return
			}
			http.Redirect(w, r, rd+"?code=fake-code&state="+url.QueryEscape(r.URL.Query().Get("state")), http.StatusFound)
		case "/token":
			tokens++
			if err := r.ParseForm(); err != nil || r.FormValue("client_id") == "" || r.FormValue("client_secret") == "" {
				http.Error(w, "bad token request", http.StatusBadRequest)
				return
			}
			writeJSONH(w, map[string]string{"access_token": "fake-google-token"})
		case "/userinfo":
			if r.Header.Get("Authorization") != "Bearer fake-google-token" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			writeJSONH(w, map[string]any{
				"sub": "google-123", "email": "Alice.Smith@gmail.com", "email_verified": true, "name": "Alice Smith",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer google.Close()

	t.Setenv("GITDASH_GOOGLE_AUTH_URL", google.URL+"/auth")
	t.Setenv("GITDASH_GOOGLE_TOKEN_URL", google.URL+"/token")
	t.Setenv("GITDASH_GOOGLE_USERINFO_URL", google.URL+"/userinfo")

	hs, _ := startAPISeed(t, func(st *store.Store) {
		hash, _ := bcrypt.GenerateFromPassword([]byte("admin-pass-123"), bcrypt.DefaultCost)
		if err := st.CreateAdminUser("admin", string(hash)); err != nil {
			t.Fatalf("seed admin: %v", err)
		}
	})

	jar, _ := cookiejar.New(nil)
	admin := &http.Client{Jar: jar}
	login, _ := http.NewRequest("POST", hs.URL+"/api/admin/login",
		strings.NewReader(`{"username":"admin","password":"admin-pass-123"}`))
	login.Header.Set("Content-Type", "application/json")
	rl, err := admin.Do(login)
	if err != nil {
		t.Fatal(err)
	}
	_ = rl.Body.Close()
	if rl.StatusCode != 200 {
		t.Fatalf("admin login = %d", rl.StatusCode)
	}

	// 开启 Google 登录
	save, _ := http.NewRequest("POST", hs.URL+"/api/admin/settings", strings.NewReader(
		`{"google_oauth_enabled":true,"google_client_id":"gcid","google_client_secret":"gsecret-very-long"}`))
	save.Header.Set("Content-Type", "application/json")
	rs, err := admin.Do(save)
	if err != nil {
		t.Fatal(err)
	}
	_ = rs.Body.Close()
	if rs.StatusCode != 200 {
		t.Fatalf("save settings = %d", rs.StatusCode)
	}

	// providers 公开可见
	pub, _ := http.NewRequest("GET", hs.URL+"/api/auth/providers", nil)
	rp, err := noRedirectClient().Do(pub)
	if err != nil {
		t.Fatal(err)
	}
	var prov struct {
		Google struct {
			Enabled bool `json:"enabled"`
		} `json:"google"`
	}
	if err := json.NewDecoder(rp.Body).Decode(&prov); err != nil {
		t.Fatal(err)
	}
	_ = rp.Body.Close()
	if !prov.Google.Enabled {
		t.Fatalf("google provider not enabled: %+v", prov)
	}

	// 完整授权码流程
	user, _ := jarNoRedirectClient()
	start, _ := http.NewRequest("GET", hs.URL+"/api/auth/google", nil)
	r1, err := user.Do(start)
	if err != nil {
		t.Fatal(err)
	}
	_ = r1.Body.Close()
	loc := r1.Header.Get("Location")
	if r1.StatusCode != 302 || !strings.Contains(loc, google.URL+"/auth?") || !strings.Contains(loc, "client_id=gcid") {
		t.Fatalf("start = %d %s", r1.StatusCode, loc)
	}

	// 假授权端点 → 跳回我们的回调
	authReq, _ := http.NewRequest("GET", loc, nil)
	r2, err := user.Do(authReq)
	if err != nil {
		t.Fatal(err)
	}
	_ = r2.Body.Close()
	cbLoc := r2.Header.Get("Location")
	if r2.StatusCode != 302 || !strings.Contains(cbLoc, "/api/auth/google/callback") {
		t.Fatalf("authorize = %d %s", r2.StatusCode, cbLoc)
	}

	// 回调：换 token + 拉 userinfo + 建会话
	cbReq, _ := http.NewRequest("GET", cbLoc, nil)
	r3, err := user.Do(cbReq)
	if err != nil {
		t.Fatal(err)
	}
	_ = r3.Body.Close()
	if r3.StatusCode != 302 {
		t.Fatalf("callback = %d", r3.StatusCode)
	}
	if tokens != 1 {
		t.Fatalf("token endpoint calls = %d, want 1", tokens)
	}

	// 会话已建立，用户名由邮箱推断（Alice.Smith@gmail.com → alice-smith）
	me, _ := http.NewRequest("GET", hs.URL+"/api/me", nil)
	r4, err := user.Do(me)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r4.Body.Close() }()
	var who struct {
		Username string `json:"username"`
	}
	if err := json.NewDecoder(r4.Body).Decode(&who); err != nil {
		t.Fatal(err)
	}
	if who.Username != "alice-smith" {
		t.Fatalf("me = %+v", who)
	}

	// 再次登录同 sub：复用同一账号（不新建）
	start2, _ := http.NewRequest("GET", hs.URL+"/api/auth/google", nil)
	r5, err := user.Do(start2)
	if err != nil {
		t.Fatal(err)
	}
	_ = r5.Body.Close()
	auth2, _ := http.NewRequest("GET", r5.Header.Get("Location"), nil)
	r6, err := user.Do(auth2)
	if err != nil {
		t.Fatal(err)
	}
	_ = r6.Body.Close()
	cb2, _ := http.NewRequest("GET", r6.Header.Get("Location"), nil)
	r7, err := user.Do(cb2)
	if err != nil {
		t.Fatal(err)
	}
	_ = r7.Body.Close()
	if r7.StatusCode != 302 {
		t.Fatalf("relogin callback = %d", r7.StatusCode)
	}
}
