package api

import (
	"gitdash/backend/internal/jobs"
	"net/http"
	"strings"
)

// ---- push mirror ----

// getMirror 查看仓库推送镜像配置。
//
//	@Summary     查看镜像配置
//	@Tags        repos
//	@Produce     json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Success     200 {object} map[string]any "url 与 created_at"
//	@Failure     500 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/mirror [get]
func (a *API) getMirror(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, false)
	if !ok {
		return
	}
	m, err := a.store.GetMirror(owner, name)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"url":        m.URL,
		"status":     m.Status,
		"created_at": m.CreatedAt,
	})
}

// setMirror 设置推送镜像目标。
//
//	@Summary     设置镜像
//	@Tags        repos
//	@Accept      json
//	@Produce     json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Param       body  body setMirrorReq true "url 与 private_key（可选）"
//	@Success     200 {object} map[string]any "url、status 与 created_at"
//	@Failure     400 {object} map[string]string
//	@Failure     500 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/mirror [put]
func (a *API) setMirror(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireOwner(w, r)
	if !ok {
		return
	}
	var in setMirrorReq
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	raw, err := validImportURL(in.URL)
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_url", "URL must be a valid http(s)/ssh/git repository URL")
		return
	}
	if err := a.store.SetMirror(owner, name, raw, strings.TrimSpace(in.PrivateKey)); err != nil {
		internalError(w, err)
		return
	}
	_ = a.store.SetMirrorStatus(owner, name, "", "") // 换目标后重置状态
	m, err := a.store.GetMirror(owner, name)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"url":        m.URL,
		"status":     m.Status,
		"created_at": m.CreatedAt,
	})
}

// deleteMirror 删除推送镜像配置。
//
//	@Summary     删除镜像
//	@Tags        repos
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Success     204 {string} string ""
//	@Failure     500 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/mirror [delete]
func (a *API) deleteMirror(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireOwner(w, r)
	if !ok {
		return
	}
	if err := a.store.DeleteMirror(owner, name); err != nil {
		internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// syncMirror 立即触发一次镜像推送（异步队列）。
//
//	@Summary     同步镜像
//	@Tags        repos
//	@Produce     json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Success     202 {object} map[string]any "status=queued"
//	@Failure     400 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/mirror/sync [post]
func (a *API) syncMirror(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireOwner(w, r)
	if !ok {
		return
	}
	m, err := a.store.GetMirror(owner, name)
	if err != nil {
		internalError(w, err)
		return
	}
	if m.URL == "" {
		writeCode(w, http.StatusBadRequest, "mirror_not_configured", "no mirror target configured")
		return
	}
	// 异步同步：排队后立即返回，通过 GET mirror 的 status 轮询进度
	if err := a.store.SetMirrorStatus(owner, name, jobs.StatusQueued, ""); err != nil {
		internalError(w, err)
		return
	}
	if err := jobs.EnqueueMirror(owner, name, m.URL, m.PrivateKey); err != nil {
		_ = a.store.SetMirrorStatus(owner, name, jobs.StatusFailed, "enqueue: "+err.Error())
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"status": jobs.StatusQueued})
}

// deleteRepo 删除仓库。
//
//	@Summary     删除仓库
//	@Description 仅仓库所有者可删除。
//	@Tags        repos
//	@Param       owner path string false "仓库所有者（简写路由时省略）"
//	@Param       name  path string true  "仓库名"
//	@Success     204 {string} string ""
//	@Failure     404 {object} map[string]string
//	@Failure     500 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /repos/{name} [delete]
//	@Router      /users/{owner}/repos/{name} [delete]
