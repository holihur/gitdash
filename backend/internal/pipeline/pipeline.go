package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"gitdash/backend/internal/gitsvc"
	"gitdash/backend/internal/logx"
	"gitdash/backend/internal/queue"
	"gitdash/backend/internal/store"
	"gitdash/backend/internal/webhooks"
)

// FileName 仓库根目录中的流水线定义文件（单文件向后兼容）。
const FileName = ".gitdash.yml"

// DirName 多流水线目录：其中每个 *.yml / *.yaml 都是独立的流水线定义。
// 目录与根目录的 .gitdash.yml 可共存，推送时会分别触发对应流水线。
const DirName = ".gitdash"

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
	// ErrTriggerDisabled 该事件未在 .gitdash.yml 的 on 白名单中（静默跳过）。
	ErrTriggerDisabled = errors.New("trigger disabled for this event")
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

// pipelineExts 多流水线目录中可被识别的文件后缀。
func isPipelineFile(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, ".yml") || strings.HasSuffix(lower, ".yaml")
}

// DiscoverFiles 返回某个提交上所有的流水线定义文件路径（排序后）。
// 包含：根目录 .gitdash.yml（向后兼容）与 .gitdash/ 目录下顶层的 *.yml / *.yaml。
// 读取失败或无文件时返回空切片。
func DiscoverFiles(owner, repo, ref string) []string {
	var files []string
	if blob, err := gitsvc.ReadBlob(owner, repo, ref, FileName); err == nil && blob.Encoding == "utf-8" && strings.TrimSpace(blob.Content) != "" {
		files = append(files, FileName)
	}
	if names, err := gitsvc.ListDir(owner, repo, ref, DirName); err == nil {
		for _, n := range names {
			if isPipelineFile(n) {
				files = append(files, DirName+"/"+n)
			}
		}
	}
	sort.Strings(files)
	return files
}

// RunJob 队列载荷：执行一次流水线所需的最小信息。
type RunJob struct {
	RunID int64  `json:"run_id"`
	Owner string `json:"owner"`
	Repo  string `json:"repo"`
	File  string `json:"file,omitempty"` // 流水线定义文件路径；空 = 旧的 .gitdash.yml
	SHA   string `json:"sha"`
	Ref   string `json:"ref"`
	Event string `json:"event,omitempty"` // push | pull_request | schedule | workflow_dispatch | manual（when 依据）
	By    string `json:"by,omitempty"`    // 触发者（webhook pipeline 事件的 actor）
	// Inputs dispatch 传入的键值对（注入为 INPUT_<KEY> 环境变量，优先级低于仓库级/DSL env）。
	Inputs map[string]string `json:"inputs,omitempty"`
}

var (
	boundStore *store.Store
	boundQueue queue.Queue
)

// 进程内（本地 docker / host）运行的可取消上下文登记表：
// CancelRun 据此即时终止正在执行的步骤。
var (
	runCancelsMu sync.Mutex
	runCancels   = map[int64]context.CancelFunc{}
)

func registerRunCancel(id int64, cancel context.CancelFunc) {
	runCancelsMu.Lock()
	if old, ok := runCancels[id]; ok {
		old()
	}
	runCancels[id] = cancel
	runCancelsMu.Unlock()
}

func unregisterRunCancel(id int64) {
	runCancelsMu.Lock()
	delete(runCancels, id)
	runCancelsMu.Unlock()
}

// cancelBuiltinRun 取消进程内运行的执行上下文；存在则返回 true。
func cancelBuiltinRun(id int64) bool {
	runCancelsMu.Lock()
	cancel, ok := runCancels[id]
	runCancelsMu.Unlock()
	if ok {
		cancel()
	}
	return ok
}

// eventPublisher 出站 pipeline webhook 事件发布器（main 注入；nil = 不发布）。
var eventPublisher func(webhooks.Event)

// SetEventPublisher 注入 pipeline webhook 事件发布器。
func SetEventPublisher(fn func(webhooks.Event)) { eventPublisher = fn }

