package webhooks

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gitdash/backend/internal/store"
)

func TestDrainDeliversAndCleansSpool(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	spool := filepath.Join(dir, "events")
	if err := os.MkdirAll(spool, 0o755); err != nil {
		t.Fatal(err)
	}

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

	if _, err := st.CreateWebhook("alice", "demo", srv.URL+"/a"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateWebhook("alice", "demo", srv.URL+"/b"); err != nil {
		t.Fatal(err)
	}

	ev := `{"event":"push","owner":"alice","repo":"demo","old":"0000","new":"1111","ref":"refs/heads/main","user":"bob","created_at":"2026-09-04T00:00:00Z"}`
	if err := os.WriteFile(filepath.Join(spool, "alice__demo-x.json"), []byte(ev), 0o644); err != nil {
		t.Fatal(err)
	}

	drain(spool, st)

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
	drain(spool, st)
	left, _ = filepath.Glob(filepath.Join(spool, "*.json"))
	if len(left) != 0 {
		t.Fatalf("spool files not cleaned for unknown repo: %v", left)
	}
}
