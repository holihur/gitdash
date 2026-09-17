package api

import (
	"errors"
	"net/http"
	"strings"

	"gitdash/backend/internal/store"
)

// listRepoSecrets 列出仓库 CI secrets 元信息（仅 owner，永不返回明文值）。
//
//	@Summary     列出仓库 secrets
//	@Tags        pipeline
//	@Produce     json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Success     200 {array}  object
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/secrets [get]
func (a *API) listRepoSecrets(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireOwner(w, r)
	if !ok {
		return
	}
	secrets, err := a.store.ListRepoSecrets(owner, name)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, secrets)
}

// setRepoSecret 新增或覆盖一个仓库 secret（仅 owner），返回更新后的完整列表。
//
//	@Summary     设置仓库 secret
//	@Tags        pipeline
//	@Accept      json
//	@Produce     json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Param       body  body setRepoSecretReq true "name 与 value"
//	@Success     200 {array}  object
//	@Failure     400 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/secrets [put]
func (a *API) setRepoSecret(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireOwner(w, r)
	if !ok {
		return
	}
	var in setRepoSecretReq
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	if err := a.store.SetRepoSecret(owner, name, strings.TrimSpace(in.Name), in.Value); err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_secret", err.Error())
		return
	}
	secrets, err := a.store.ListRepoSecrets(owner, name)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, secrets)
}

// deleteRepoSecret 删除一个仓库 secret（仅 owner）。
//
//	@Summary     删除仓库 secret
//	@Tags        pipeline
//	@Produce     json
//	@Param       owner  path string true "仓库所有者"
//	@Param       name   path string true "仓库名"
//	@Param       secret path string true "secret 名"
//	@Success     200 {object} map[string]any
//	@Failure     404 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/secrets/{secret} [delete]
func (a *API) deleteRepoSecret(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireOwner(w, r)
	if !ok {
		return
	}
	if err := a.store.DeleteRepoSecret(owner, name, r.PathValue("secret")); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeNotFound(w, "secret")
		} else {
			internalError(w, err)
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
}