// emitPipelineEvent 发布一条 pipeline 事件（queued / started / success / failed / cancelled）。
func emitPipelineEvent(job RunJob, action string) {
	if eventPublisher == nil {
		return
	}
	eventPublisher(webhooks.Event{
		Event: "pipeline", Owner: job.Owner, Repo: job.Repo,
		Kind: job.File, Action: action, Number: job.RunID, Title: job.File,
		Actor: job.By, Ref: job.Ref, New: job.SHA,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	})
}

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
// 仓库开启流水线且该提交包含流水线定义文件时，为每个匹配的流水线各触发一次运行
// （分支与 tag push 均可，条件步骤据此可用 when: tag / when: branch）。
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
		if _, err := TriggerAll(st, TriggerOpts{Owner: ev.Owner, Repo: ev.Repo, SHA: ev.New, Ref: ref, By: ev.User, Event: "push"}); err != nil &&
			!ignorableTriggerErr(err) {
			logx.Infof("pipeline: trigger %s/%s: %v", ev.Owner, ev.Repo, err)
		}
		// 分支 push：若该分支是某个 open PR 的源分支，按 pull_request 事件再触发一次（synchronize）
		if branch, ok := strings.CutPrefix(ev.Ref, "refs/heads/"); ok {
			TriggerOpenPRsForBranch(st, ev.Owner, ev.Repo, branch, ev.New, ev.User)
		}
	}
}

// ignorableTriggerErr 常见的“静默跳过”错误（不记服务端错误日志）。
func ignorableTriggerErr(err error) bool {
	return errors.Is(err, ErrNoPipeline) || errors.Is(err, ErrTooManyRuns) || errors.Is(err, ErrTriggerDisabled)
}

// TriggerOpts 触发一次流水线运行的参数。
type TriggerOpts struct {
	Owner string
	Repo  string
	// File 流水线定义文件路径；为空时表示遗留单文件 .gitdash.yml。
	File  string
	SHA   string
	Ref   string // 完整 ref（refs/heads/x）或短分支名
	By    string // 触发者（用户 / schedule / dispatch actor）
	Event string // push | pull_request | schedule | workflow_dispatch | manual
	// Delay > 0 时延迟执行：创建 pending 运行并记录 run_at，由延迟调度器到期后派发。
	Delay time.Duration
	// Inputs 仅 workflow_dispatch 使用，注入为 INPUT_<KEY> 环境变量。
	Inputs map[string]string
	// Force 跳过 on 白名单校验（手动触发与重跑用）。
	Force bool
}

// pipelineFile 归一化流水线文件路径（空 → 遗留 .gitdash.yml）。
func pipelineFile(file string) string {
	if strings.TrimSpace(file) == "" {
		return FileName
	}
	return file
}

// Trigger 创建一次流水线运行（单文件）。
// opts.File 为空时按遗留 .gitdash.yml 处理。
// 目标文件不存在时返回 ErrNoPipeline；事件未被 on 白名单启用时返回 ErrTriggerDisabled；
// 同一文件进行中运行达上限时返回 ErrTooManyRuns。DSL 解析错误会记为 failed 的运行，便于排查。
func Trigger(st *store.Store, opts TriggerOpts) (store.PipelineRun, error) {
	file := pipelineFile(opts.File)
	if active, err := st.RunningPipelineRunIDs(opts.Owner, opts.Repo, file); err == nil && len(active) >= maxActiveRuns {
		return store.PipelineRun{}, ErrTooManyRuns
	}
	blob, err := gitsvc.ReadBlob(opts.Owner, opts.Repo, opts.SHA, file)
	if err != nil || blob.Encoding != "utf-8" || strings.TrimSpace(blob.Content) == "" {
		return store.PipelineRun{}, ErrNoPipeline
	}
	cfg, perr := Parse([]byte(blob.Content))
	if perr != nil {
		run, cerr := st.CreatePipelineRun(opts.Owner, opts.Repo, file, opts.SHA, opts.Ref, opts.By, opts.Event, opts.Inputs, 0)
		if cerr != nil {
			return run, cerr
		}
		msg := "invalid " + file + ": " + perr.Error()
		_ = st.FinishPipelineRun(run.ID, "failed", msg)
		run.Status = "failed"
		run.Error = msg
		return run, nil
	}
	if !opts.Force && !cfg.Triggers(opts.Event) {
		return store.PipelineRun{}, ErrTriggerDisabled
	}
	run, err := st.CreatePipelineRun(opts.Owner, opts.Repo, file, opts.SHA, opts.Ref, opts.By, opts.Event, opts.Inputs, cfg.UnitCount())
	if err != nil {
		return run, err
	}
	job := RunJob{RunID: run.ID, Owner: opts.Owner, Repo: opts.Repo, File: file, SHA: opts.SHA, Ref: opts.Ref, Event: opts.Event, By: opts.By, Inputs: opts.Inputs}
	emitPipelineEvent(job, "queued")
	// 延迟执行：仅落库 run_at，由 StartDelayedRunner 到期后派发。
	if opts.Delay > 0 {
		runAt := time.Now().UTC().Add(opts.Delay).Format(time.RFC3339)
		if err := st.SetPipelineRunRunAt(run.ID, runAt); err != nil {
			return run, err
		}
		run.RunAt = runAt
		return run, nil
	}
	if err := dispatchJob(st, job); err != nil {
		msg := "enqueue: " + err.Error()
		_ = st.FinishPipelineRun(run.ID, "failed", msg)
		run.Status = "failed"
		run.Error = msg
		// 入队失败已记为 failed 运行，调用方应拿到该运行（API 返回 201）。
		return run, nil //nolint:nilerr
	}
	return run, nil
}

