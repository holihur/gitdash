package api

import (
	"errors"
	"gitdash/backend/internal/gpgsig"
	"gitdash/backend/internal/store"
	"net/http"
	"strconv"
	"strings"
)

func (a *API) listGPGKeys(w http.ResponseWriter, r *http.Request) {
	keys, err := a.store.ListGPGKeys(userFrom(r))
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, keys)
}

// addGPGKey 添加 GPG 公钥。
//
//	@Summary     添加 GPG 公钥
//	@Description 解析 armored 公钥并注册。返回 201 与新键。
//	@Tags        users
//	@Accept      json
//	@Produce     json
//	@Security    BearerAuth
//	@Param       body body addGPGKeyReq true "armored 公钥"
//	@Success     201 {object} store.GPGKey
//	@Failure     400 {object} map[string]string
//	@Failure     401 {object} map[string]string
//	@Failure     409 {object} map[string]string
//	@Router      /gpg [post]
func (a *API) addGPGKey(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Armor string `json:"armor"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	armor := strings.TrimSpace(in.Armor)
	if armor == "" {
		writeCode(w, http.StatusBadRequest, "gpg_key_required", "armored public key is required")
		return
	}
	fp, err := gpgsig.ParseArmoredKey(armor)
	if err != nil {
		writeCode(w, http.StatusBadRequest, "gpg_key_invalid", err.Error())
		return
	}
	k, err := a.store.AddGPGKey(userFrom(r), fp, armor)
	a.invalidateGPGKeys()
	if errors.Is(err, store.ErrExists) {
		writeCode(w, http.StatusConflict, "gpg_key_exists", "this gpg key is already registered")
		return
	}
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, k)
}

// deleteGPGKey 删除指定 GPG 公钥。
//
//	@Summary     删除 GPG 公钥
//	@Description 按 id 删除当前用户的 GPG 公钥。返回 204。
//	@Tags        users
//	@Produce     json
//	@Security    BearerAuth
//	@Param       id path int true "GPG key id"
//	@Success     204 {object} nil
//	@Failure     400 {object} map[string]string
//	@Failure     401 {object} map[string]string
//	@Failure     404 {object} map[string]string
//	@Router      /gpg/{id} [delete]
func (a *API) deleteGPGKey(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_id", "invalid id")
		return
	}
	if errors.Is(a.store.DeleteGPGKey(userFrom(r), id), store.ErrNotFound) {
		writeCode(w, http.StatusNotFound, "gpg_key_not_found", "gpg key not found")
		return
	}
	a.invalidateGPGKeys()
	w.WriteHeader(http.StatusNoContent)
}

// oauthIssueSession 为第三方登录用户签发 cookie 会话并跳回前端（不写 JSON）。
