package tests

import (
	"encoding/json"
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
	"gitdash/backend/internal/gitsvc"
	"gitdash/backend/internal/jobs"
	"gitdash/backend/internal/queue"
	"gitdash/backend/internal/store"
)

// startConnTest 启动一个带任务队列的 HTTP 实例（无需 SSH），用于账号绑定与批量导入测试。
func startConnTest(t *testing.T) (*httptest.Server, *store.Store) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("GITDASH_DATA", dir)
	t.Setenv("GITDASH_PROFILE_REPO", "0")
	t.Setenv("GITDASH_SSRF_ALLOW_PRIVATE", "1")
	if err := gitsvc.Init(dir); err != nil {
		t.Fatalf("gitsvc init: %v", err)
	}
	st, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	a := api.New(st, "test")
	a.SetJobsManager(jobs.New(st, queue.NewMemory(64, 4)))
	hs := httptest.NewServer(a.Handler(""))
	t.Cleanup(hs.Close)
	return hs, st
}

type connClient struct {
	t     *testing.T
	base  string
	token string
}

func (c *connClient) do(method, path string, body string) (int, []byte, http.Header) {
	c.t.Helper()
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, c.base+"/api"+path, rd)
	if err != nil {
		c.t.Fatal(err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, raw, resp.Header
}

func (c *connClient) json(method, path, body string, want int) map[string]any {
	c.t.Helper()
	code, raw, _ := c.do(method, path, body)
	if code != want {
		c.t.Fatalf("%s %s = %d, want %d: %s", method, path, code, want, raw)
	}
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	return m
}

// TestGiteaConnectionAndBatchImport 覆盖：配置 Gitea → 绑定账号 → 列出仓库 → 批量导入 → 解绑。
func TestGiteaConnectionAndBatchImport(t *testing.T) {
	var mock *httptest.Server
	mock = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/login/oauth/access_token":
			_, _ = w.Write([]byte(`{"access_token":"gitea-tok","scope":"repo"}`))
		case "/api/v1/user":
			_, _ = w.Write([]byte(`{"id":7,"login":"alice","avatar_url":"https://example/avatar.png"}`))
		case "/api/v1/user/repos":
			_, _ = w.Write([]byte(`[
				{"full_name":"alice/repo1","name":"repo1","private":false,"clone_url":"` + mock.URL + `/alice/repo1.git","default_branch":"main","description":"first"},
				{"full_name":"alice/repo2","name":"repo2","private":true,"clone_url":"` + mock.URL + `/alice/repo2.git","default_branch":"main","description":"second"}
			]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer mock.Close()

	hs, st := startConnTest(t)
	_ = st.SetSetting("gitea_enabled", "1")
	_ = st.SetSetting("gitea_base_url", mock.URL)
	_ = st.SetSetting("gitea_client_id", "cid")
	_ = st.SetSetting("gitea_client_secret", "csecret")

	c := &connClient{t: t, base: hs.URL}
	reg := c.json("POST", "/auth/register", `{"username":"alice","password":"password-123456"}`, 201)
	c.token, _ = reg["token"].(string)

	// 列出连接：gitea 已启用且未绑定。
	code, raw, _ := c.do("GET", "/connections", "")
	if code != 200 {
		t.Fatalf("list connections = %d: %s", code, raw)
	}
	var conns []map[string]any
	_ = json.Unmarshal(raw, &conns)
	var gitea map[string]any
	for _, v := range conns {
		if v["provider"] == "gitea" {
			gitea = v
		}
	}
	if gitea == nil || gitea["enabled"] != true || gitea["connected"] != false {
		t.Fatalf("gitea connection = %v", gitea)
	}

	// 发起绑定：从重定向 URL 中取出 state。
	code, _, hdr := c.do("GET", "/connections/gitea/start", "")
	if code != http.StatusFound {
		t.Fatalf("connect start = %d", code)
	}
	loc, err := url.Parse(hdr.Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	state := loc.Query().Get("state")
	if state == "" || !strings.HasPrefix(hdr.Get("Location"), mock.URL+"/login/oauth/authorize") {
		t.Fatalf("authorize redirect = %s", hdr.Get("Location"))
	}

	// 回调（state 已绑定用户，无需携带会话）。
	code, _, hdr = c.do("GET", "/connections/gitea/callback?code=abc&state="+url.QueryEscape(state), "")
	if code != http.StatusFound || !strings.Contains(hdr.Get("Location"), "connected=gitea") {
		t.Fatalf("callback = %d %s", code, hdr.Get("Location"))
	}

	// 已绑定。
	code, raw, _ = c.do("GET", "/connections", "")
	if code != 200 {
		t.Fatalf("list connections after = %d", code)
	}
	_ = json.Unmarshal(raw, &conns)
	for _, v := range conns {
		if v["provider"] == "gitea" {
			if v["connected"] != true || v["login"] != "alice" {
				t.Fatalf("gitea after connect = %v", v)
			}
		}
	}

	// 列出远程仓库。
	code, raw, _ = c.do("GET", "/connections/gitea/repos", "")
	if code != 200 {
		t.Fatalf("list repos = %d: %s", code, raw)
	}
	var repos []map[string]any
	_ = json.Unmarshal(raw, &repos)
	if len(repos) != 2 {
		t.Fatalf("repos = %v", repos)
	}

	// 批量导入两个仓库。
	code, raw, _ = c.do("POST", "/imports/batch", `{"provider":"gitea","repos":["alice/repo1","alice/repo2"]}`)
	if code != http.StatusAccepted {
		t.Fatalf("batch import = %d: %s", code, raw)
	}
	var res struct {
		Imported []string          `json:"imported"`
		Skipped  map[string]string `json:"skipped"`
	}
	_ = json.Unmarshal(raw, &res)
	if len(res.Imported) != 2 || len(res.Skipped) != 0 {
		t.Fatalf("batch result = %+v", res)
	}
	for _, name := range []string{"repo1", "repo2"} {
		if _, err := st.GetRepo("alice", name); err != nil {
			t.Fatalf("repo %s not created: %v", name, err)
		}
	}

	// 重复导入应跳过。
	code, raw, _ = c.do("POST", "/imports/batch", `{"provider":"gitea","repos":["alice/repo1"]}`)
	if code != http.StatusAccepted {
		t.Fatalf("re-batch = %d", code)
	}
	_ = json.Unmarshal(raw, &res)
	if len(res.Imported) != 0 || res.Skipped["alice/repo1"] == "" {
		t.Fatalf("re-batch result = %+v", res)
	}

	// 解绑。
	code, _, _ = c.do("DELETE", "/connections/gitea", "")
	if code != http.StatusNoContent {
		t.Fatalf("disconnect = %d", code)
	}
	// 未绑定后列仓库 → 404。
	code, _, _ = c.do("GET", "/connections/gitea/repos", "")
	if code != http.StatusNotFound {
		t.Fatalf("list repos after disconnect = %d", code)
	}
}

// TestAdminBindingSettings 验证 admin 面板对 GitLab/Gitea/Bitbucket 绑定配置的保存与读取。
func TestAdminBindingSettings(t *testing.T) {
	hs, st := startConnTest(t)
	hash, _ := bcrypt.GenerateFromPassword([]byte("admin-pass-123"), bcrypt.DefaultCost)
	if err := st.CreateAdminUser("admin", string(hash)); err != nil {
		t.Fatalf("seed admin: %v", err)
	}
	jar, _ := cookiejar.New(nil)
	admin := &http.Client{Jar: jar}
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

	save, _ := http.NewRequest("POST", hs.URL+"/api/admin/settings", strings.NewReader(`{
		"gitlab_enabled":true,"gitlab_base_url":"https://gitlab.example.com","gitlab_client_id":"glid","gitlab_client_secret":"glsecret",
		"gitea_enabled":true,"gitea_base_url":"https://gitea.example.com","gitea_client_id":"gtid","gitea_client_secret":"gtsecret",
		"bitbucket_enabled":true,"bitbucket_client_id":"bbid","bitbucket_client_secret":"bbsecret"
	}`))
	save.Header.Set("Content-Type", "application/json")
	sres, err := admin.Do(save)
	if err != nil {
		t.Fatal(err)
	}
	_ = sres.Body.Close()
	if sres.StatusCode != 200 {
		t.Fatalf("save settings = %d", sres.StatusCode)
	}

	gres, _ := admin.Get(hs.URL + "/api/admin/settings")
	var s map[string]any
	_ = json.NewDecoder(gres.Body).Decode(&s)
	_ = gres.Body.Close()
	for _, k := range []string{"gitlab_enabled", "gitea_enabled", "bitbucket_enabled"} {
		if s[k] != true {
			t.Fatalf("%s = %v, want true", k, s[k])
		}
	}
	if s["gitlab_base_url"] != "https://gitlab.example.com" || s["gitea_base_url"] != "https://gitea.example.com" {
		t.Fatalf("base urls = %v / %v", s["gitlab_base_url"], s["gitea_base_url"])
	}
	for _, k := range []string{"gitlab_has_secret", "gitea_has_secret", "bitbucket_has_secret"} {
		if s[k] != true {
			t.Fatalf("%s = %v, want true", k, s[k])
		}
	}
	if _, leaked := s["gitlab_client_secret"]; leaked {
		t.Fatal("settings leaked gitlab_client_secret")
	}
}

// TestInstanceDocsURL 验证 /api/instance 的文档地址：环境变量优先于管理端设置。
func TestInstanceDocsURL(t *testing.T) {
	hs, st := startConnTest(t)

	get := func() string {
		t.Helper()
		res, err := http.Get(hs.URL + "/api/instance")
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = res.Body.Close() }()
		var m map[string]any
		_ = json.NewDecoder(res.Body).Decode(&m)
		s, _ := m["docs_url"].(string)
		return s
	}

	// 默认未配置。
	if got := get(); got != "" {
		t.Fatalf("docs_url = %q, want empty", got)
	}

	// 仅设置 admin docs_url。
	if err := st.SetSetting("docs_url", "https://docs.example.com/gitdash/"); err != nil {
		t.Fatal(err)
	}
	if got := get(); got != "https://docs.example.com/gitdash" {
		t.Fatalf("docs_url = %q, want trimmed setting", got)
	}

	// 环境变量优先。
	t.Setenv("GITDASH_DOCS_URL", "https://env.example.com/docs/")
	if got := get(); got != "https://env.example.com/docs" {
		t.Fatalf("docs_url = %q, want env override", got)
	}
}

// TestConnectDisabledProvider 未启用的 provider 不可绑定。
func TestConnectDisabledProvider(t *testing.T) {
	hs, _ := startConnTest(t)
	c := &connClient{t: t, base: hs.URL}
	reg := c.json("POST", "/auth/register", `{"username":"bobby","password":"password-123456"}`, 201)
	c.token, _ = reg["token"].(string)
	code, _, _ := c.do("GET", "/connections/gitlab/start", "")
	if code != http.StatusNotFound {
		t.Fatalf("disabled provider start = %d", code)
	}
}
