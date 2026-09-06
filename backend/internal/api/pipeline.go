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
		writeErr(w, http.StatusInternalServerError, err.Error())
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
		writeErr(w, http.StatusInternalServerError, err.Error())
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
		writeErr(w, http.StatusInternalServerError, err.Error())
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

// createPipelineRun 手动触发流水线（body: {ref?: branch}）。
//
//	@Summary     手动触发流水线
//	@Tags        pipeline
//	@Accept      json
//	@Produce     json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Param       body  body createPipelineRunReq false "可选分支 ref，默认取默认分支"
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
	var in struct {
		Ref string `json:"ref"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	in.Ref = strings.TrimSpace(in.Ref)
	if in.Ref == "" {
		in.Ref, _ = gitsvc.HeadBranch(owner, name)
	}
	if in.Ref == "" {
		writeCode(w, http.StatusBadRequest, "ref_required", "no branch available")
		return
	}
	sha, err := gitsvc.RevSHA(owner, name, "refs/heads/"+in.Ref)
	if err != nil {
		writeCode(w, http.StatusBadRequest, "ref_not_found", "branch not found: "+in.Ref)
		return
	}
	run, err := pipeline.Trigger(a.store, owner, name, sha, in.Ref, userFrom(r))
	switch {
	case errors.Is(err, pipeline.ErrNoPipeline):
		writeCode(w, http.StatusBadRequest, "pipeline_file_missing",
			"no "+pipeline.FileName+" found at "+in.Ref)
		return
	case errors.Is(err, pipeline.ErrTooManyRuns):
		writeCode(w, http.StatusTooManyRequests, "too_many_runs", "too many active pipeline runs")
		return
	case err != nil:
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, run)
}
