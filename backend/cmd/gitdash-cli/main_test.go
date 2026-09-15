package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestExtractGlobals(t *testing.T) {
	rest, host, token, jsonOut := extractGlobals([]string{
		"repo", "list", "--host", "http://x", "--token=tok", "--json",
	})
	if host != "http://x" || token != "tok" || !jsonOut {
		t.Fatalf("host=%q token=%q json=%v", host, token, jsonOut)
	}
	if strings.Join(rest, " ") != "repo list" {
		t.Fatalf("rest = %v", rest)
	}
}

func TestSplitRepo(t *testing.T) {
	owner, repo, err := splitRepo("alice/demo")
	if err != nil || owner != "alice" || repo != "demo" {
		t.Fatalf("got %q %q %v", owner, repo, err)
	}
	if _, _, err := splitRepo("bad"); err == nil {
		t.Fatal("expected error for missing slash")
	}
}

func TestResolveClientRequiresHostAndToken(t *testing.T) {
	t.Setenv("GITDASH_CONFIG_DIR", t.TempDir())
	t.Setenv("GITDASH_HOST", "")
	t.Setenv("GITDASH_TOKEN", "")
	if _, err := resolveClient("", ""); err == nil {
		t.Fatal("expected error without host")
	}
	if _, err := resolveClient("http://x", ""); err == nil {
		t.Fatal("expected error without token")
	}
}

func TestRunCommandsAgainstMockServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/me":
			_ = json.NewEncoder(w).Encode(map[string]any{"username": "alice", "email": "a@b.c", "created_at": "2020-01-01"})
		case "/api/repos":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"owner": "alice", "name": "demo", "private": true, "description": "d"},
			})
		case "/api/users/alice/repos/demo/issues":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"number": 1, "title": "Bug", "state": "open", "author": "alice"},
			})
		case "/api/users/alice/repos/demo/pulls":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"number": 2, "title": "Fix", "state": "open", "source_branch": "f", "target_branch": "main"},
			})
		case "/api/me/byok":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"id": 5, "name": "work", "provider": "anthropic", "key_set": true},
			})
		case "/api/users/alice/repos/demo/copilots":
			if r.Method == http.MethodPost {
				w.WriteHeader(http.StatusCreated)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"id": 3, "byok_id": 5, "issue_number": 7, "prompt": "fix issue #7", "branch": "copilot/session-3",
				})
				return
			}
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"id": 3, "status": "idle", "issue_number": 7, "branch": "copilot/session-3"},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
		}
	}))
	defer srv.Close()

	t.Setenv("GITDASH_CONFIG_DIR", t.TempDir())
	t.Setenv("GITDASH_HOST", srv.URL)
	t.Setenv("GITDASH_TOKEN", "test-token")

	cases := [][]string{
		{"me"},
		{"repo", "list"},
		{"issue", "list", "alice/demo"},
		{"copilot", "list", "alice/demo"},
		{"copilot", "create", "alice/demo", "--issue", "7"},
		{"copilot", "fix", "alice/demo", "7", "--detach"},
		{"issue", "fix", "alice/demo", "7", "--detach"},
		{"pr", "list", "alice/demo"},
		{"--host", srv.URL, "--token", "x", "me"},
		{"version"},
	}
	for _, c := range cases {
		if err := run(c); err != nil {
			t.Fatalf("run(%v) = %v", c, err)
		}
	}

	// 参数校验错误
	for _, c := range [][]string{
		{"issue", "create", "alice/demo"},              // 缺 --title
		{"pr", "create", "alice/demo", "--title", "x"}, // 缺 --head/--base
		{"repo"},                              // 缺子命令
		{"copilot"},                           // 缺子命令
		{"copilot", "run", "alice/demo", "0"}, // 非法会话 id
		{"copilot", "fix", "alice/demo"},      // 缺 issue 编号
		{"copilot", "create", "alice/demo", "--byok", "nope"}, // 未知 byok
		{"unknowncmd"},           // 未知命令
		{"issue", "list", "bad"}, // owner/repo 格式错
	} {
		if err := run(c); err == nil {
			t.Fatalf("run(%v) expected error", c)
		}
	}
}

