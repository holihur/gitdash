package tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func whPath(owner, repo, suffix string) string {
	return fmt.Sprintf("/users/%s/repos/%s/webhooks%s", owner, repo, suffix)
}

func listWebhooks(t *testing.T, c *Client, owner, repo string) []Webhook {
	t.Helper()
	req, err := http.NewRequest("GET", c.env.BaseURL+"/api"+whPath(owner, repo, ""), nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("list webhooks: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		t.Fatalf("list webhooks: status %d", resp.StatusCode)
	}
	var out []Webhook
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return out
}

type Webhook struct {
	ID    int64  `json:"id"`
	Owner string `json:"owner"`
	Repo  string `json:"repo"`
	URL   string `json:"url"`
}

func TestWebhookCRUD(t *testing.T) {
	env := start(t)
	alice := register(t, env, "alice", "alice-pass-123")
	bob := register(t, env, "bob", "bob-pass-123456")
	alice.mustStatus("POST", "/repos", map[string]string{"name": "demo"}, 201)

	// 创建
	m := alice.mustStatus("POST", whPath("alice", "demo", ""),
		map[string]string{"url": "https://example.com/hook"}, 201)
	if m["url"] != "https://example.com/hook" {
		t.Fatalf("create webhook = %v", m)
	}

	// 重复创建同 URL
	alice.mustFail("POST", whPath("alice", "demo", ""),
		map[string]string{"url": "https://example.com/hook"}, 409)

	// 列表
	whs := listWebhooks(t, alice, "alice", "demo")
	if len(whs) != 1 || whs[0].URL != "https://example.com/hook" {
		t.Fatalf("webhooks = %+v", whs)
	}

	// 非法 URL
	for _, bad := range []string{"", "ftp://x", "not a url", "javascript:alert(1)"} {
		alice.mustFail("POST", whPath("alice", "demo", ""), map[string]string{"url": bad}, 400)
	}

	// 删除
	alice.mustStatus("DELETE", whPath("alice", "demo", fmt.Sprintf("/%d", whs[0].ID)), nil, 204)
	alice.mustFail("DELETE", whPath("alice", "demo", fmt.Sprintf("/%d", whs[0].ID)), nil, 404)

	// 非 owner 不能管理；协作者也不能
	bob.mustFail("GET", whPath("alice", "demo", ""), nil, 404)
	bob.mustFail("POST", whPath("alice", "demo", ""), map[string]string{"url": "https://x"}, 404)
	alice.mustStatus("POST", "/users/alice/repos/demo/collabs",
		map[string]string{"username": "bob", "permission": "write"}, 200)
	bob.mustFail("GET", whPath("alice", "demo", ""), nil, 404)

	// 仓库不存在
	alice.mustFail("GET", whPath("alice", "nope", ""), nil, 404)
}

// TestPushSpoolsHookEvent 用真实 SSH push 验证 post-receive hook 落盘事件（含 pusher）。
func TestPushSpoolsHookEvent(t *testing.T) {
	requireBins(t, "git", "ssh", "ssh-keygen")
	env := start(t)
	alice := register(t, env, "alice", "alice-pass-123")
	alice.mustStatus("POST", "/repos", map[string]string{"name": "hooks"}, 201)

	key, pub := genKey(t, env.DataDir, "alice_key")
	alice.mustStatus("POST", "/keys", map[string]string{"name": "e2e", "public_key": pub}, 201)

	work := t.TempDir()
	runCmd(t, work, sshEnv(key), "git", "clone", "-q",
		fmt.Sprintf("ssh://git@127.0.0.1:%s/hooks.git", env.SSHPort), "hooks")
	repo := filepath.Join(work, "hooks")
	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	runCmd(t, repo, sshEnv(key), "git", "add", "-A")
	runCmd(t, repo, sshEnv(key), "git", "-c", "user.name=alice", "-c", "user.email=a@b.c",
		"commit", "-q", "-m", "hook commit")
	runCmd(t, repo, sshEnv(key), "git", "push", "-q", "origin", "HEAD")

	// 读取 spool
	spool := filepath.Join(env.DataDir, "webhook-events")
	matches, err := filepath.Glob(filepath.Join(spool, "alice__hooks-*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) == 0 {
		t.Fatalf("no spooled push event in %s", spool)
	}
	raw, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatal(err)
	}
	var ev map[string]string
	if err := json.Unmarshal(raw, &ev); err != nil {
		t.Fatalf("bad spool json %q: %v", raw, err)
	}
	if ev["event"] != "push" || ev["owner"] != "alice" || ev["repo"] != "hooks" {
		t.Fatalf("spool event = %v", ev)
	}
	if ev["user"] != "alice" {
		t.Fatalf("pusher missing in event: %v", ev)
	}
	if !strings.HasPrefix(ev["ref"], "refs/heads/") || ev["new"] == "" || len(ev["new"]) != 40 {
		t.Fatalf("spool event refs = %v", ev)
	}
}
