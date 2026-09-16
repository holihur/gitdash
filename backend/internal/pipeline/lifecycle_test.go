package pipeline

import (
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"gitdash/backend/internal/gitsvc"
	"gitdash/backend/internal/store"
	"gitdash/backend/internal/webhooks"
)

// waitRunStatus 轮询直到运行到达期望状态之一（超时报错）。
func waitRunStatus(t *testing.T, st *store.Store, owner, repo string, id int64, want ...string) store.PipelineRun {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		r, err := st.GetPipelineRun(owner, repo, id)
		if err == nil {
			for _, w := range want {
				if r.Status == w {
					return r
				}
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	r, _ := st.GetPipelineRun(owner, repo, id)
	t.Fatalf("run %d status = %q, want one of %v", id, r.Status, want)
	return r
}

func openPipelineStore(t *testing.T, dir string) *store.Store {
	t.Helper()
	if err := gitsvc.Init(dir); err != nil {
		t.Fatalf("gitsvc init: %v", err)
	}
	if err := Init(dir); err != nil {
		t.Fatalf("pipeline init: %v", err)
	}
	st, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	return st
}

// TestPipelineEventsPublished 验证运行生命周期会发布 queued/started/success 事件。
func TestPipelineEventsPublished(t *testing.T) {
	dir := t.TempDir()
	st := openPipelineStore(t, dir)
	SetHostAllowed(true)
	t.Cleanup(func() { SetHostAllowed(false) })

	var mu sync.Mutex
	var events []webhooks.Event
	SetEventPublisher(func(ev webhooks.Event) {
		mu.Lock()
		events = append(events, ev)
		mu.Unlock()
	})
	t.Cleanup(func() { SetEventPublisher(nil) })

	sha := seedRepoFiles(t, "alice", "evt", map[string]string{
		".gitdash.yml": "steps:\n  - run: echo hi\n",
	})
	if err := st.SetPipeline("alice", "evt", true); err != nil {
		t.Fatal(err)
	}
	run, err := Trigger(st, TriggerOpts{Owner: "alice", Repo: "evt", SHA: sha, Ref: "main", By: "tester", Event: "manual", Force: true})
	if err != nil {
		t.Fatalf("trigger: %v", err)
	}
	waitRunStatus(t, st, "alice", "evt", run.ID, "success")

	mu.Lock()
	defer mu.Unlock()
	got := map[string]bool{}
	for _, ev := range events {
		if ev.Event == "pipeline" && ev.Number == run.ID {
			got[ev.Action] = true
			if ev.Actor != "tester" {
				t.Fatalf("pipeline event actor = %q", ev.Actor)
			}
		}
	}
	for _, a := range []string{"queued", "started", "success"} {
		if !got[a] {
			t.Fatalf("missing pipeline %q event: %v", a, got)
		}
	}
}

func TestDockerRunArgsNamesContainer(t *testing.T) {
	n1 := containerName(7)
	n2 := containerName(7)
	if n1 == n2 {
		t.Fatalf("container names should be unique: %q", n1)
	}
	if !strings.HasPrefix(n1, "gitdash-run-7-") {
		t.Fatalf("container name = %q", n1)
	}

	args := dockerRunArgs(n1, "/tmp/ws", &Config{Image: "alpine"}, Step{Name: "x", Run: "echo hi"},
		RunJob{RunID: 7, Owner: "a", Repo: "b", Ref: "main", SHA: "s"})
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--name "+n1) {
		t.Fatalf("missing --name: %v", args)
	}
	if !strings.Contains(joined, "--rm") {
		t.Fatalf("missing --rm: %v", args)
	}
	if args[len(args)-1] != "echo hi" {
		t.Fatalf("script not last: %v", args)
	}
}

func TestParseJobTimeout(t *testing.T) {
	cfg, err := Parse([]byte("job_timeout: 5m\nsteps:\n  - run: echo x\n"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg.JobTimeout != 5*time.Minute {
		t.Fatalf("job_timeout = %v, want 5m", cfg.JobTimeout)
	}
	// 缺省 = 不限
	cfg, err = Parse([]byte("steps:\n  - run: echo x\n"))
	if err != nil || cfg.JobTimeout != 0 {
		t.Fatalf("default job_timeout = %v, err=%v", cfg.JobTimeout, err)
	}
	if _, err := Parse([]byte("job_timeout: nonsense\nsteps:\n  - run: x\n")); err == nil {
		t.Fatal("invalid job_timeout should fail")
	}
	// 超上限自动截断到 MaxJobTimeout
	cfg, err = Parse([]byte("job_timeout: 100h\nsteps:\n  - run: x\n"))
	if err != nil {
		t.Fatalf("parse long: %v", err)
	}
	if cfg.JobTimeout != MaxJobTimeout {
		t.Fatalf("clamped job_timeout = %v, want %v", cfg.JobTimeout, MaxJobTimeout)
	}
}

func TestCancelBuiltinRun(t *testing.T) {
	dir := t.TempDir()
	st := openPipelineStore(t, dir)
	SetHostAllowed(true)
	t.Cleanup(func() { SetHostAllowed(false) })

	sha := seedRepoFiles(t, "alice", "cancel", map[string]string{
		".gitdash.yml": "steps:\n  - name: long\n    run: sleep 30\n",
	})
	if err := st.SetPipeline("alice", "cancel", true); err != nil {
		t.Fatal(err)
	}
	run, err := Trigger(st, TriggerOpts{Owner: "alice", Repo: "cancel", SHA: sha, Ref: "main", By: "tester", Event: "manual", Force: true})
	if err != nil {
		t.Fatalf("trigger: %v", err)
	}
	waitRunStatus(t, st, "alice", "cancel", run.ID, "running")

	if err := CancelRun(st, "alice", "cancel", run.ID); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	got := waitRunStatus(t, st, "alice", "cancel", run.ID, "cancelled")
	if got.Error == "" {
		t.Fatalf("cancelled run should carry a reason: %+v", got)
	}
	// 再次取消 → 已结束
	if err := CancelRun(st, "alice", "cancel", run.ID); err == nil {
		t.Fatal("cancelling a finished run should fail")
	}
}

func TestJobTimeoutFailsRun(t *testing.T) {
	dir := t.TempDir()
	st := openPipelineStore(t, dir)
	SetHostAllowed(true)
	t.Cleanup(func() { SetHostAllowed(false) })

	sha := seedRepoFiles(t, "alice", "timeout", map[string]string{
		".gitdash.yml": "job_timeout: 1s\nsteps:\n  - name: hang\n    run: sleep 30\n",
	})
	if err := st.SetPipeline("alice", "timeout", true); err != nil {
		t.Fatal(err)
	}
	run, err := Trigger(st, TriggerOpts{Owner: "alice", Repo: "timeout", SHA: sha, Ref: "main", By: "tester", Event: "manual", Force: true})
	if err != nil {
		t.Fatalf("trigger: %v", err)
	}
	got := waitRunStatus(t, st, "alice", "timeout", run.ID, "failed")
	if !strings.Contains(got.Error, "job timeout") {
		t.Fatalf("error = %q, want job timeout", got.Error)
	}
}

func TestDelayedExecution(t *testing.T) {
	dir := t.TempDir()
	st := openPipelineStore(t, dir)
	SetHostAllowed(true)
	t.Cleanup(func() { SetHostAllowed(false) })

	sha := seedRepoFiles(t, "alice", "delayed", map[string]string{
		".gitdash.yml": "steps:\n  - name: hi\n    run: echo hi\n",
	})
	if err := st.SetPipeline("alice", "delayed", true); err != nil {
		t.Fatal(err)
	}
	run, err := Trigger(st, TriggerOpts{Owner: "alice", Repo: "delayed", SHA: sha, Ref: "main", By: "tester", Event: "manual", Force: true, Delay: time.Hour})
	if err != nil {
		t.Fatalf("trigger: %v", err)
	}
	if run.RunAt == "" {
		t.Fatalf("delayed run must record run_at: %+v", run)
	}
	// 未到点：调度器不派发
	RunDelayed(st, time.Now().UTC())
	if cur, _ := st.GetPipelineRun("alice", "delayed", run.ID); cur.Status != "pending" {
		t.Fatalf("delayed run started early: %+v", cur)
	}
	// 到点后派发并成功
	RunDelayed(st, time.Now().UTC().Add(2*time.Hour))
	waitRunStatus(t, st, "alice", "delayed", run.ID, "success")
}
