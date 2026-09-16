package pipeline

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"gitdash/backend/internal/gitsvc"
	"gitdash/backend/internal/runner"
	"gitdash/backend/internal/store"
)

// seedRepoFiles 建 bare 仓库并推入一组文件，返回 main 的 head SHA。
func seedRepoFiles(t *testing.T, owner, repo string, files map[string]string) string {
	t.Helper()
	repoPath := gitsvc.RepoPath(owner, repo)
	runGit := func(args ...string) {
		t.Helper()
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	runGit("init", "--bare", "-b", "main", repoPath)
	seed := t.TempDir()
	runGit("clone", "--quiet", repoPath, seed)
	runGit("-C", seed, "config", "user.email", "t@t")
	runGit("-C", seed, "config", "user.name", "t")
	for p, c := range files {
		full := filepath.Join(seed, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runGit("-C", seed, "add", "-A")
	runGit("-C", seed, "-c", "commit.gpgsign=false", "commit", "-m", "init")
	runGit("-C", seed, "push", "--quiet", "origin", "HEAD:main")
	out, err := exec.Command("git", "-C", repoPath, "rev-parse", "main").Output()
	if err != nil {
		t.Fatalf("rev-parse: %v", err)
	}
	return strings.TrimSpace(string(out))
}

func waitRuns(t *testing.T, st *store.Store, owner, repo string) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		runs, err := st.ListPipelineRuns(owner, repo, 20)
		if err != nil {
			t.Fatal(err)
		}
		done := true
		for _, r := range runs {
			if r.Status == "pending" || r.Status == "running" {
				done = false
				break
			}
		}
		if done {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("pipeline runs did not finish")
}

func TestDiscoverFiles(t *testing.T) {
	dir := t.TempDir()
	if err := gitsvc.Init(dir); err != nil {
		t.Fatalf("gitsvc init: %v", err)
	}
	sha := seedRepoFiles(t, "alice", "multi", map[string]string{
		".gitdash.yml":         "steps:\n  - run: echo legacy\n",
		".gitdash/ci.yml":      "steps:\n  - run: echo ci\n",
		".gitdash/deploy.yaml": "steps:\n  - run: echo deploy\n",
		".gitdash/notes.txt":   "not a pipeline\n",
		".gitdash/sub/x.yml":   "steps:\n  - run: echo nested\n",
	})
	got := DiscoverFiles("alice", "multi", sha)
	want := []string{".gitdash.yml", ".gitdash/ci.yml", ".gitdash/deploy.yaml"}
	sort.Strings(want)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("DiscoverFiles = %v, want %v", got, want)
	}

	// 无任何流水线文件
	empty := seedRepoFiles(t, "alice", "none", map[string]string{"README.md": "x\n"})
	if files := DiscoverFiles("alice", "none", empty); len(files) != 0 {
		t.Fatalf("expected no files, got %v", files)
	}
}

// TestScheduledTriggersPerFile 验证同一仓库多个流水线文件各自按 cron 独立触发（去重键含 file）。
func TestScheduledTriggersPerFile(t *testing.T) {
	dir := t.TempDir()
	if err := gitsvc.Init(dir); err != nil {
		t.Fatalf("gitsvc init: %v", err)
	}
	if err := Init(dir); err != nil {
		t.Fatalf("pipeline init: %v", err)
	}
	SetHostAllowed(true)
	t.Cleanup(func() { SetHostAllowed(false) })

	st, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	seedRepoFiles(t, "alice", "sched", map[string]string{
		".gitdash/a.yml": "on: [schedule]\nschedule:\n  - \"* * * * *\"\nsteps:\n  - run: echo a\n",
		".gitdash/b.yml": "on: [schedule]\nschedule:\n  - \"* * * * *\"\nsteps:\n  - run: echo b\n",
	})
	if err := st.SetPipeline("alice", "sched", true); err != nil {
		t.Fatal(err)
	}
	past := time.Now().UTC().Add(-2 * time.Minute).Format(time.RFC3339)
	for _, f := range []string{".gitdash/a.yml", ".gitdash/b.yml"} {
		if claimed, err := st.ClaimSchedule("alice", "sched", f, "* * * * *", past); err != nil || !claimed {
			t.Fatalf("seed schedule %s: claimed=%v err=%v", f, claimed, err)
		}
	}

	RunScheduled(st, time.Now().UTC())

	deadline := time.Now().Add(20 * time.Second)
	files := map[string]bool{}
	for time.Now().Before(deadline) && len(files) < 2 {
		runs, _ := st.ListPipelineRuns("alice", "sched", 20)
		for _, r := range runs {
			if r.Event == "schedule" {
				files[r.File] = true
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !files[".gitdash/a.yml"] || !files[".gitdash/b.yml"] {
		t.Fatalf("expected a schedule run per file, got %v", files)
	}
	// 等待终态，避免异步执行协程与 TempDir 清理竞态
	deadline = time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		runs, _ := st.ListPipelineRuns("alice", "sched", 20)
		done := len(runs) >= 2
		for _, r := range runs {
			if r.Status == "pending" || r.Status == "running" {
				done = false
			}
		}
		if done {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("schedule runs did not finish")
}

func TestTriggerAllMultipleFiles(t *testing.T) {
	dir := t.TempDir()
	if err := gitsvc.Init(dir); err != nil {
		t.Fatalf("gitsvc init: %v", err)
	}
	if err := Init(dir); err != nil {
		t.Fatalf("pipeline init: %v", err)
	}
	SetHostAllowed(true)
	t.Cleanup(func() { SetHostAllowed(false) })

	st, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	sha := seedRepoFiles(t, "alice", "multi", map[string]string{
		".gitdash.yml":         "on: [push]\nsteps:\n  - run: echo legacy\n",
		".gitdash/ci.yml":      "on: [push]\nsteps:\n  - run: echo ci\n",
		".gitdash/pr-only.yml": "on: [pull_request]\nsteps:\n  - run: echo pr\n",
	})
	if err := st.SetPipeline("alice", "multi", true); err != nil {
		t.Fatal(err)
	}

	// push 事件：仅触发 on 含 push 的两个文件
	runs, err := TriggerAll(st, TriggerOpts{Owner: "alice", Repo: "multi", SHA: sha, Ref: "main", By: "tester", Event: "push"})
	if err != nil {
		t.Fatalf("TriggerAll: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("push runs = %d, want 2: %#v", len(runs), runs)
	}
	got := map[string]bool{}
	for _, r := range runs {
		got[r.File] = true
	}
	if !got[".gitdash.yml"] || !got[".gitdash/ci.yml"] {
		t.Fatalf("unexpected files: %v", got)
	}
	waitRuns(t, st, "alice", "multi")

	// pull_request 事件：仅触发 pr-only
	runs, err = TriggerAll(st, TriggerOpts{Owner: "alice", Repo: "multi", SHA: sha, Ref: "main", By: "tester", Event: "pull_request"})
	if err != nil {
		t.Fatalf("TriggerAll pr: %v", err)
	}
	if len(runs) != 1 || runs[0].File != ".gitdash/pr-only.yml" {
		t.Fatalf("pr runs = %#v", runs)
	}
	waitRuns(t, st, "alice", "multi")

	// 指定文件触发（Force）
	one, err := TriggerAll(st, TriggerOpts{Owner: "alice", Repo: "multi", File: ".gitdash/ci.yml", SHA: sha, Ref: "main", By: "tester", Event: "manual", Force: true})
	if err != nil || len(one) != 1 || one[0].File != ".gitdash/ci.yml" {
		t.Fatalf("single file trigger = %#v, %v", one, err)
	}
	waitRuns(t, st, "alice", "multi")

	// 运行列表按文件区分
	all, _ := st.ListPipelineRuns("alice", "multi", 50)
	if len(all) != 4 {
		t.Fatalf("total runs = %d, want 4", len(all))
	}
}

// TestTriggerAllAllThrottled 验证所有文件都达并发上限时，TriggerAll 返回 ErrTooManyRuns。
func TestTriggerAllAllThrottled(t *testing.T) {
	dir := t.TempDir()
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
	sha := seedRepoFiles(t, "alice", "throttle", map[string]string{
		".gitdash/a.yml": "on: [push]\nsteps:\n  - run: echo a\n",
		".gitdash/b.yml": "on: [push]\nsteps:\n  - run: echo b\n",
	})
	if err := st.SetPipeline("alice", "throttle", true); err != nil {
		t.Fatal(err)
	}
	// 每个文件都堆满 maxActiveRuns（3）个进行中的运行
	for _, f := range []string{".gitdash/a.yml", ".gitdash/b.yml"} {
		for i := 0; i < maxActiveRuns; i++ {
			if _, err := st.CreatePipelineRun("alice", "throttle", f, sha, "main", "tester", "push", nil, 1); err != nil {
				t.Fatal(err)
			}
		}
	}
	runs, err := TriggerAll(st, TriggerOpts{Owner: "alice", Repo: "throttle", SHA: sha, Ref: "main", By: "tester", Event: "push"})
	if !errors.Is(err, ErrTooManyRuns) {
		t.Fatalf("err = %v, want ErrTooManyRuns (runs=%d)", err, len(runs))
	}
	if len(runs) != 0 {
		t.Fatalf("expected no new runs, got %d", len(runs))
	}
}

// recordingHub 记录远程派发收到的 runner.Job（用于验证多文件下发的 DSL）。
type recordingHub struct {
	mu  sync.Mutex
	job runner.Job
}

func (h *recordingHub) SelectRunner(labels, scopes []string) (store.Runner, bool) {
	return store.Runner{Name: "fake-runner"}, true
}

func (h *recordingHub) RunRemote(_ context.Context, _ string, job runner.Job, workspace io.Reader, _ io.Writer, _ func(int)) error {
	_, _ = io.Copy(io.Discard, workspace)
	h.mu.Lock()
	h.job = job
	h.mu.Unlock()
	return nil
}

func (h *recordingHub) Cancel(int64) {}

// TestRemoteDispatchUsesSelectedFile 验证 runs-on 远程派发时按 job.File 读取并下发对应的 DSL。
func TestRemoteDispatchUsesSelectedFile(t *testing.T) {
	dir := t.TempDir()
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
	sha := seedRepoFiles(t, "alice", "remote", map[string]string{
		".gitdash/ci.yml": "image: alpine\nsteps:\n  - run: echo FROM_CI_FILE\n",
		".gitdash.yml":    "image: alpine\nsteps:\n  - run: echo FROM_ROOT_FILE\n",
	})

	hub := &recordingHub{}
	Bind(st, nil, nil)
	BindHub(hub)
	t.Cleanup(func() { Bind(nil, nil, nil); BindHub(nil) })

	cfg := &Config{RunsOn: []string{"docker"}, Image: "alpine"}
	var logBuf bytes.Buffer
	err = (&dispatchExecutor{}).Execute(context.Background(),
		RunJob{RunID: 1, Owner: "alice", Repo: "remote", File: ".gitdash/ci.yml", SHA: sha, Ref: "main"},
		cfg, &logBuf, func(int) {})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	hub.mu.Lock()
	dsl := hub.job.DSL
	hub.mu.Unlock()
	if !strings.Contains(dsl, "FROM_CI_FILE") {
		t.Fatalf("dispatched DSL = %q, want selected file content", dsl)
	}
	if strings.Contains(dsl, "FROM_ROOT_FILE") {
		t.Fatalf("dispatched wrong file: %q", dsl)
	}
}
