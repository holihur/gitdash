package api

import (
	"errors"
	"net/http"
	"strings"

	"gitdash/backend/internal/store"
)

// listRepoEnvVars 列出仓库级流水线环境变量（仅 owner）。
//
//	@Summary     列出仓库环境变量
//	@Tags        pipeline
//	@Produce     json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Success     200 {array}  object
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/env [get]
func (a *API) listRepoEnvVars(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireOwner(w, r)
	if !ok {
		return
	}
	vars, err := a.store.ListRepoEnvVars(owner, name)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, vars)
}

// setRepoEnvVar 新增或覆盖一个仓库环境变量（仅 owner），返回更新后的完整列表。
//
//	@Summary     设置仓库环境变量
//	@Tags        pipeline
//	@Accept      json
//	@Produce     json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Param       body  body setRepoEnvVarReq true "key 与 value"
//	@Success     200 {array}  object
//	@Failure     400 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/env [put]
func (a *API) setRepoEnvVar(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireOwner(w, r)
	if !ok {
		return
	}
	var in setRepoEnvVarReq
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	in.Key = strings.TrimSpace(in.Key)
	if err := a.store.SetRepoEnvVar(owner, name, in.Key, in.Value); err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_env_var", err.Error())
		return
	}
	vars, err := a.store.ListRepoEnvVars(owner, name)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, vars)
}

// deleteRepoEnvVar 删除一个仓库环境变量（仅 owner）。
//
//	@Summary     删除仓库环境变量
//	@Tags        pipeline
//	@Produce     json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Param       key   path string true "环境变量名"
//	@Success     200 {object} map[string]any
//	@Failure     404 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/env/{key} [delete]
func (a *API) deleteRepoEnvVar(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireOwner(w, r)
	if !ok {
		return
	}
	if err := a.store.DeleteRepoEnvVar(owner, name, r.PathValue("key")); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeNotFound(w, "env var")
		} else {
			internalError(w, err)
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
}
