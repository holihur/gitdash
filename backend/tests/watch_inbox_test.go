package tests

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// watch & inbox：关注仓库 + 收件箱通知。

type Notif struct {
	ID     int64  `json:"id"`
	Kind   string `json:"kind"`
	Action string `json:"action"`
	Owner  string `json:"owner"`
	Repo   string `json:"repo"`
	Number int64  `json:"number"`
	Title  string `json:"title"`
	Actor  string `json:"actor"`
	Read   bool   `json:"read"`
}

func unreadCount(t *testing.T, c *Client) int {
	t.Helper()
	m := c.mustStatus("GET", "/inbox/unread", nil, 200)
	n, _ := m["count"].(float64)
	return int(n)
}

func actionsOf(items []Notif) []string {
	out := make([]string, 0, len(items))
	for _, n := range items {
		out = append(out, n.Action)
	}
	return out
}

func TestWatchLifecycle(t *testing.T) {
	env := start(t)
	alice := register(t, env, "alice", "alice-pass-123")
	bob := register(t, env, "bob", "bob-pass-123456")

	// 新建仓库：创建者自动 watch
	alice.mustStatus("POST", "/repos", map[string]string{"name": "demo"}, 201)
	r := alice.mustStatus("GET", "/users/alice/repos/demo", nil, 200)
	if r["watchers"] != float64(1) || r["watching"] != true {
		t.Fatalf("owner auto watch = %v", r)
	}

	// 私有仓库：他人不可 watch
	alice.mustStatus("POST", "/repos", map[string]string{"name": "secret"}, 201)
	bob.mustFail("PUT", "/users/alice/repos/secret/watch", nil, 404)
	bob.mustFail("DELETE", "/users/alice/repos/secret/watch", nil, 404)

	// 公开后 bob 可 watch
	alice.mustStatus("POST", "/users/alice/repos/demo/visibility", map[string]bool{"private": false}, 200)
	w := bob.mustStatus("PUT", "/users/alice/repos/demo/watch", nil, 200)
	if w["watching"] != true || w["watchers"] != float64(2) {
		t.Fatalf("bob watch = %v", w)
	}
	// 幂等
	w = bob.mustStatus("PUT", "/users/alice/repos/demo/watch", nil, 200)
	if w["watchers"] != float64(2) {
		t.Fatalf("idempotent watch = %v", w)
	}
	// 仓库详情 / 列表携带 watching
	r = bob.mustStatus("GET", "/users/alice/repos/demo", nil, 200)
	if r["watching"] != true || r["watchers"] != float64(2) {
		t.Fatalf("repo view = %v", r)
	}
	watched := getJSON[[]Repo](t, bob, "/watched", 200)
	if len(watched) != 1 || watched[0].Name != "demo" {
		t.Fatalf("watched list = %+v", watched)
	}

	// 取消关注
	w = bob.mustStatus("DELETE", "/users/alice/repos/demo/watch", nil, 200)
	if w["watching"] != false || w["watchers"] != float64(1) {
		t.Fatalf("unwatch = %v", w)
	}
	watched = getJSON[[]Repo](t, bob, "/watched", 200)
	if len(watched) != 0 {
		t.Fatalf("watched after unwatch = %+v", watched)
	}

	// 未登录不可 watch / 看列表
	anon := &Client{env: env}
	anon.mustFail("PUT", "/users/alice/repos/demo/watch", nil, 401)
	anon.mustFail("GET", "/watched", nil, 401)
}

