package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"gitdash/backend/internal/gitsvc"
	"gitdash/backend/internal/queue"
	"gitdash/backend/internal/store"
	"gitdash/backend/internal/webhooks"
)

// FileName 仓库根目录中的流水线定义文件。
const FileName = ".gitdash.yml"

// KindPipelineRun 队列任务类型：执行一次流水线运行。
const KindPipelineRun = "pipeline:run"

var logsDir string

var (
	// ErrNoPipeline 目标提交上没有流水线定义文件（push 触发时静默跳过）。
	ErrNoPipeline = errors.New("no pipeline file")
	// ErrTooManyRuns 同仓库进行中的运行过多（MVP 保护：最多同时 3 个）。
	ErrTooManyRuns = errors.New("too many active runs")
	// ErrDockerMissing docker 不可用。
	ErrDockerMissing = errors.New("docker not available")
)

const maxActiveRuns = 3

const maxLogBytes = 512 << 10

// RunJob 队列载荷：执行一次流水线所需的最小信息。
type RunJob struct {
	RunID int64  `json:"run_id"`
	Owner string `json:"owner"`
	Repo  string `json:"repo"`
	SHA   string `json:"sha"`
	Ref   string `json:"ref"`
}

var (
	boundStore *store.Store
	boundQueue queue.Queue
)

// Bind 绑定任务队列消费者。q 为 nil 时沿用进程内 goroutine 直接调度（默认，零依赖）。
// 队列模式（如 asynq/redis）下，Trigger 只入队，由 Start 启动的工人真正执行。
func Bind(st *store.Store, q queue.Queue) {
	boundStore, boundQueue = st, q
	if q != nil {
		q.Start(context.Background(), []queue.JobKind{KindPipelineRun}, jobHandler)
	}
}

// jobHandler 队列工人入口。
func jobHandler(ctx context.Context, job queue.Job) error {
	if job.Kind != KindPipelineRun {
		return nil
	}
	var rj RunJob
	if err := json.Unmarshal(job.Payload, &rj); err != nil {
		return err
	}
	if boundStore == nil {
		return errors.New("pipeline store not bound")
	}
	executeRun(boundStore, rj)
	return nil
}

// Init 创建流水线日志目录（main 启动时调用）。
// 环境变量 GITDASH_PIPELINE_DEFAULT_TIMEOUT 可覆盖单步默认超时（如 "30s"，用于测试）。
func Init(dataDir string) error {
	if d, err := time.ParseDuration(os.Getenv("GITDASH_PIPELINE_DEFAULT_TIMEOUT")); err == nil && d > 0 && d <= MaxStepTimeout {
		DefaultStepTimeout = d
	}
	logsDir = filepath.Join(dataDir, "pipelines")
	return os.MkdirAll(logsDir, 0o755)
}

// LogPath 运行日志落盘位置：data/pipelines/{owner}/{repo}/run-{id}.log。
func LogPath(owner, repo string, id int64) string {
	return filepath.Join(logsDir, owner, repo, fmt.Sprintf("run-%d.log", id))
}

// DeleteLogs 删除仓库全部运行日志（删仓库时调用）。
func DeleteLogs(owner, repo string) error {
	if logsDir == "" {
		return nil
	}
	return os.RemoveAll(filepath.Join(logsDir, owner, repo))
}

// ReadLog 读取运行日志（超长截断）。
func ReadLog(owner, repo string, id int64) (string, error) {
	b, err := os.ReadFile(LogPath(owner, repo, id))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	if len(b) > maxLogBytes {
		return string(b[:maxLogBytes]) + "\n... (truncated)\n", nil
	}
	return string(b), nil
}

// PushHandler 返回挂到 webhook spool 调度器上的 push 事件处理器：
// 仓库开启流水线且该提交含 .gitdash.yml 时触发一次运行。
func PushHandler(st *store.Store) func(webhooks.Event) {
	return func(ev webhooks.Event) {
		if ev.Event != "push" || ev.New == "" || isZeroSHA(ev.New) {
			return
		}
		if !strings.HasPrefix(ev.Ref, "refs/heads/") {
			return
		}
		if !gitsvc.ValidName(ev.Owner) || !gitsvc.ValidName(ev.Repo) {
			return
		}
		if !st.IsPipelineEnabled(ev.Owner, ev.Repo) {
			return
		}
		branch := strings.TrimPrefix(ev.Ref, "refs/heads/")
		if _, err := Trigger(st, ev.Owner, ev.Repo, ev.New, branch, ev.User); err != nil &&
			!errors.Is(err, ErrNoPipeline) && !errors.Is(err, ErrTooManyRuns) {
			log.Printf("pipeline: trigger %s/%s: %v", ev.Owner, ev.Repo, err)
		}
	}
}