// dispatchJob 派发一次已创建的运行：队列模式入队，否则进程内直接执行。
func dispatchJob(st *store.Store, job RunJob) error {
	if boundQueue == nil {
		go executeRun(st, job)
		return nil
	}
	payload, err := json.Marshal(job)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	qj := queue.Job{
		Kind:    KindPipelineRun,
		ID:      fmt.Sprintf("%s-%s-%d", job.Owner, job.Repo, job.RunID),
		Payload: payload,
	}
	return boundQueue.Enqueue(ctx, qj)
}

// TriggerAll 为某提交上所有匹配的流水线文件各创建一次运行。
// opts.File 非空时仅触发该文件。
// 返回已创建/已失败的运行列表；提交上没有任何流水线文件时返回 ErrNoPipeline，
// 有文件但没有任何文件启用该事件时返回 ErrTriggerDisabled。
func TriggerAll(st *store.Store, opts TriggerOpts) ([]store.PipelineRun, error) {
	files := []string{pipelineFile(opts.File)}
	if strings.TrimSpace(opts.File) == "" {
		files = DiscoverFiles(opts.Owner, opts.Repo, opts.SHA)
	}
	if len(files) == 0 {
		return nil, ErrNoPipeline
	}
	runs := make([]store.PipelineRun, 0, len(files))
	enabled := false
	throttled := false
	for _, f := range files {
		one := opts
		one.File = f
		run, err := Trigger(st, one)
		switch {
		case err == nil:
			runs = append(runs, run)
			enabled = true
		case errors.Is(err, ErrTooManyRuns):
			// 同一文件并发受限：跳过
			throttled = true
		case errors.Is(err, ErrTriggerDisabled):
			// 未启用该事件：跳过
		case errors.Is(err, ErrNoPipeline):
			// 发现与实际读取之间的竞态：忽略
		default:
			return runs, err
		}
	}
	if !enabled && len(runs) == 0 {
		if throttled {
			return runs, ErrTooManyRuns
		}
		return runs, ErrTriggerDisabled
	}
	return runs, nil
}

// InputEnv 把 dispatch inputs 转成 INPUT_<KEY> 环境变量（键大写、非字母数字转下划线）。
func InputEnv(inputs map[string]string) []string {
	if len(inputs) == 0 {
		return nil
	}
	keys := make([]string, 0, len(inputs))
	for k := range inputs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(inputs))
	for _, k := range keys {
		out = append(out, "INPUT_"+inputEnvKey(k)+"="+inputs[k])
	}
	return out
}

func inputEnvKey(k string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(k) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}