func TestInboxIssueNotifications(t *testing.T) {
	env := start(t)
	alice := register(t, env, "alice", "alice-pass-123")
	bob := register(t, env, "bob", "bob-pass-123456")

	alice.mustStatus("POST", "/repos", map[string]string{"name": "demo"}, 201)
	alice.mustStatus("POST", "/users/alice/repos/demo/visibility", map[string]bool{"private": false}, 200)
	bob.mustStatus("PUT", "/users/alice/repos/demo/watch", nil, 200)
	// bob 获得写权限以便开 issue / 改状态
	alice.mustStatus("POST", "/users/alice/repos/demo/collabs",
		map[string]string{"username": "bob", "permission": "write"}, 200)

	// alice 开 issue → 通知 watcher bob（actor alice 不通知自己）
	alice.mustStatus("POST", "/repos/demo/issues", map[string]string{"title": "t1"}, 201)
	bobItems := getJSON[[]Notif](t, bob, "/inbox", 200)
	if len(bobItems) != 1 || bobItems[0].Kind != "issue" || bobItems[0].Action != "opened" ||
		bobItems[0].Number != 1 || bobItems[0].Actor != "alice" || bobItems[0].Read {
		t.Fatalf("bob inbox = %+v", bobItems)
	}
	if unreadCount(t, bob) != 1 {
		t.Fatal("bob unread should be 1")
	}
	if got := getJSON[[]Notif](t, alice, "/inbox", 200); len(got) != 0 {
		t.Fatalf("alice (actor) should not be notified: %+v", got)
	}

	// alice 取消关注自己的仓库后仍收 owner 级通知（隐式订阅）
	alice.mustStatus("DELETE", "/users/alice/repos/demo/watch", nil, 200)

	// bob 开 issue → 通知 alice（owner，即使未 watch）
	bob.mustStatus("POST", "/users/alice/repos/demo/issues", map[string]string{"title": "t2"}, 201)
	aliceItems := getJSON[[]Notif](t, alice, "/inbox", 200)
	if len(aliceItems) != 1 || aliceItems[0].Action != "opened" || aliceItems[0].Actor != "bob" ||
		aliceItems[0].Number != 2 {
		t.Fatalf("alice inbox = %+v", aliceItems)
	}

	// alice 关闭 #2 → 通知 bob（closed）；alice 自己是 actor 不再收
	alice.mustStatus("PATCH", "/users/alice/repos/demo/issues/2", map[string]string{"state": "closed"}, 200)
	bobItems = getJSON[[]Notif](t, bob, "/inbox", 200)
	if len(bobItems) != 2 || bobItems[0].Action != "closed" || bobItems[0].Number != 2 {
		t.Fatalf("bob inbox after close = %+v", bobItems)
	}
	// 重复关闭（状态不变）不再通知
	alice.mustStatus("PATCH", "/users/alice/repos/demo/issues/2", map[string]string{"state": "closed"}, 200)
	if got := getJSON[[]Notif](t, bob, "/inbox", 200); len(got) != 2 {
		t.Fatalf("dup close should not notify: %+v", got)
	}

	// bob 重新打开 #2 → 通知 alice（reopened）
	bob.mustStatus("PATCH", "/users/alice/repos/demo/issues/2", map[string]string{"state": "open"}, 200)
	aliceItems = getJSON[[]Notif](t, alice, "/inbox", 200)
	if len(aliceItems) != 2 || aliceItems[0].Action != "reopened" || aliceItems[0].Actor != "bob" {
		t.Fatalf("alice inbox after reopen = %+v", aliceItems)
	}

	// 已读 / 未读 / 单条已读 / 全部已读 / 删除
	if unreadCount(t, alice) != 2 {
		t.Fatal("alice unread = 2 expected")
	}
	first := aliceItems[0]
	alice.mustStatus("POST", fmt.Sprintf("/inbox/read/%d", first.ID), nil, 200)
	if unreadCount(t, alice) != 1 {
		t.Fatal("alice unread = 1 after single read")
	}
	// 越权读他人通知 / 不存在通知
	bob.mustFail("POST", fmt.Sprintf("/inbox/read/%d", first.ID), nil, 404)
	alice.mustFail("POST", "/inbox/read/999999", nil, 404)

	alice.mustStatus("POST", "/inbox/read", nil, 200)
	if unreadCount(t, alice) != 0 {
		t.Fatal("alice unread = 0 after read all")
	}
	alice.mustStatus("DELETE", fmt.Sprintf("/inbox/%d", first.ID), nil, 204)
	if got := getJSON[[]Notif](t, alice, "/inbox", 200); len(got) != 1 {
		t.Fatalf("alice inbox after delete = %+v", got)
	}
	alice.mustFail("DELETE", "/inbox/999999", nil, 404)

	// 收件箱需登录
	anon := &Client{env: env}
	anon.mustFail("GET", "/inbox", nil, 401)
	anon.mustFail("POST", "/inbox/read", nil, 401)
	anon.mustFail("DELETE", fmt.Sprintf("/inbox/%d", first.ID), nil, 401)
}