// Trigger 创建一次流水线运行（push 或手动）。
// 提交上无 .gitdash.yml 时返回 ErrNoPipeline；DSL 解析错误会记为 failed 的运行，便于排查。
func Trigger(st *store.Store, owner, repo, sha, ref, by string) (store.PipelineRun, error) {
	if active, err := st.RunningPipelineRunIDs(owner, repo); err == nil && len(active) >= maxActiveRuns {
		return store.PipelineRun{}, ErrTooManyRuns
	}
	blob, err := gitsvc.ReadBlob(owner, repo, sha, FileName)
	if err != nil || blob.Encoding != "utf-8" || strings.TrimSpace(blob.Content) == "" {
		return store.PipelineRun{}, ErrNoPipeline
	}
	cfg, perr := Parse([]byte(blob.Content))
	if perr != nil {
		run, cerr := st.CreatePipelineRun(owner, repo, sha, ref, by, 0)
		if cerr != nil {
			return run, cerr
		}
		msg := "invalid " + FileName + ": " + perr.Error()
		_ = st.FinishPipelineRun(run.ID, "failed", msg)
		run.Status = "failed"
		run.Error = msg
		return run, nil
	}
	run, err := st.CreatePipelineRun(owner, repo, sha, ref, by, len(cfg.Steps))
	if err != nil {
		return run, err
	}
	job := RunJob{RunID: run.ID, Owner: owner, Repo: repo, SHA: sha, Ref: ref}
	if boundQueue == nil {
		// 进程内直接调度（默认）
		go executeRun(st, job)
		return run, nil
	}
	payload, err := json.Marshal(job)
	if err != nil {
		return run, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	qj := queue.Job{
		Kind:    KindPipelineRun,
		ID:      fmt.Sprintf("%s-%s-%d", owner, repo, run.ID),
		Payload: payload,
	}
	if err := boundQueue.Enqueue(ctx, qj); err != nil {
		msg := "enqueue: " + err.Error()
		_ = st.FinishPipelineRun(run.ID, "failed", msg)
		run.Status = "failed"
		run.Error = msg
		return run, nil
	}
	return run, nil
}

// executeRun 执行流水线：checkout 到触发提交，逐步骤在 docker 容器中运行并写日志。
func executeRun(st *store.Store, job RunJob) {
	runID, owner, repo, sha, ref := job.RunID, job.Owner, job.Repo, job.SHA, job.Ref
	_ = st.StartPipelineRun(runID)

	lf := LogPath(owner, repo, runID)
	if err := os.MkdirAll(filepath.Dir(lf), 0o755); err != nil {
		_ = st.FinishPipelineRun(runID, "failed", "create log dir: "+err.Error())
		return
	}
	logFile, err := os.Create(lf)
	if err != nil {
		_ = st.FinishPipelineRun(runID, "failed", "create log file: "+err.Error())
		return
	}
	defer func() { _ = logFile.Close() }()

	writeLog := func(format string, args ...any) {
		_, _ = fmt.Fprintf(logFile, format+"\n", args...)
	}
	fail := func(format string, args ...any) {
		msg := fmt.Sprintf(format, args...)
		writeLog("!! %s", msg)
		_ = st.FinishPipelineRun(runID, "failed", msg)
	}

	writeLog("== gitdash pipeline run %d ==", runID)
	writeLog("repo: %s/%s  ref: %s  sha: %s", owner, repo, ref, sha)

	// 执行时重新读取并解析 DSL（Trigger 已校验过；此处失败则直接记 failed）
	blob, err := gitsvc.ReadBlob(owner, repo, sha, FileName)
	if err != nil || blob.Encoding != "utf-8" || strings.TrimSpace(blob.Content) == "" {
		fail("pipeline file %s not found at %s", FileName, sha)
		return
	}
	cfg, perr := Parse([]byte(blob.Content))
	if perr != nil {
		fail("invalid %s: %v", FileName, perr)
		return
	}
	writeLog("image: %s  steps: %d", cfg.Image, len(cfg.Steps))

	if err := dockerAvailable(); err != nil {
		fail("docker not available: %v", err)
		return
	}

	tmp, err := os.MkdirTemp("", "gitdash-run-*")
	if err != nil {
		fail("create workdir: %v", err)
		return
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	// checkout 触发提交到临时工作区
	if out, err := gitsvc.GitOut("", "clone", "--quiet", gitsvc.RepoPath(owner, repo), tmp); err != nil {
		fail("clone repo: %v: %s", err, strings.TrimSpace(out))
		return
	}
	if out, err := gitsvc.GitOut(tmp, "checkout", "--quiet", "--detach", sha); err != nil {
		fail("checkout %s: %v: %s", sha, err, strings.TrimSpace(out))
		return
	}
	writeLog("workspace: checked out %s", sha)

	for i, step := range cfg.Steps {
		writeLog("\n==> [%d/%d] %s", i+1, len(cfg.Steps), step.Name)
		if err := runStep(tmp, cfg, step, owner, repo, ref, sha, logFile); err != nil {
			fail("step %q failed: %v", step.Name, err)
			return
		}
		writeLog("<== %s ok", step.Name)
		_ = st.ProgressPipelineRun(runID, i+1)
	}

	writeLog("\n== pipeline success ==")
	_ = st.FinishPipelineRun(runID, "success", "")
}

// runStep 在 docker 容器里执行单步：工作区挂载到 /workspace，输出实时写入日志。
func runStep(workdir string, cfg *Config, step Step, owner, repo, ref, sha string, logFile *os.File) error {
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	args := []string{
		"run", "--rm",
		"--workdir", "/workspace",
		"-v", workdir + ":/workspace",
		"-e", "CI=1",
		"-e", "GITDASH=true",
		"-e", "GITDASH_REPO=" + owner + "/" + repo,
		"-e", "GITDASH_REF=" + ref,
		"-e", "GITDASH_SHA=" + sha,
	}
	for _, e := range cfg.Env {
		args = append(args, "-e", e)
	}
	args = append(args, cfg.Image, "sh", "-ec", step.Run)

	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("timeout after %s", cfg.Timeout)
		}
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return fmt.Errorf("exit code %d", ee.ExitCode())
		}
		return err
	}
	return nil
}

// dockerAvailable 检查 docker CLI 与守护进程是否可用。
func dockerAvailable() error {
	if _, err := exec.LookPath("docker"); err != nil {
		return ErrDockerMissing
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "docker", "version", "--format", "{{.Server.Version}}").CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", ErrDockerMissing, strings.TrimSpace(string(out)))
	}
	return nil
}

func isZeroSHA(s string) bool {
	if len(s) != 40 {
		return false
	}
	return strings.Trim(s, "0") == ""
}
