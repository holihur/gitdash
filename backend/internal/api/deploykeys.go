package api

import (
	"bytes"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"golang.org/x/crypto/ssh"

	"gitdash/backend/internal/store"
)

// 仓库部署密钥（deploy key）：绑定单个仓库的 SSH 公钥，权限为 read 或 write。
// 管理接口仅仓库 owner 可用；SSH 网关据此把访问限制到绑定仓库（见 authz/sshserver）。

// listDeployKeys 列出仓库部署密钥（仅 owner）。
//
//	@Summary     列出部署密钥
//	@Tags        repos
//	@Produce     json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Success     200 {array} store.DeployKey
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/deploy-keys [get]
func (a *API) listDeployKeys(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireOwner(w, r)
	if !ok {
		return
	}
	keys, err := a.store.ListDeployKeys(owner, name)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, keys)
}

// createDeployKey 新增部署密钥（仅 owner）。
//
//	@Summary     新增部署密钥
//	@Tags        repos
//	@Accept      json
//	@Produce     json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Param       body  body object true "title/key/read_only"
//	@Success     201 {object} store.DeployKey
//	@Failure     400 {object} map[string]string
//	@Failure     409 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/deploy-keys [post]
func (a *API) createDeployKey(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireOwner(w, r)
	if !ok {
		return
	}
	var in struct {
		Title    string `json:"title"`
		Key      string `json:"key"`
		ReadOnly bool   `json:"read_only"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	in.Title = strings.TrimSpace(in.Title)
	pub := strings.TrimSpace(in.Key)
	if in.Title == "" || pub == "" {
		writeCode(w, http.StatusBadRequest, "deploy_key_invalid", "title and key are required")
		return
	}
	if tooLong(w, "title", in.Title, maxNameRunes) {
		return
	}
	parsed, _, _, _, err := ssh.ParseAuthorizedKey([]byte(pub))
	if err != nil {
		writeCode(w, http.StatusBadRequest, "deploy_key_invalid", "invalid public key: "+err.Error())
		return
	}
	pub = strings.Join(strings.Fields(string(bytes.TrimSpace(ssh.MarshalAuthorizedKey(parsed)))), " ")
	fp := ssh.FingerprintSHA256(parsed)
	permission := store.DeployPermissionWrite
	if in.ReadOnly {
		permission = store.DeployPermissionRead
	}
	key, err := a.store.CreateDeployKey(owner, name, in.Title, pub, fp, permission)
	if errors.Is(err, store.ErrExists) {
		writeCode(w, http.StatusConflict, "deploy_key_exists", "this key is already registered")
		return
	}
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, key)
}

// deleteDeployKey 删除部署密钥（仅 owner）。
//
//	@Summary     删除部署密钥
//	@Tags        repos
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Param       id    path int    true "部署密钥 ID"
//	@Success     204 {object} nil
//	@Failure     404 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/deploy-keys/{id} [delete]
func (a *API) deleteDeployKey(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireOwner(w, r)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeCode(w, http.StatusBadRequest, "invalid_id", "invalid id")
		return
	}
	if errors.Is(a.store.DeleteDeployKey(owner, name, id), store.ErrNotFound) {
		writeNotFound(w, "deploy key")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