// seedBranchPush 往已存在的 bare 仓库推送一条分支（file:// 协议，无需 SSH）。
func seedBranchPush(t *testing.T, env *Env, owner, name, branch, file, content string) {
	t.Helper()
	requireBins(t, "git")
	dir := t.TempDir()
	runCmd(t, dir, nil, "git", "clone", "-q", "file://"+repoOnDisk(env, owner, name), "w")
	w := filepath.Join(dir, "w")
	runCmd(t, w, nil, "git", "checkout", "-q", "-b", branch)
	if err := os.WriteFile(filepath.Join(w, file), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	runCmd(t, w, nil, "git", "add", "-A")
	runCmd(t, w, nil, "git", "-c", "user.name=alice", "-c", "user.email=alice@example.com", "commit", "-q", "-m", branch)
	runCmd(t, w, nil, "git", "push", "-q", "origin", branch)
}

func TestInboxPullNotifications(t *testing.T) {
	requireBins(t, "git")
	env := start(t)
	alice := register(t, env, "alice", "alice-pass-123")
	bob := register(t, env, "bob", "bob-pass-123456")

	alice.mustStatus("POST", "/repos", map[string]string{"name": "prd"}, 201)
	alice.mustStatus("POST", "/users/alice/repos/prd/visibility", map[string]bool{"private": false}, 200)
	bob.mustStatus("PUT", "/users/alice/repos/prd/watch", nil, 200)

	seedCommits(t, env, "alice", "prd", map[string]string{"main.txt": "m"})
	seedBranchPush(t, env, "alice", "prd", "feat", "feat.txt", "feature")

	// 创建 PR → 通知 bob（opened）
	alice.mustStatus("POST", prPath("alice", "prd", ""),
		map[string]string{"title": "add feat", "body": "b", "source_branch": "feat", "target_branch": "main"}, 201)
	items := getJSON[[]Notif](t, bob, "/inbox", 200)
	if len(items) != 1 || items[0].Kind != "pull" || items[0].Action != "opened" ||
		items[0].Number != 1 || items[0].Actor != "alice" {
		t.Fatalf("bob inbox after pr open = %+v", items)
	}

	// 关闭 → reopened → merged：完整状态流各通知一次
	alice.mustStatus("POST", prPath("alice", "prd", "/1/state"), map[string]string{"state": "closed"}, 200)
	alice.mustStatus("POST", prPath("alice", "prd", "/1/state"), map[string]string{"state": "open"}, 200)
	merged := alice.mustStatus("POST", prPath("alice", "prd", "/1/merge"), nil, 200)
	if merged["state"] != "merged" {
		t.Fatalf("merge = %v", merged)
	}
	items = getJSON[[]Notif](t, bob, "/inbox", 200)
	if len(items) != 4 {
		t.Fatalf("bob inbox = %+v (want 4)", items)
	}
	got := actionsOf(items)
	want := []string{"merged", "reopened", "closed", "opened"} // 新→旧
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("actions = %v, want %v", got, want)
		}
	}
	// actor（alice）不通知自己
	if got := getJSON[[]Notif](t, alice, "/inbox", 200); len(got) != 0 {
		t.Fatalf("alice should not be notified: %+v", got)
	}
}
