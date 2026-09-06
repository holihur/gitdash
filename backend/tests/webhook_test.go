package tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gitdash/backend/internal/webhooks"
)

// listDeliveries 拉取投递记录数组（do 只解 map，这里单独解数组）。
func listDeliveries(t *testing.T, c *Client, owner, repo string, hookID int64) []map[string]any {
	t.Helper()
	req, err := http.NewRequest("GET", c.env.BaseURL+"/api"+whPath(owner, repo, fmt.Sprintf("/%d/deliveries", hookID)), nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("list deliveries: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	var out []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode deliveries: %v", err)
	}
	return out
}

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

// TestWebhookDeliveries 验证投递记录 + 失败重试：直接向 spool 写事件，
// 服务器侧 webhooks.Run 会投递并落 webhook_deliveries 记录（成功 / 失败均可查）。
func TestWebhookDeliveries(t *testing.T) {
	env := start(t)
	alice := register(t, env, "alice", "alice-pass-123")
	alice.mustStatus("POST", "/repos", map[string]string{"name": "deliv"}, 201)

	// 成功端点：本地回环 http 监听
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	m1 := alice.mustStatus("POST", whPath("alice", "deliv", ""),
		map[string]string{"url": srv.URL + "/hook"}, 201)
	hook1 := int64(m1["id"].(float64))
	m2 := alice.mustStatus("POST", whPath("alice", "deliv", ""),
		map[string]string{"url": "http://127.0.0.1:1/unreachable"}, 201)
	hook2 := int64(m2["id"].(float64))

	// harness 不经 main.go，需手动启动 spool 消费循环（与生产同路径）
	go webhooks.Run(filepath.Join(env.DataDir, "webhook-events"), env.Store, 200*time.Millisecond)

	// 投一个 push 事件（模拟 post-receive / API spool）
	ev := map[string]string{
		"event": "push", "owner": "alice", "repo": "deliv",
		"old": strings.Repeat("0", 40), "new": strings.Repeat("a", 40),
		"ref": "refs/heads/main", "user": "alice",
		"created_at": "2026-01-01T00:00:00Z",
	}
	b, err := json.Marshal(ev)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(env.DataDir, "webhook-events", "alice__deliv-test.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}

	// 成功记录出现（drain 间隔 2s，多轮轮询）
	deadline := time.Now().Add(20 * time.Second)
	var dl []map[string]any
	for {
		dl = listDeliveries(t, alice, "alice", "deliv", hook1)
		if len(dl) > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("no delivery record for hook %d", hook1)
		}
		time.Sleep(500 * time.Millisecond)
	}
	if dl[0]["status"] != "success" {
		t.Fatalf("delivery = %v", dl[0])
	}
	if atomic.LoadInt32(&hits) == 0 {
		t.Fatal("endpoint not hit")
	}

	// 失败记录出现（不可达端口）
	deadline = time.Now().Add(20 * time.Second)
	for {
		dl = listDeliveries(t, alice, "alice", "deliv", hook2)
		if len(dl) > 0 && dl[0]["status"] != "success" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("no failed delivery record for hook %d: %v", hook2, dl)
		}
		time.Sleep(500 * time.Millisecond)
	}

	// 非 owner 查不到
	bob := register(t, env, "bob2", "bob-pass-123456")
	bob.mustFail("GET", whPath("alice", "deliv", fmt.Sprintf("/%d/deliveries", hook1)), nil, 404)
}
