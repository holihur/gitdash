package tests

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"gitdash/backend/internal/store"
)

// TestFeedbackIssueCreation 验证反馈开关：开启后 POST /api/feedback 会在管理员配置的
// 远端仓库（GitHub / Gitea 兼容 API）创建 Issue。
func TestFeedbackIssueCreation(t *testing.T) {
	var gotAuth, gotPath string
	var gotBody map[string]any
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"number":42,"html_url":"` + r.Host + `/acme/feedback/issues/42"}`))
	}))
	defer remote.Close()

	hs, st := startAPISeed(t, nil)
	_ = st.SetSetting("feedback_enabled", "1")
	_ = st.SetSetting("feedback_repo", remote.URL+"/acme/feedback")
	_ = st.SetSetting("feedback_token", "secret-token")

	// 公开状态：已启用。
	resp, err := http.Get(hs.URL + "/api/feedback")
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&cfg)
	_ = resp.Body.Close()
	if cfg["enabled"] != true {
		t.Fatalf("feedback config = %v, want enabled", cfg)
	}

	// 未登录提交（匿名允许）。
	body := strings.NewReader(`{"body":"Something is broken","url":"https://gitdash.example/repo/a/b"}`)
	resp, err = http.Post(hs.URL+"/api/feedback", "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("submit status = %d, body = %v", resp.StatusCode, out)
	}
	if gotPath != "/api/v1/repos/acme/feedback/issues" {
		t.Fatalf("remote path = %q", gotPath)
	}
	if gotAuth != "token secret-token" {
		t.Fatalf("remote auth = %q", gotAuth)
	}
	if gotBody["title"] != "Something is broken" {
		t.Fatalf("remote title = %v", gotBody["title"])
	}
	if !strings.Contains(gotBody["body"].(string), "https://gitdash.example/repo/a/b") {
		t.Fatalf("remote body missing page url: %v", gotBody["body"])
	}
	if !strings.Contains(gotBody["body"].(string), "- User: anonymous") {
		t.Fatalf("remote body missing reporter: %v", gotBody["body"])
	}
	if out["number"] != float64(42) {
		t.Fatalf("response number = %v", out["number"])
	}
}

// TestAdminFeedbackSettings 验证 admin 面板反馈配置的校验与只写令牌。
func TestAdminFeedbackSettings(t *testing.T) {
	hs, _ := startAPISeed(t, func(st *store.Store) {
		hash, _ := bcrypt.GenerateFromPassword([]byte("admin-pass-123"), bcrypt.DefaultCost)
		if err := st.CreateAdminUser("admin", string(hash)); err != nil {
			t.Fatalf("seed admin: %v", err)
		}
	})
	jar, _ := cookiejar.New(nil)
	admin := &http.Client{Jar: jar}

	post := func(body string) int {
		t.Helper()
		req, _ := http.NewRequest("POST", hs.URL+"/api/admin/settings", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		res, err := admin.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = res.Body.Close()
		return res.StatusCode
	}

	login, _ := http.NewRequest("POST", hs.URL+"/api/admin/login", strings.NewReader(`{"username":"admin","password":"admin-pass-123"}`))
	login.Header.Set("Content-Type", "application/json")
	res, err := admin.Do(login)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("admin login = %d", res.StatusCode)
	}

	if code := post(`{"feedback_enabled":true,"feedback_repo":"not-a-url"}`); code != http.StatusBadRequest {
		t.Fatalf("invalid repo = %d, want 400", code)
	}
	if code := post(`{"feedback_enabled":true,"feedback_repo":"https://github.com/acme/feedback"}`); code != http.StatusBadRequest {
		t.Fatalf("missing token = %d, want 400", code)
	}
	if code := post(`{"feedback_enabled":true,"feedback_repo":"https://github.com/acme/feedback","feedback_token":"tok"}`); code != 200 {
		t.Fatalf("valid config = %d, want 200", code)
	}

	// 令牌只写不读，且配置生效。
	res, err = admin.Get(hs.URL + "/api/admin/settings")
	if err != nil {
		t.Fatal(err)
	}
	var s map[string]any
	_ = json.NewDecoder(res.Body).Decode(&s)
	_ = res.Body.Close()
	if s["feedback_has_token"] != true {
		t.Fatalf("feedback_has_token = %v", s["feedback_has_token"])
	}
	if _, leaked := s["feedback_token"]; leaked {
		t.Fatal("settings leaked feedback_token")
	}
}

// TestFeedbackDisabled 未启用 / 配置不完整时不可提交。
func TestFeedbackDisabled(t *testing.T) {
	hs, st := startAPISeed(t, nil)

	resp, _ := http.Get(hs.URL + "/api/feedback")
	var cfg map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&cfg)
	_ = resp.Body.Close()
	if cfg["enabled"] != false {
		t.Fatalf("feedback config = %v, want disabled", cfg)
	}

	// 开关开启但缺少仓库 / 令牌 → 仍视为未启用。
	_ = st.SetSetting("feedback_enabled", "1")
	resp, _ = http.Post(hs.URL+"/api/feedback", "application/json", strings.NewReader(`{"body":"hi"}`))
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("submit status = %d, want 404", resp.StatusCode)
	}
}

// TestFeedbackEmptyBody 空内容返回 400。
func TestFeedbackEmptyBody(t *testing.T) {
	hs, st := startAPISeed(t, nil)
	_ = st.SetSetting("feedback_enabled", "1")
	_ = st.SetSetting("feedback_repo", "https://github.com/acme/feedback")
	_ = st.SetSetting("feedback_token", "secret-token")

	resp, _ := http.Post(hs.URL+"/api/feedback", "application/json", strings.NewReader(`{"body":"   "}`))
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("submit status = %d, want 400", resp.StatusCode)
	}
}
