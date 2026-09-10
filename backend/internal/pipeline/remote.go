// 远程执行：runs-on 指定标签时，把流水线派发给匹配的在线 agent。
// server 端用 git archive 生成快照流（无需在 agent 侧下发任何 git 凭证）。
package pipeline

import (
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os/exec"
	"strings"
	"time"

	"gitdash/backend/internal/gitsvc"
	"gitdash/backend/internal/runner"
	"gitdash/backend/internal/store"
)

// RemoteHub 远程调度所需能力（由 runner.Hub 实现，main 注入）。
type RemoteHub interface {
	SelectRunner(labels []string, scopes []string) (store.Runner, bool)
	RunRemote(ctx context.Context, runnerName string, job runner.Job, workspace io.Reader, logSink io.Writer, progress func(int)) error
	Cancel(runID int64)
}

var boundHub RemoteHub

// BindHub 注入远程调度 Hub（nil = 禁用远程执行，runs-on 一律失败）。
func BindHub(h RemoteHub) { boundHub = h }

// dispatchExecutor 默认执行器：runs-on 为空走 builtin 本地 docker，否则走远程 agent。
type dispatchExecutor struct{}

// CancelRun 取消远程流水线运行（仅 remote run 支持；builtin run 不支持）。
func CancelRun(st *store.Store, owner, repo string, id int64) error {
	run, err := st.GetPipelineRun(owner, repo, id)
	if err != nil {
		return err
	}
	if run.Status != "running" && run.Status != "pending" {
		return errors.New("run already finished")
	}
	if run.RunnerName == "" {
		return errors.New("cancel not supported for builtin runs")
	}
	if boundHub != nil {
		boundHub.Cancel(id)
	}
	return nil
}

// Execute 实现 Executor。
func (d *dispatchExecutor) Execute(ctx context.Context, job RunJob, cfg *Config, logSink io.Writer, progress func(int)) error {
	if len(cfg.RunsOn) == 0 || boundHub == nil {
		if len(cfg.RunsOn) > 0 {
			return errors.New("remote runner support is disabled on this server")
		}
		return BuiltinExecutor().Execute(ctx, job, cfg, logSink, progress)
	}

	owner := job.Owner
	scopes := []string{"", "user:" + owner}
	if boundStore != nil && boundStore.IsOrg(owner) {
		scopes = []string{"", "org:" + owner}
	}
	r, ok := boundHub.SelectRunner(cfg.RunsOn, scopes)
	if !ok {
		return fmt.Errorf("no online runner matches labels [%s]", strings.Join(cfg.RunsOn, ", "))
	}
	if boundStore != nil {
		_ = boundStore.SetPipelineRunRunner(job.RunID, r.Name)
	}

	// 重新读取 DSL 原文（agent 端自行解析）
	dsl := ""
	if boundStore != nil {
		if blob, err := gitsvc.ReadBlob(job.Owner, job.Repo, job.SHA, FileName); err == nil && blob.Encoding == "utf-8" {
			dsl = blob.Content
		}
	}
	// 仓库级环境变量 + dispatch inputs 随任务下发（agent 端解析 DSL 后合并，DSL env 优先）
	var repoEnv []string
	if boundStore != nil {
		repoEnv, _ = boundStore.RepoEnvVars(job.Owner, job.Repo)
	}
	repoEnv = append(InputEnv(job.Inputs), repoEnv...)
	rjob := runner.Job{
		JobID: fmt.Sprintf("%s-%s-%d", job.Owner, job.Repo, job.RunID),
		RunID: job.RunID, Owner: job.Owner, Repo: job.Repo, SHA: job.SHA, Ref: job.Ref, Event: job.Event, DSL: dsl, Env: repoEnv,
	}

	workspace, done, err := workspaceSnapshot(job.Owner, job.Repo, job.SHA)
	if err != nil {
		return fmt.Errorf("workspace snapshot: %w", err)
	}
	defer done()

	log.Printf("pipeline audit: REMOTE runner=%s image=%s repo=%s/%s time=%s",
		r.Name, cfg.Image, job.Owner, job.Repo, time.Now().UTC().Format(time.RFC3339))
	return boundHub.RunRemote(ctx, r.Name, rjob, workspace, logSink, progress)
}

// workspaceSnapshot 生成提交快照（tar.gz 流）：git archive | gzip。
// 返回读端与清理函数；流读完或清理被调用后回收子进程。
func workspaceSnapshot(owner, repo, sha string) (io.Reader, func(), error) {
	cmd := exec.Command("git", "-C", gitsvc.RepoPath(owner, repo), "archive", "--format=tar", sha)
	pr, pw := io.Pipe()
	gw := gzip.NewWriter(pw)
	cmd.Stdout = gw
	cmd.Stderr = io.Discard
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