func TestSkillEmbedded(t *testing.T) {
	if !strings.Contains(skillContent, "name: gitdash-cli") {
		t.Fatal("embedded skill missing frontmatter name")
	}
	if !strings.Contains(skillContent, "description:") {
		t.Fatal("embedded skill missing description")
	}
	if err := run([]string{"skill", "show"}); err != nil {
		t.Fatalf("skill show: %v", err)
	}
	if err := run([]string{"skill"}); err != nil {
		t.Fatalf("skill (default show): %v", err)
	}
}

func TestSkillInstallToDir(t *testing.T) {
	dir := t.TempDir()
	if err := run([]string{"skill", "install", "--dir", dir}); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "gitdash-cli", "SKILL.md")
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("expected skill at %s: %v", p, err)
	}
	if !strings.Contains(string(b), "name: gitdash-cli") {
		t.Fatal("installed SKILL.md missing frontmatter")
	}
}

func TestSkillInstallProjectTargets(t *testing.T) {
	cwd := t.TempDir()
	t.Chdir(cwd)
	if err := run([]string{"skill", "install", "--project"}); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{
		filepath.Join(cwd, ".claude", "skills", "gitdash-cli", "SKILL.md"),
		filepath.Join(cwd, ".agents", "skills", "gitdash-cli", "SKILL.md"),
	} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("missing %s: %v", p, err)
		}
	}
}

func TestSkillInstallUnknownTarget(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // 避免误写真实 home
	if err := run([]string{"skill", "install", "--target", "nope"}); err == nil {
		t.Fatal("expected error for unknown target")
	}
}

func TestDeviceLoginFlow(t *testing.T) {
	// 第一轮轮询返回 authorization_pending，第二轮返回 token。
	polls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/login/oauth/device/code":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"device_code": "dc", "user_code": "ABCD-EFGH",
				"verification_uri":          "http://example/device",
				"verification_uri_complete": "http://example/device?user_code=ABCD-EFGH",
				"expires_in":                60, "interval": 1,
			})
		case "/login/oauth/access_token":
			polls++
			if polls < 2 {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "pending", "code": "authorization_pending"})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "at-123", "token_type": "bearer", "scope": "repo"})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	token, err := deviceLogin(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if token != "at-123" {
		t.Fatalf("token = %q", token)
	}
	if polls < 2 {
		t.Fatalf("expected at least 2 polls, got %d", polls)
	}
}

func TestPermuteFlags(t *testing.T) {
	got := permuteFlags([]string{"7", "--detach", "--byok", "work"}, map[string]bool{"--byok": true})
	want := []string{"--detach", "--byok", "work", "7"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("permuteFlags = %v, want %v", got, want)
	}
}

func TestRunCopilotTurnStreamsToDone(t *testing.T) {
	detailHit := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/chat") {
			if r.Header.Get("Authorization") != "Bearer tok" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			conn, err := websocket.Accept(w, r, nil)
			if err != nil {
				return
			}
			defer func() { _ = conn.Close(websocket.StatusNormalClosure, "") }()
			ctx := r.Context()
			_, data, err := conn.Read(ctx)
			if err != nil {
				return
			}
			var in map[string]string
			_ = json.Unmarshal(data, &in)
			if in["text"] != "hello" {
				return
			}
			_ = conn.Write(ctx, websocket.MessageText, []byte(`{"type":"tool_start","name":"write"}`))
			_ = conn.Write(ctx, websocket.MessageText, []byte(`{"type":"done","text":"done"}`))
			return
		}
		detailHit = true
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": 1, "status": "idle", "pr_number": 9, "branch": "copilot/session-1",
		})
	}))
	defer srv.Close()

	cl := &client{host: srv.URL, token: "tok"}
	if err := runCopilotTurn(cl, "alice", "demo", 1, "hello", 5*time.Second, false); err != nil {
		t.Fatalf("runCopilotTurn: %v", err)
	}
	if !detailHit {
		t.Fatal("expected the session detail to be refetched after done")
	}
}
