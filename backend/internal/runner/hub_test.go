package runner

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/redis/go-redis/v9"

	"gitdash/backend/internal/store"
)

// startTestRedis 启动临时 redis-server（与 queue 包测试相同模式；缺失则跳过）。
func startTestRedis(t *testing.T) *redis.Client {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	cmd := exec.Command("redis-server", "--port", strconv.Itoa(port), "--save", "", "--appendonly", "no")
	if err := cmd.Start(); err != nil {
		t.Skipf("redis-server start: %v", err)
	}
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	deadline := time.Now().Add(5 * time.Second)
	for {
		c, err := net.DialTimeout("tcp", addr, time.Second)
		if err == nil {
			_ = c.Close()
			break
		}
		if time.Now().After(deadline) {
			_ = cmd.Process.Kill()
			t.Skipf("redis not reachable: %v", err)
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _, _ = cmd.Process.Wait() })
	return redis.NewClient(&redis.Options{Addr: addr})
}

func TestHubDispatchAndHeartbeat(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir + "/test.db")
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	_ = st

	rdb := startTestRedis(t)
	h := NewHub(st, rdb)
	defer h.Stop()
	defer h.Stop()
	if !h.Enabled() {
		t.Fatal("hub should be enabled")
	}

	if _, err := st.CreateRunner("agent-1", "sec-1", "docker", "user:alice"); err != nil {
		t.Fatalf("create runner: %v", err)
	}

	// 假 agent：连 WS，发 hello + heartbeat
	srv := httptest.NewServer(http.HandlerFunc(h.HandleWS))
	defer srv.Close()
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/api/runner/ws"
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ws, wresp, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{ //nolint:bodyclose // coder/websocket 的 resp.Body 由连接自身管理
		HTTPHeader: map[string][]string{"Authorization": {"Bearer agent-1:sec-1"}},
	})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = ws.Close(websocket.StatusNormalClosure, "") }()
	_ = wresp // coder/websocket v1.8: 成功握手时 resp 非 nil，Body 由连接管理，不应外部关闭

	send := func(msg Message) {
		t.Helper()
		b, _ := json.Marshal(msg)
		if err := ws.Write(ctx, websocket.MessageText, b); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	recv := func() Message {
		t.Helper()
		_, data, err := ws.Read(ctx)
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		var m Message
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		return m
	}

	send(Message{Type: TypeHello, Payload: mustJSON(Hello{Name: "agent-1", Labels: []string{"docker"}})})
	send(Message{Type: TypeHeartbeat})
	time.Sleep(300 * time.Millisecond)

	if !h.IsOnline(ctx, "agent-1") {
		t.Fatal("agent should be online after heartbeat")
	}

	// 服务端 → agent 派发（经 Redis pub/sub 跨 goroutine）
	job := Job{JobID: "j1", RunID: 7, Owner: "a", Repo: "b", SHA: "deadbeef", Ref: "main", DSL: "image: alpine"}
	if err := h.Dispatch("agent-1", Message{Type: TypeJob, Payload: mustJSON(job)}); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	got := recv()
	if got.Type != TypeJob {
		t.Fatalf("got type %q, want job", got.Type)
	}
	var gotJob Job
	if err := json.Unmarshal(got.Payload, &gotJob); err != nil || gotJob.JobID != "j1" || gotJob.RunID != 7 {
		t.Fatalf("job payload: %+v err=%v", gotJob, err)
	}

	// agent → server ack 事件回调
	ackCh := make(chan Ack, 4)
	AgentEvent = func(_ context.Context, runner string, msg Message) {
		if runner == "agent-1" && msg.Type == TypeAck {
			var a Ack
			_ = json.Unmarshal(msg.Payload, &a)
			ackCh <- a
		}
	}
	defer func() { AgentEvent = nil }()
	send(Message{Type: TypeAck, Payload: mustJSON(Ack{JobID: "j1"})})
	select {
	case a := <-ackCh:
		if a.JobID != "j1" {
			t.Fatalf("ack callback: %+v", a)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("ack callback not fired")
	}
}

func TestHubRejectsBadSecret(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir + "/test.db")
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	_ = st
	rdb := startTestRedis(t)
	h := NewHub(st, rdb)
	defer h.Stop()
	defer h.Stop()

	if _, err := st.CreateRunner("agent-2", "sec-2", "", ""); err != nil {
		t.Fatalf("create runner: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(h.HandleWS))
	defer srv.Close()
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/api/runner/ws"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, wresp, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPHeader: map[string][]string{"Authorization": {"Bearer agent-2:wrong"}},
	})
	if err == nil {
		t.Fatal("bad secret should be rejected")
	}
	if wresp != nil {
		_ = wresp.Body.Close()
		if wresp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", wresp.StatusCode)
		}
	} else {
		t.Fatal("bad secret should be rejected with a response")
	}
}

func TestOfflineSweepFailsRuns(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir + "/test.db")
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	_ = st
	rdb := startTestRedis(t)
	h := NewHub(st, rdb)
	defer h.Stop()
	defer h.Stop()

	if _, err := st.CreateRunner("agent-3", "sec-3", "", ""); err != nil {
		t.Fatalf("create runner: %v", err)
	}
	run, err := st.CreatePipelineRun("alice", "demo", "s", "main", "alice", 2)
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := st.StartPipelineRun(run.ID); err != nil {
		t.Fatalf("start run: %v", err)
	}
	if err := st.DB().Exec("UPDATE pipeline_runs SET runner_name = 'agent-3' WHERE id = ?", run.ID).Error; err != nil {
		t.Fatalf("set runner_name: %v", err)
	}
	// DB 状态 online，但 redis 无心跳 → sweep 应判 offline 并把 run 标 failed
	if err := st.SetRunnerStatus("agent-3", "online"); err != nil {
		t.Fatalf("set status: %v", err)
	}
	h.sweepOffline(context.Background())

	got, err := st.GetPipelineRun("alice", "demo", run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if got.Status != "failed" || !strings.Contains(got.Error, "offline") {
		t.Fatalf("run status=%q error=%q, want failed/offline", got.Status, got.Error)
	}
	rr, err := st.GetRunner("agent-3")
	if err != nil {
		t.Fatalf("get runner: %v", err)
	}
	if rr.Status != "offline" {
		t.Fatalf("runner status=%q, want offline", rr.Status)
	}
}
