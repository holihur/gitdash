package webhooks

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gitdash/backend/internal/jobs"
	"gitdash/backend/internal/queue"
	"gitdash/backend/internal/store"
)

// testDispatcher 当前测试绑定的 Dispatcher（测试内单实例）。
var testDispatcher *Dispatcher

// bindQueue 为异步 webhook 投递绑定内存队列 + worker（drain 只负责入队）。
func bindQueue(t *testing.T, st *store.Store) {
	t.Helper()
	mgr := jobs.New(st, queue.NewMemory(64, 2))
	testDispatcher = New(st, mgr)
	mgr.SetWebhookHandler(testDispatcher.HandleJob)
	mgr.Start()
}

// drain 测试包装：委托给已绑定的 Dispatcher。
func drain(spoolDir string, _ *store.Store, handlers []func(Event)) {
	testDispatcher.drain(spoolDir, handlers)
}

func TestDrainDeliversAndCleansSpool(t *testing.T) {
	t.Setenv("GITDASH_SSRF_ALLOW_PRIVATE", "1") // httptest 端点为回环地址
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	spool := filepath.Join(dir, "events")
	if err := os.MkdirAll(spool, 0o755); err != nil {
		t.Fatal(err)
	}
	bindQueue(t, st)

	delivered := make(chan map[string]any, 4)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("bad request: %s %s", r.Method, r.Header.Get("Content-Type"))
		}
		var m map[string]any
		_ = json.NewDecoder(r.Body).Decode(&m)
		delivered <- m
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if _, err := st.CreateWebhook("alice", "demo", srv.URL+"/a", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateWebhook("alice", "demo", srv.URL+"/b", ""); err != nil {
		t.Fatal(err)
	}

	ev := `{"event":"push","owner":"alice","repo":"demo","old":"0000","new":"1111","ref":"refs/heads/main","user":"bob","created_at":"2026-09-04T00:00:00Z"}`
	if err := os.WriteFile(filepath.Join(spool, "alice__demo-x.json"), []byte(ev), 0o644); err != nil {
		t.Fatal(err)
	}

	drain(spool, st, nil)

	for i := 0; i < 2; i++ {
		select {
		case m := <-delivered:
			if m["event"] != "push" || m["user"] != "bob" || m["owner"] != "alice" || m["repo"] != "demo" {
				t.Fatalf("delivered payload = %v", m)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("missing delivery")
		}
	}
	// spool 消费后清理
	left, _ := filepath.Glob(filepath.Join(spool, "*.json"))
	if len(left) != 0 {
		t.Fatalf("spool files not cleaned: %v", left)
	}

	// 仓库无 webhook 时：事件丢弃并清理
	if err := os.WriteFile(filepath.Join(spool, "bob__other.json"), []byte(ev), 0o644); err != nil {
		t.Fatal(err)
	}
	drain(spool, st, nil)
	left, _ = filepath.Glob(filepath.Join(spool, "*.json"))
	if len(left) != 0 {
		t.Fatalf("spool files not cleaned for unknown repo: %v", left)
	}
}

func TestWebhookSignatureHeader(t *testing.T) {
	t.Setenv("GITDASH_SSRF_ALLOW_PRIVATE", "1") // httptest 端点为回环地址
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	spool := filepath.Join(dir, "events")
	_ = os.MkdirAll(spool, 0o755)
	bindQueue(t, st)

	got := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mac := hmac.New(sha256.New, []byte("super-secret-key-123"))
		mac.Write(b)
		want := "sha256=" + hex.EncodeToString(mac.Sum(nil))
		if r.Header.Get("X-Gitdash-Signature") != want {
			t.Errorf("bad signature header %q want %q", r.Header.Get("X-Gitdash-Signature"), want)
		}
		got <- r.Header.Get("X-Gitdash-Signature")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	_, _ = st.CreateWebhook("alice", "demo", srv.URL+"/h", "super-secret-key-123")
	ev := `{"event":"push","owner":"alice","repo":"demo","ref":"refs/heads/main"}`
	_ = os.WriteFile(filepath.Join(spool, "alice__demo-a.json"), []byte(ev), 0o644)
	drain(spool, st, nil)
	select {
	case h := <-got:
		if !strings.HasPrefix(h, "sha256=") {
			t.Fatalf("signature header = %q", h)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no delivery")
	}
}

func TestSubscribesCoversAllAdvertisedEvents(t *testing.T) {
	// 空订阅 = 全部
	if !Subscribes(nil, "push") || !Subscribes([]string{}, "pipeline") {
		t.Fatal("empty subscription should match every event")
	}
	// 每个对外声明的事件类型都应可被精确订阅
	for _, e := range EventTypes {
		if !Subscribes([]string{e}, e) {
			t.Fatalf("event %q is not subscribable", e)
		}
	}
	if Subscribes([]string{"star"}, "pipeline") {
		t.Fatal("star subscription should not match pipeline")
	}
}

func TestDeliverBlocksPrivateWhenNotAllowed(t *testing.T) {
	t.Setenv("GITDASH_SSRF_ALLOW_PRIVATE", "")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if _, err := deliver(srv.URL, []byte("{}"), ""); err == nil {
		t.Fatal("expected loopback delivery to be blocked by SSRF guard")
	}
}
