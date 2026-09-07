package pipeline

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"gitdash/backend/internal/gitsvc"
)

// Executor 执行一次完整流水线（多步）：日志写入 logSink，返回 error 即整体失败。
// progress 回调报告已完成步骤数（可为 nil）。
// 现有两种实现：
//   - builtinDockerExecutor：服务端本地 docker（默认，零依赖）
//   - remoteRunnerExecutor：派发给自托管 agent（P3）
type Executor interface {
	Execute(ctx context.Context, job RunJob, cfg *Config, logSink io.Writer, progress func(stepsDone int)) error
}

// boundExecutor 当前绑定的执行器；nil 时默认 builtin 本地 docker。
var boundExecutor Executor

// BuiltinExecutor 返回内置本地 docker 执行器（agent 二进制复用，保证沙箱一致）。
func BuiltinExecutor() Executor { return &builtinDockerExecutor{} }

// builtinDockerExecutor 在本地 docker 中执行：checkout 触发提交，逐步骤在容器中运行。
type builtinDockerExecutor struct{}

// RunInWorkspace 在已检出的工作区 dir 中执行流水线（agent 与 builtin 共用的核心逻辑）。
func RunInWorkspace(ctx context.Context, job RunJob, cfg *Config, dir string, logSink io.Writer, progress func(int)) error {
	be := &builtinDockerExecutor{}
	for i, step := range cfg.Steps {
		fmt.Fprintf(logSink, "\n==> [%d/%d] %s\n", i+1, len(cfg.Steps), step.Name)
		if err := be.runStep(ctx, dir, cfg, step, job.Owner, job.Repo, job.Ref, job.SHA, logSink); err != nil {
			return fmt.Errorf("step %q failed: %w", step.Name, err)
		}
		fmt.Fprintf(logSink, "<== %s ok\n", step.Name)
		if progress != nil {
			progress(i + 1)
		}
	}
	return nil
}

// Execute 实现 Executor：clone → checkout → 逐步执行。
func (e *builtinDockerExecutor) Execute(ctx context.Context, job RunJob, cfg *Config, logSink io.Writer, progress func(int)) error {
	owner, repo, sha := job.Owner, job.Repo, job.SHA

	if err := dockerAvailable(); err != nil {
		return err
	}

	tmp, err := os.MkdirTemp("", "gitdash-run-*")
	if err != nil {
		return fmt.Errorf("create workdir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	// checkout 触发提交到临时工作区
	if out, err := gitsvc.GitOut("", "clone", "--quiet", gitsvc.RepoPath(owner, repo), tmp); err != nil {
		return fmt.Errorf("clone repo: %v: %s", err, strings.TrimSpace(out))
	}
	if out, err := gitsvc.GitOut(tmp, "checkout", "--quiet", "--detach", sha); err != nil {
		return fmt.Errorf("checkout %s: %v: %s", sha, err, strings.TrimSpace(out))
	}
	fmt.Fprintf(logSink, "workspace: checked out %s\n", sha)

	return RunInWorkspace(ctx, job, cfg, tmp, logSink, progress)
}

// runStep 在 docker 容器里执行单步：工作区挂载到 /workspace，输出实时写入日志。
func (e *builtinDockerExecutor) runStep(parent context.Context, workdir string, cfg *Config, step Step, owner, repo, ref, sha string, logSink io.Writer) error {
	ctx, cancel := context.WithTimeout(parent, cfg.Timeout)
	defer cancel()

	// 网络开关：默认 none（沙箱）；GITDASH_PIPELINE_NETWORK 显式指定时用该值
	network := strings.TrimSpace(os.Getenv("GITDASH_PIPELINE_NETWORK"))
	if network == "" {
		network = "none"
	}
	args := []string{
		"run", "--rm",
		"--workdir", "/workspace",
		"-v", workdir + ":/workspace",
		// 沙箱加固：默认禁外网（依赖拉取需镜像内预装或镜像自身可达源）、
		// 限制资源、禁止提权、丢弃 capabilities，防止流水线脚本攻击宿主或同级容器。
		"--network", network,
		"--memory", "512m",
		"--memory-swap", "512m",
		"--cpus", "1.0",
		"--pids-limit", "128",
		"--security-opt", "no-new-privileges",
		"--cap-drop", "ALL",
		"-e", "CI=1",
		"-e", "GITDASH=true",
		"-e", "GITDASH_REPO=" + owner + "/" + repo,
		"-e", "GITDASH_REF=" + ref,
		"-e", "GITDASH_SHA=" + sha,
	}
	for _, ev := range cfg.Env {
		args = append(args, "-e", ev)
	}
	for _, m := range cfg.Volumes { // DSL 声明的额外挂载卷（docker.sock 已在 validate 拒绝）
		args = append(args, "-v", m)
	}
	args = append(args, cfg.Image, "sh", "-ec", step.Run)

	// 镜像拉取/运行审计日志
	log.Printf("pipeline audit: image=%s repo=%s/%s step=%q time=%s",
		cfg.Image, owner, repo, step.Name, time.Now().UTC().Format(time.RFC3339))
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdout = logSink
	cmd.Stderr = logSink
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