// executeRun 执行流水线：解析 DSL（Trigger 已校验过），交由绑定的 Executor 执行并写日志。
func executeRun(st *store.Store, job RunJob) {
	runID, owner, repo, sha, ref := job.RunID, job.Owner, job.Repo, job.SHA, job.Ref
	file := pipelineFile(job.File)
	// 原子认领（pending → running）：已在别处启动或已被取消时直接跳过。
	if started, err := st.ClaimPipelineRun(runID); err != nil || !started {
		return
	}
	emitPipelineEvent(job, "started")

	// 可取消上下文：CancelRun 通过登记表即时终止本地执行的步骤。
	ctx, cancel := context.WithCancel(context.Background())
	registerRunCancel(runID, cancel)
	defer func() { unregisterRunCancel(runID); cancel() }()

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
		emitPipelineEvent(job, "failed")
	}

	writeLog("== gitdash pipeline run %d ==", runID)
	writeLog("repo: %s/%s  file: %s  ref: %s  sha: %s", owner, repo, file, ref, sha)

	// 执行时重新读取并解析 DSL（Trigger 已校验过；此处失败则直接记 failed）
	blob, err := gitsvc.ReadBlob(owner, repo, sha, file)
	if err != nil || blob.Encoding != "utf-8" || strings.TrimSpace(blob.Content) == "" {
		fail("pipeline file %s not found at %s", file, sha)
		return
	}
	cfg, perr := Parse([]byte(blob.Content))
	if perr != nil {
		// 拒绝运行（如挂载宿主路径/卷）记审计日志
		if strings.Contains(perr.Error(), "volume") {
			logx.Infof("pipeline audit: REJECTED docker socket mount repo=%s/%s time=%s", owner, repo, time.Now().UTC().Format(time.RFC3339))
		}
		fail("invalid %s: %v", file, perr)
		return
	}
	// 环境变量优先级（低→高）：dispatch inputs < 仓库级环境变量 < DSL env
	if repoEnv, err := st.RepoEnvVars(owner, repo); err == nil {
		cfg.Env = append(repoEnv, cfg.Env...)
	} else {
		writeLog("!! load repo env vars: %v", err)
	}
	if len(job.Inputs) > 0 {
		cfg.Env = append(InputEnv(job.Inputs), cfg.Env...)
	}
	// CI secrets：按 DSL 白名单解析并注入（优先级最低，DSL/repo env 可覆盖同名变量）。
	var secretValues []string
	if len(cfg.Secrets) > 0 {
		vals, serr := st.RepoSecretValues(owner, repo, cfg.Secrets)
		if serr != nil {
			writeLog("!! load secrets: %v", serr)
		} else {
			env := make([]string, 0, len(vals))
			for name, v := range vals {
				env = append(env, name+"="+v)
				secretValues = append(secretValues, v)
			}
			sort.Strings(env)
			cfg.Env = append(env, cfg.Env...)
			writeLog("secrets: %d injected", len(vals))
		}
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
	// 整次运行超时（job_timeout）：在取消上下文之上再包一层超时。
	execCtx := ctx
	if cfg.JobTimeout > 0 {
		writeLog("job_timeout: %s", cfg.JobTimeout)
		var timeoutCancel context.CancelFunc
		execCtx, timeoutCancel = context.WithTimeout(ctx, cfg.JobTimeout)
		defer timeoutCancel()
	}
	if err := exec.Execute(execCtx, job, cfg, newMaskingWriter(logFile, secretValues), func(stepsDone int) {
		_ = st.ProgressPipelineRun(runID, stepsDone)
	}); err != nil {
		if errors.Is(execCtx.Err(), context.DeadlineExceeded) {
			fail("job timeout exceeded (%s)", cfg.JobTimeout)
			return
		}
		if execCtx.Err() != nil {
			writeLog("\n== cancelled ==")
			_ = st.FinishPipelineRun(runID, "cancelled", "cancelled by user")
			emitPipelineEvent(job, "cancelled")
			return
		}
		fail("%v", err)
		return
	}

	writeLog("\n== pipeline success ==")
	_ = st.FinishPipelineRun(runID, "success", "")
	emitPipelineEvent(job, "success")
}

func isZeroSHA(s string) bool {
	if len(s) != 40 {
		return false
	}
	return strings.Trim(s, "0") == ""
}
