package pipeline

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gitdash/backend/internal/gitsvc"
	"gitdash/backend/internal/store"
)

// seedPipelineRepo 在临时 data 目录下建裸仓库并推入含 .gitdash.yml 的提交，返回 head SHA。
func seedPipelineRepo(t *testing.T, owner, repo, yaml string) string {
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
	if err := os.WriteFile(filepath.Join(seed, "README.md"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit("-C", seed, "add", ".")
	runGit("-C", seed, "-c", "commit.gpgsign=false", "commit", "-m", "init")
	if err := os.WriteFile(filepath.Join(seed, ".gitdash.yml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit("-C", seed, "add", ".gitdash.yml")
	runGit("-C", seed, "-c", "commit.gpgsign=false", "commit", "-m", "pipeline")
	runGit("-C", seed, "push", "--quiet", "origin", "HEAD:main")
	out, err := exec.Command("git", "-C", repoPath, "rev-parse", "main").Output()
	if err != nil {
		t.Fatalf("rev-parse: %v", err)
	}
	return strings.TrimSpace(string(out))
}

func TestRunScheduledTriggersDueCron(t *testing.T) {
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
	seedPipelineRepo(t, "alice", "demo", "on: [schedule]\nschedule:\n  - \"* * * * *\"\nsteps:\n  - run: echo scheduled\n")
	if err := st.SetPipeline("alice", "demo", true); err != nil {
		t.Fatal(err)
	}
	// 预置 lastFired 为 2 分钟前 → cron 已到期
	past := time.Now().UTC().Add(-2 * time.Minute).Format(time.RFC3339)
	if claimed, err := st.ClaimSchedule("alice", "demo", "* * * * *", past); err != nil || !claimed {
		t.Fatalf("seed schedule: claimed=%v err=%v", claimed, err)
	}

	RunScheduled(st, time.Now().UTC())

	deadline := time.Now().Add(20 * time.Second)
	var id int64
	for time.Now().Before(deadline) && id == 0 {
		runs, _ := st.ListPipelineRuns("alice", "demo", 10)
		for _, r := range runs {
			if r.Event == "schedule" {
				id = r.ID
				break
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	if id == 0 {
		t.Fatal("no schedule run was created")
	}
	// 等运行结束，避免异步执行协程与 TempDir 清理竞态
	for time.Now().Before(deadline) {
		run, err := st.GetPipelineRun("alice", "demo", id)
		if err == nil && (run.Status == "success" || run.Status == "failed") {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatal("schedule run did not finish")
}

func TestRunScheduledSkipsWhenNotDue(t *testing.T) {
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
	// cron 每年 1 月 1 日 00:00；lastFired=刚刚 → 未到期
	seedPipelineRepo(t, "alice", "demo", "on: [schedule]\nschedule:\n  - \"0 0 1 1 *\"\nsteps:\n  - run: echo x\n")
	if err := st.SetPipeline("alice", "demo", true); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if claimed, _ := st.ClaimSchedule("alice", "demo", "0 0 1 1 *", now.Format(time.RFC3339)); !claimed {
		t.Fatal("seed failed")
	}
	RunScheduled(st, now)
	if runs, _ := st.ListPipelineRuns("alice", "demo", 10); len(runs) != 0 {
		t.Fatalf("unexpected runs: %#v", runs)
	}
}
