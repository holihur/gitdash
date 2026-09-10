package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
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
	// ErrHostDisabled host（无 Docker）执行未开启：image 留空的流水线被拒绝。
	ErrHostDisabled = errors.New("host execution is disabled (set GITDASH_PIPELINE_EXEC=host on the server, or register the runner with -exec host)")
)

// hostAllowed host（无 Docker）执行开关：环境变量 GITDASH_PIPELINE_EXEC=host 时开启。
// 流水线 .gitdash.yml 省略 image 即直接在宿主 sh 执行（无容器沙箱），默认关闭。
var hostAllowed = false

// HostAllowed 是否允许 host 执行。
func HostAllowed() bool { return hostAllowed }

// SetHostAllowed 显式开关 host 执行（agent 命令行 -exec host 使用）。
func SetHostAllowed(v bool) { hostAllowed = v }

const maxActiveRuns = 3

const maxLogBytes = 512 << 10

// RunJob 队列载荷：执行一次流水线所需的最小信息。
type RunJob struct {
	RunID int64  `json:"run_id"`
	Owner string `json:"owner"`
	Repo  string `json:"repo"`
	SHA   string `json:"sha"`
	Ref   string `json:"ref"`
	Event string `json:"event,omitempty"` // push | manual（条件步骤 when 依据）
}

var (
	boundStore *store.Store
	boundQueue queue.Queue
)

// Bind 绑定任务队列消费者与执行器。q 为 nil 时沿用进程内 goroutine 直接调度（默认，零依赖）；
// exec 为 nil 时使用内置本地 docker 执行器。
// 队列模式（如 asynq/redis）下，Trigger 只入队，由 Start 启动的工人真正执行。
func Bind(st *store.Store, q queue.Queue, exec Executor) {
	boundStore, boundQueue = st, q
	if exec != nil {
		boundExecutor = exec
	}
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
	if os.Getenv("GITDASH_PIPELINE_EXEC") == "host" {
		hostAllowed = true
	}
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
// 仓库开启流水线且该提交含 .gitdash.yml 时触发一次运行（分支与 tag push 均可，
// 条件步骤据此可用 when: tag / when: branch）。
func PushHandler(st *store.Store) func(webhooks.Event) {
	return func(ev webhooks.Event) {
		if ev.Event != "push" || ev.New == "" || isZeroSHA(ev.New) {
			return
		}
		if !strings.HasPrefix(ev.Ref, "refs/heads/") && !strings.HasPrefix(ev.Ref, "refs/tags/") {
			return
		}
		if !gitsvc.ValidName(ev.Owner) || !gitsvc.ValidName(ev.Repo) {
			return
		}
		if !st.IsPipelineEnabled(ev.Owner, ev.Repo) {
			return
		}
		ref := ev.Ref // tag 传完整 ref（when: tag 依据完整 ref 前缀判定）
		if strings.HasPrefix(ev.Ref, "refs/heads/") {
			ref = strings.TrimPrefix(ev.Ref, "refs/heads/")
		}
		if _, err := Trigger(st, ev.Owner, ev.Repo, ev.New, ref, ev.User, "push"); err != nil &&
			!errors.Is(err, ErrNoPipeline) && !errors.Is(err, ErrTooManyRuns) {
			log.Printf("pipeline: trigger %s/%s: %v", ev.Owner, ev.Repo, err)
		}
	}
}

// Trigger 创建一次流水线运行（push 或手动）。
// 提交上无 .gitdash.yml 时返回 ErrNoPipeline；DSL 解析错误会记为 failed 的运行，便于排查。
// event 为 "push" 或 "manual"，进入条件步骤（when: event ...）的求值。
func Trigger(st *store.Store, owner, repo, sha, ref, by, event string) (store.PipelineRun, error) {
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
	run, err := st.CreatePipelineRun(owner, repo, sha, ref, by, cfg.UnitCount())
	if err != nil {
		return run, err
	}
	job := RunJob{RunID: run.ID, Owner: owner, Repo: repo, SHA: sha, Ref: ref, Event: event}
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

// executeRun 执行流水线：解析 DSL（Trigger 已校验过），交由绑定的 Executor 执行并写日志。
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
		// 拒绝运行（如挂载 docker socket）记审计日志
		if strings.Contains(perr.Error(), dockerSockPath) {
			log.Printf("pipeline audit: REJECTED docker socket mount repo=%s/%s time=%s", owner, repo, time.Now().UTC().Format(time.RFC3339))
		}
		fail("invalid %s: %v", FileName, perr)
		return
	}
	// 仓库级环境变量在前，DSL 声明的 env 在后（同 key 时后者覆盖前者）
	if repoEnv, err := st.RepoEnvVars(owner, repo); err == nil {
		cfg.Env = append(repoEnv, cfg.Env...)
	} else {
		writeLog("!! load repo env vars: %v", err)
	}

	img := cfg.Image
	if img == "" {
		img = "host"
	}
	writeLog("image: %s  steps: %d", img, cfg.UnitCount())

	exec := boundExecutor
	if exec == nil {
		exec = &dispatchExecutor{}
	}
	if err := exec.Execute(context.Background(), job, cfg, logFile, func(stepsDone int) {
		_ = st.ProgressPipelineRun(runID, stepsDone)
	}); err != nil {
		fail("%v", err)
		return
	}

	writeLog("\n== pipeline success ==")
	_ = st.FinishPipelineRun(runID, "success", "")
}

func isZeroSHA(s string) bool {
	if len(s) != 40 {
		return false
	}
	return strings.Trim(s, "0") == ""
}
