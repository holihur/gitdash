package api

import (
	"bytes"
	"errors"
	"gitdash/backend/internal/store"
	"net/http"
	"strconv"
	"strings"

	"golang.org/x/crypto/ssh"
)

// listKeys 列出当前用户的 SSH 公钥。
//
//	@Summary     列出 SSH 公钥
//	@Description 返回当前用户注册的所有 SSH 公钥。
//	@Tags        users
//	@Produce     json
//	@Security    BearerAuth
//	@Success     200 {array}  store.SSHKey
//	@Failure     401 {object} map[string]string
//	@Failure     500 {object} map[string]string
//	@Router      /keys [get]
func (a *API) listKeys(w http.ResponseWriter, r *http.Request) {
	keys, err := a.store.ListKeys(userFrom(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, keys)
}

// createKey 添加 SSH 公钥。
//
//	@Summary     添加 SSH 公钥
//	@Description 校验并规范化公钥后注册。返回 201 与新键。
//	@Tags        users
//	@Accept      json
//	@Produce     json
//	@Security    BearerAuth
//	@Param       body body createKeyReq true "名称与公钥内容"
//	@Success     201 {object} store.SSHKey
//	@Failure     400 {object} map[string]string
//	@Failure     401 {object} map[string]string
//	@Failure     409 {object} map[string]string
//	@Router      /keys [post]
func (a *API) createKey(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name      string `json:"name"`
		PublicKey string `json:"public_key"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	pub := strings.TrimSpace(in.PublicKey)
	if in.Name == "" || pub == "" {
		writeCode(w, http.StatusBadRequest, "key_name_required", "name and public_key are required")
		return
	}
	key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(pub))
	if err != nil {
		writeCode(w, http.StatusBadRequest, "key_invalid", "invalid public key: "+err.Error())
		return
	}
	pub = strings.Join(strings.Fields(string(bytes.TrimSpace(ssh.MarshalAuthorizedKey(key)))), " ")
	k, err := a.store.CreateKey(userFrom(r), in.Name, pub, ssh.FingerprintSHA256(key))
	if errors.Is(err, store.ErrExists) {
		writeCode(w, http.StatusConflict, "key_exists", "this key is already registered")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, k)
}

// deleteKey 删除指定 SSH 公钥。
//
//	@Summary     删除 SSH 公钥
//	@Description 按 id 删除当前用户的 SSH 公钥。返回 204。
//	@Tags        users
//	@Produce     json
//	@Security    BearerAuth
//	@Param       id path int true "SSH key id"
//	@Success     204 {object} nil
//	@Failure     400 {object} map[string]string
//	@Failure     401 {object} map[string]string
//	@Failure     404 {object} map[string]string
//	@Router      /keys/{id} [delete]
func (a *API) deleteKey(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_id", "invalid id")
		return
	}
	if errors.Is(a.store.DeleteKey(userFrom(r), id), store.ErrNotFound) {
		writeNotFound(w, "ssh key")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
