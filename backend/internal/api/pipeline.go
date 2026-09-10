package api

import (
	"errors"
	"gitdash/backend/internal/gitsvc"
	"gitdash/backend/internal/pipeline"
	"net/http"
	"strconv"
	"strings"
)

// getPipeline 获取仓库流水线开关状态。
//
//	@Summary     获取流水线配置
//	@Tags        pipeline
//	@Produce     json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Success     200 {object} map[string]any
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/pipeline [get]
func (a *API) getPipeline(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, false)
	if !ok {
		return
	}
	p, err := a.store.GetPipeline(owner, name)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled": p.Enabled,
		"file":    pipeline.FileName,
	})
}

// setPipeline 启用/禁用流水线（仅 owner）。
//
//	@Summary     设置流水线开关
//	@Tags        pipeline
//	@Accept      json
//	@Produce     json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Param       body  body setPipelineReq true "{enabled: bool}"
//	@Success     200 {object} map[string]any
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/pipeline [put]
func (a *API) setPipeline(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireOwner(w, r)
	if !ok {
		return
	}
	var in struct {
		Enabled bool `json:"enabled"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	if err := a.store.SetPipeline(owner, name, in.Enabled); err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"enabled": in.Enabled})
}

// listPipelineRuns 列出流水线运行记录（?limit 限制数量）。
//
//	@Summary     列出流水线运行
//	@Tags        pipeline
//	@Produce     json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Param       limit query int false "数量上限"
//	@Success     200 {array}  object
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/pipeline/runs [get]
func (a *API) listPipelineRuns(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, false)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	runs, err := a.store.ListPipelineRuns(owner, name, limit)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, runs)
}

// getPipelineRun 获取单次流水线运行详情（含日志）。
//
//	@Summary     获取流水线运行详情
//	@Tags        pipeline
//	@Produce     json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Param       id    path int    true "运行 ID"
//	@Success     200 {object} object
//	@Failure     400 {object} map[string]string
//	@Failure     404 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/pipeline/runs/{id} [get]
func (a *API) getPipelineRun(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, false)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeCode(w, http.StatusBadRequest, "invalid_run_id", "invalid run id")
		return
	}
	run, err := a.store.GetPipelineRun(owner, name, id)
	if err != nil {
		writeNotFound(w, "run")
		return
	}
	run.Log, _ = pipeline.ReadLog(owner, name, id)
	writeJSON(w, http.StatusOK, run)
}

// cancelPipelineRun 取消远程 runner 执行中的流水线运行。
//
//	@Summary     取消流水线运行
//	@Tags        pipeline
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Param       id    path int    true "运行 ID"
//	@Success     200 {object} map[string]any
//	@Failure     404 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/pipeline/runs/{id}/cancel [post]
func (a *API) cancelPipelineRun(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, true)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeCode(w, http.StatusBadRequest, "invalid_run_id", "invalid run id")
		return
	}
	if err := pipeline.CancelRun(a.store, owner, name, id); err != nil {
		writeCode(w, http.StatusBadRequest, "cancel_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"cancelled": true})
}

// createPipelineRun 手动触发流水线（body: {ref?, sha?, inputs?}）。ref 可为分支或 tag；
// sha 可直接指定提交（分支名/tag/SHA）；两者都为空时取默认分支。手动触发不受 on 白名单限制。
//
//	@Summary     手动触发流水线
//	@Tags        pipeline
//	@Accept      json
//	@Produce     json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Param       body  body createPipelineRunReq false "可选 ref/sha/inputs"
//	@Success     201 {object} object
//	@Failure     400 {object} map[string]string
//	@Failure     429 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/pipeline/runs [post]
func (a *API) createPipelineRun(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, true)
	if !ok {
		return
	}
	var in createPipelineRunReq
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	a.triggerRun(w, r, owner, name, in, "manual", true)
}

// dispatchPipelineRun 外部 webhook dispatch 触发（需 .gitdash.yml 的 on 含 workflow_dispatch）。
// 供外部系统（CI/机器人）用 PAT 调用；inputs 注入为 INPUT_<KEY> 环境变量。
//
//	@Summary     外部 dispatch 触发流水线
//	@Tags        pipeline
//	@Accept      json
//	@Produce     json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Param       body  body createPipelineRunReq false "可选 ref/sha/inputs"
//	@Success     201 {object} object
//	@Failure     400 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/pipeline/dispatch [post]
func (a *API) dispatchPipelineRun(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, true)
	if !ok {
		return
	}
	var in createPipelineRunReq
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	a.triggerRun(w, r, owner, name, in, "workflow_dispatch", false)
}

// rerunPipelineRun 重跑一次既有运行（复用其提交、ref、事件与 inputs；不受 on 白名单限制）。
//
//	@Summary     重跑流水线运行
//	@Tags        pipeline
//	@Produce     json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Param       id    path int    true "既有运行 ID"
//	@Success     201 {object} object
//	@Failure     404 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/pipeline/runs/{id}/rerun [post]
func (a *API) rerunPipelineRun(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, true)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeCode(w, http.StatusBadRequest, "invalid_run_id", "invalid run id")
		return
	}
	prev, err := a.store.GetPipelineRun(owner, name, id)
	if err != nil {
		writeNotFound(w, "run")
		return
	}
	event := prev.Event
	if event == "" {
		event = "manual"
	}
	run, terr := pipeline.Trigger(a.store, pipeline.TriggerOpts{
		Owner: owner, Repo: name, SHA: prev.SHA, Ref: prev.Ref,
		By: userFrom(r), Event: event, Inputs: prev.Inputs, Force: true,
	})
	if !a.writeTriggerResult(w, terr) {
		return
	}
	writeJSON(w, http.StatusCreated, run)
}

// triggerRun 手动/dispatch 共用的目标解析与触发（event=manual 时 force 跳过 on 白名单）。
func (a *API) triggerRun(w http.ResponseWriter, r *http.Request, owner, name string, in createPipelineRunReq, event string, force bool) {
	resolvedRef, sha, code, msg := resolveRunTarget(owner, name, in.Ref, in.SHA)
	if code != "" {
		writeCode(w, http.StatusBadRequest, code, msg)
		return
	}
	if event == "workflow_dispatch" && !a.pipelineDispatchEnabled(owner, name, sha) {
		writeCode(w, http.StatusBadRequest, "dispatch_not_enabled",
			"add \"workflow_dispatch\" to on: in "+pipeline.FileName+" to enable dispatch")
		return
	}
	var inputs map[string]string
	if event == "workflow_dispatch" {
		inputs = sanitizeInputs(in.Inputs)
	}
	run, err := pipeline.Trigger(a.store, pipeline.TriggerOpts{
		Owner: owner, Repo: name, SHA: sha, Ref: resolvedRef,
		By: userFrom(r), Event: event, Inputs: inputs, Force: force,
	})
	if !a.writeTriggerResult(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, run)
}

// writeTriggerResult 统一处理 Trigger 错误；返回 true 表示可继续写 201。
func (a *API) writeTriggerResult(w http.ResponseWriter, err error) bool {
	switch {
	case err == nil:
		return true
	case errors.Is(err, pipeline.ErrNoPipeline):
		writeCode(w, http.StatusBadRequest, "pipeline_file_missing", "no "+pipeline.FileName+" found at the target commit")
	case errors.Is(err, pipeline.ErrTriggerDisabled):
		writeCode(w, http.StatusBadRequest, "trigger_disabled",
			"this pipeline does not enable the requested trigger (see on: in "+pipeline.FileName+")")
	case errors.Is(err, pipeline.ErrTooManyRuns):
		writeCode(w, http.StatusTooManyRequests, "too_many_runs", "too many active pipeline runs")
	default:
		internalError(w, err)
	}
	return false
}

// pipelineDispatchEnabled 目标提交的 DSL 是否启用了外部 dispatch。
func (a *API) pipelineDispatchEnabled(owner, name, sha string) bool {
	blob, err := gitsvc.ReadBlob(owner, name, sha, pipeline.FileName)
	if err != nil || blob.Encoding != "utf-8" {
		return false
	}
	cfg, err := pipeline.Parse([]byte(blob.Content))
	return err == nil && cfg.DispatchEnabled()
}

// resolveRunTarget 解析触发目标：sha 优先，其次 ref（分支或 tag 短名），都空取默认分支。
func resolveRunTarget(owner, name, ref, sha string) (resolvedRef, resolvedSHA, code, msg string) {
	sha = strings.TrimSpace(sha)
	ref = strings.TrimSpace(ref)
	if sha != "" {
		s, err := gitsvc.RevSHA(owner, name, sha)
		if err != nil || s == "" {
			return "", "", "commit_not_found", "commit not found: " + sha
		}
		if ref == "" {
			ref = sha
		}
		return ref, s, "", ""
	}
	if ref != "" {
		ref = strings.TrimPrefix(strings.TrimPrefix(ref, "refs/heads/"), "refs/tags/")
		if s, err := gitsvc.RevSHA(owner, name, "refs/heads/"+ref); err == nil {
			return ref, s, "", "" // 分支存短名（与 push 触发一致）
		}
		if s, err := gitsvc.RevSHA(owner, name, "refs/tags/"+ref); err == nil {
			return "refs/tags/" + ref, s, "", "" // tag 存完整 ref（when: tag 依据）
		}
		return "", "", "ref_not_found", "branch or tag not found: " + ref
	}
	branch, err := gitsvc.HeadBranch(owner, name)
	if err != nil || branch == "" {
		return "", "", "ref_required", "no branch available"
	}
	s, err := gitsvc.RevSHA(owner, name, "refs/heads/"+branch)
	if err != nil {
		return "", "", "ref_not_found", "branch not found: " + branch
	}
	return branch, s, "", ""
}

// sanitizeInputs 限制 dispatch inputs 的数量与长度，过滤非法键。
func sanitizeInputs(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		if k == "" || len(k) > 64 || len(v) > 4096 {
			continue
		}
		out[k] = v
		if len(out) >= 20 {
			break
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
