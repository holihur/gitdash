package runner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"gitdash/backend/internal/store"
)

// sha256Hex 与服务端 runnerHash 一致，用于测试假 runner 的反向认证。
func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// TestReverseDialConnectsAndDispatches：服务端按 runner.URL 主动拨号，
// 用 `Bearer name:sha256(secret)` 认证；连上后经 Redis 派发的 job 能到达 runner。
func TestReverseDialConnectsAndDispatches(t *testing.T) {
	t.Setenv("GITDASH_SSRF_ALLOW_PRIVATE", "1") // httptest 绑定 127.0.0.1
	dir := t.TempDir()
	st, err := store.Open(dir + "/test.db")
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	rdb := startTestRedis(t)
	h := NewHub(st, rdb)
	defer h.Stop()

	const name, secret = "rev-1", "sec-rev"
	want := sha256Hex(secret)

	jobs := make(chan Job, 4)
	authed := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n, v, ok := BearerCredentials(r)
		if !ok || n != name || v != want {
			http.Error(w, "invalid credentials", http.StatusUnauthorized)
			return
		}
		ws, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = ws.Close(websocket.StatusNormalClosure, "") }()
		select {
		case authed <- struct{}{}:
		default:
		}
		ctx := r.Context()
		for {
			_, data, err := ws.Read(ctx)
			if err != nil {
				return
			}
			var msg Message
			if json.Unmarshal(data, &msg) != nil {
				continue
			}
			if msg.Type == TypeJob {
				var j Job
				if json.Unmarshal(msg.Payload, &j) == nil {
					select {
					case jobs <- j:
					default:
					}
				}
			}
		}
	}))
	defer srv.Close()
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")

	if _, err := st.CreateRunner(name, secret, "docker", "", ModeReverse, wsURL); err != nil {
		t.Fatalf("create runner: %v", err)
	}

	// reverseWatcher 在 NewHub 中启动，等它抢锁并拨号
	select {
	case <-authed:
	case <-time.After(20 * time.Second):
		t.Fatal("reverse dial never authenticated")
	}

	deadline := time.Now().Add(5 * time.Second)
	for !h.IsOnline(context.Background(), name) {
		if time.Now().After(deadline) {
			t.Fatal("runner should be online after reverse connect")
		}
		time.Sleep(50 * time.Millisecond)
	}

	// 派发可能早于服务端 Redis 订阅就绪，重试直至投递成功
	job := Job{JobID: "j-rev", RunID: 9, Owner: "a", Repo: "b", SHA: "deadbeef", Ref: "main", DSL: "image: alpine"}
	deadline = time.Now().Add(5 * time.Second)
	for {
		if err := h.Dispatch(name, Message{Type: TypeJob, Payload: mustJSON(job)}); err != nil {
			t.Fatalf("dispatch: %v", err)
		}
		select {
		case got := <-jobs:
			if got.JobID != "j-rev" || got.RunID != 9 {
				t.Fatalf("job payload: %+v", got)
			}
			return
		case <-time.After(200 * time.Millisecond):
		}
		if time.Now().After(deadline) {
			t.Fatal("job not delivered to reverse runner")
		}
	}
}

// TestReverseDialFollowsURLChange：删除后以同名新 url 重新注册时，拨号循环应切换到新地址。
func TestReverseDialFollowsURLChange(t *testing.T) {
	t.Setenv("GITDASH_SSRF_ALLOW_PRIVATE", "1")
	dir := t.TempDir()
	st, err := store.Open(dir + "/test.db")
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	rdb := startTestRedis(t)
	h := NewHub(st, rdb)
	defer h.Stop()

	const name, secret = "rev-url", "sec-url"
	newFake := func() (*httptest.Server, chan struct{}) {
		got := make(chan struct{}, 1)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			n, v, ok := BearerCredentials(r)
			if !ok || n != name || v != sha256Hex(secret) {
				http.Error(w, "invalid credentials", http.StatusUnauthorized)
				return
			}
			ws, err := websocket.Accept(w, r, nil)
			if err != nil {
				return
			}
			defer func() { _ = ws.Close(websocket.StatusNormalClosure, "") }()
			select {
			case got <- struct{}{}:
			default:
			}
			for {
				if _, _, err := ws.Read(r.Context()); err != nil {
					return
				}
			}
		}))
		return srv, got
	}

	srv1, got1 := newFake()
	defer srv1.Close()
	if _, err := st.CreateRunner(name, secret, "", "", ModeReverse, "ws"+strings.TrimPrefix(srv1.URL, "http")); err != nil {
		t.Fatalf("create runner: %v", err)
	}
	select {
	case <-got1:
	case <-time.After(20 * time.Second):
		t.Fatal("first url was never dialed")
	}

	srv2, got2 := newFake()
	defer srv2.Close()
	if err := st.DeleteRunner(name); err != nil {
		t.Fatalf("delete runner: %v", err)
	}
	if _, err := st.CreateRunner(name, secret, "", "", ModeReverse, "ws"+strings.TrimPrefix(srv2.URL, "http")); err != nil {
		t.Fatalf("recreate runner: %v", err)
	}
	select {
	case <-got2:
	case <-time.After(25 * time.Second):
		t.Fatal("new url was never dialed after url change")
	}
}

// TestReverseDialRejectsBadHash：runner 拒绝错误 hash（401）时保持离线，不会误判在线。
func TestReverseDialRejectsBadHash(t *testing.T) {
	t.Setenv("GITDASH_SSRF_ALLOW_PRIVATE", "1")
	dir := t.TempDir()
	st, err := store.Open(dir + "/test.db")
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	rdb := startTestRedis(t)
	h := NewHub(st, rdb)
	defer h.Stop()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
	}))
	defer srv.Close()
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")

	if _, err := st.CreateRunner("rev-bad", "sec", "", "", ModeReverse, wsURL); err != nil {
		t.Fatalf("create runner: %v", err)
	}

	time.Sleep(2 * time.Second) // 覆盖首次 reconcile + 拨号
	if h.IsOnline(context.Background(), "rev-bad") {
		t.Fatal("runner with rejected auth must stay offline")
	}
}
