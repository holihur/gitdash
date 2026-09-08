package runner

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"gitdash/backend/internal/gitsvc"
	"gitdash/backend/internal/store"
)

// fakeAgent 模拟 agent：hello → ack → 收工作区 → 验证 tar.gz → 回传日志与成功状态。
func fakeAgentRun(t *testing.T, srv *httptest.Server, name, secret string) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/api/runner/ws"
	ws, wresp, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{ //nolint:bodyclose // coder/websocket 的 resp.Body 由连接自身管理
		HTTPHeader: map[string][]string{"Authorization": {"Bearer " + name + ":" + secret}},
	})
	if err != nil {
		t.Errorf("dial: %v", err)
		return
	}
	defer func() { _ = ws.Close(websocket.StatusNormalClosure, "") }()
	_ = wresp // coder/websocket v1.8: 成功握手时 resp 非 nil，Body 由连接管理，不应外部关闭

	send := func(msg Message) bool {
		b, _ := json.Marshal(msg)
		return ws.Write(ctx, websocket.MessageText, b) == nil
	}
	if !send(Message{Type: TypeHello, Payload: mustJSON(Hello{Name: name})}) {
		return
	}

	workspace := &bytes.Buffer{}
	runID := int64(0)
	for {
		_, data, err := ws.Read(ctx)
		if err != nil {
			return // 连接正常关闭（测试结束）
		}
		var msg Message
		if json.Unmarshal(data, &msg) != nil {
			continue
		}
		switch msg.Type {
		case TypeJob:
			var job Job
			if json.Unmarshal(msg.Payload, &job) != nil {
				t.Errorf("bad job payload")
				return
			}
			runID = job.RunID
			send(Message{Type: TypeAck, Payload: mustJSON(Ack{JobID: job.JobID})})
		case TypeJobData:
			var jd JobData
			if json.Unmarshal(msg.Payload, &jd) != nil {
				continue
			}
			_, _ = workspace.Write(jd.Data)
			if !jd.Eof {
				continue
			}
			// 验证快照是合法的 tar.gz
			gz, err := gzip.NewReader(bytes.NewReader(workspace.Bytes()))
			if err != nil {
				t.Errorf("workspace not gzip: %v", err)
				return
			}
			tr := tar.NewReader(gz)
			if _, err := tr.Next(); err != nil {
				t.Errorf("workspace empty tar: %v", err)
				return
			}
			send(Message{Type: TypeJobLog, Payload: mustJSON(JobLog{RunID: runID, Chunk: "workspace ok\n"})})
			send(Message{Type: TypeJobStatus, Payload: mustJSON(JobStatus{RunID: runID, Status: "success"})})
		}
	}
}

func TestRunRemoteEndToEnd(t *testing.T) {
	dir := t.TempDir()
	if err := gitsvc.Init(dir); err != nil {
		t.Fatalf("gitsvc init: %v", err)
	}
	st, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	_ = st
	rdb := startTestRedis(t)
	h := NewHub(st, rdb)
	defer h.Stop()
	defer h.Stop()

	if _, err := st.CreateRunner("agent-e2e", "sec", "docker", ""); err != nil {
		t.Fatalf("create runner: %v", err)
	}

	// 建一个带 .gitdash.yml 的 bare 仓库
	repoPath := gitsvc.RepoPath("alice", "demo")
	if out, err := exec.Command("git", "init", "--bare", "-b", "main", repoPath).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	seed := filepath.Join(dir, "seed")
	if out, err := exec.Command("git", "clone", "--quiet", repoPath, seed).CombinedOutput(); err != nil {
		t.Fatalf("clone: %v %s", err, out)
	}
	for _, args := range [][]string{
		{"-C", seed, "config", "user.email", "t@t"},
		{"-C", seed, "config", "user.name", "t"},
		{"-C", seed, "-c", "commit.gpgsign=false", "commit", "--allow-empty", "-m", "init"},
	} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	if err := writeFileCommit(seed, ".gitdash.yml", "image: alpine\nsteps:\n  - name: build\n    run: echo ok\n"); err != nil {
		t.Fatalf("commit dsl: %v", err)
	}
	out, err := exec.Command("git", "-C", seed, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatalf("rev-parse: %v", err)
	}
	sha := strings.TrimSpace(string(out))

	srv := httptest.NewServer(http.HandlerFunc(h.HandleWS))
	t.Cleanup(srv.Close)
	go fakeAgentRun(t, srv, "agent-e2e", "sec")

	var r store.Runner
	deadline := time.Now().Add(5 * time.Second)
	for {
		var ok bool
		r, ok = h.SelectRunner([]string{"docker"}, []string{"", "user:alice"})
		if ok && r.Name == "agent-e2e" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("select runner: ok=%v r=%+v", ok, r)
		}
		time.Sleep(200 * time.Millisecond)
	}

	job := Job{JobID: "j-e2e", RunID: 42, Owner: "alice", Repo: "demo", SHA: sha, Ref: "main", DSL: "image: alpine"}
	snap, cleanup, err := snapshotForTest("alice", "demo", sha)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	defer cleanup()

	logSink := &bytes.Buffer{}
	done := make(chan error, 1)
	go func() { done <- h.RunRemote(context.Background(), "agent-e2e", job, snap, logSink, nil) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("RunRemote: %v; log=%q", err, logSink.String())
		}
	case <-time.After(15 * time.Second):
		t.Fatalf("RunRemote timed out; log=%q", logSink.String())
	}
	if !strings.Contains(logSink.String(), "workspace ok") {
		t.Fatalf("log missing chunk: %q", logSink.String())
	}
}

// writeFileCommit 在 seed 工作区写文件并提交推送。
func writeFileCommit(seedDir, name, content string) error {
	p := filepath.Join(seedDir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		return err
	}
	for _, args := range [][]string{
		{"-C", seedDir, "add", name},
		{"-C", seedDir, "-c", "commit.gpgsign=false", "commit", "-m", "dsl"},
		{"-C", seedDir, "push", "--quiet", "origin", "HEAD:main"},
	} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			return &testErr{err: err, out: string(out)}
		}
	}
	return nil
}

type testErr struct {
	err error
	out string
}

func (e *testErr) Error() string { return e.err.Error() + ": " + e.out }

// snapshotForTest 与 pipeline.workspaceSnapshot 相同的 git archive | gzip 流（避免包循环引用，测试内联实现）。
func snapshotForTest(owner, repo, sha string) (io.Reader, func(), error) {
	cmd := exec.Command("git", "-C", gitsvc.RepoPath(owner, repo), "archive", "--format=tar", sha)
	pr, pw := io.Pipe()
	gw := gzip.NewWriter(pw)
	cmd.Stdout = gw
	if err := cmd.Start(); err != nil {
		_ = pw.Close()
		return nil, nil, err
	}
	go func() {
		// exec 拷贝协程结束后再收尾 gzip/pipe（与写路径串行，避免竞争）
		werr := cmd.Wait()
		if gerr := gw.Close(); gerr != nil && werr == nil {
			werr = gerr
		}
		_ = pw.CloseWithError(werr)
	}()
	return pr, func() { _ = cmd.Process.Kill() }, nil
}
