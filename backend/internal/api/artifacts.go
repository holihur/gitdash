package api

import (
	"fmt"
	"net/http"
	"strconv"

	"gitdash/backend/internal/pipeline"
)

// parseRunID 解析路径参数中的运行 ID。
func parseRunID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeCode(w, http.StatusBadRequest, "invalid_run_id", "invalid run id")
		return 0, false
	}
	return id, true
}

// listRunArtifacts 列出某次流水线运行的归档产物。
//
//	@Summary     列出运行产物
//	@Tags        pipeline
//	@Produce     json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Param       id    path int    true "运行 ID"
//	@Success     200 {array}  object
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/pipeline/runs/{id}/artifacts [get]
func (a *API) listRunArtifacts(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, false)
	if !ok {
		return
	}
	id, ok := parseRunID(w, r)
	if !ok {
		return
	}
	files, err := pipeline.ListArtifacts(owner, name, id)
	if err != nil {
		internalError(w, err)
		return
	}
	if files == nil {
		files = []pipeline.ArtifactFile{}
	}
	writeJSON(w, http.StatusOK, files)
}

// downloadRunArtifacts 下载某次运行的全部归档产物（tar.gz）。
//
//	@Summary     下载运行产物
//	@Tags        pipeline
//	@Produce     application/gzip
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Param       id    path int    true "运行 ID"
//	@Success     200 {file} binary
//	@Failure     404 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/pipeline/runs/{id}/artifacts/download [get]
func (a *API) downloadRunArtifacts(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, false)
	if !ok {
		return
	}
	id, ok := parseRunID(w, r)
	if !ok {
		return
	}
	if !pipeline.HasArtifacts(owner, name, id) {
		writeNotFound(w, "artifacts")
		return
	}
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition",
		fmt.Sprintf("attachment; filename=%s-%s-run-%d-artifacts.tar.gz", owner, name, id))
	// 已开始写响应后无法再改状态码，出错只能中断传输。
	_ = pipeline.WriteArtifactArchive(owner, name, id, w)
}
