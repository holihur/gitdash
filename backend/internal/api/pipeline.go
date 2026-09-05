package api

import (
	"errors"
	"gitdash/backend/internal/gitsvc"
	"gitdash/backend/internal/pipeline"
	"net/http"
	"strconv"
	"strings"
)

// GET /api/users/{owner}/repos/{name}/pipeline
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

// PUT /api/users/{owner}/repos/{name}/pipeline
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

// GET /api/users/{owner}/repos/{name}/pipeline/runs
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

// GET /api/users/{owner}/repos/{name}/pipeline/runs/{id}
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

// POST /api/users/{owner}/repos/{name}/pipeline/runs  手动触发（body: {ref?: branch}）
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
