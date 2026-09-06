package api

import (
	"errors"
	"gitdash/backend/internal/store"
	"net/http"
	"strconv"
	"strings"
)

// listTokens 列出当前用户的个人访问令牌。
//
//	@Summary     列出个人访问令牌
//	@Description 返回当前用户创建的所有 PAT（不含明文 token）。
//	@Tags        tokens
//	@Produce     json
//	@Security    BearerAuth
//	@Success     200 {array}  store.PAT
//	@Failure     401 {object} map[string]string
//	@Failure     500 {object} map[string]string
//	@Router      /tokens [get]
func (a *API) listTokens(w http.ResponseWriter, r *http.Request) {
	uid, err := a.store.UserID(userFrom(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	pats, err := a.store.ListPATs(uid)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, pats)
}

// createTokens 创建个人访问令牌；明文 token 仅此一次返回。
//
//	@Summary     创建个人访问令牌
//	@Description 校验 name/scopes 后生成新 PAT，明文 token 只在本次响应中出现。
//	@Tags        tokens
//	@Accept      json
//	@Produce     json
//	@Security    BearerAuth
//	@Param       body body createPATReq true "名称与授权范围"
//	@Success     201 {object} store.CreatedPAT
//	@Failure     400 {object} map[string]string
//	@Failure     401 {object} map[string]string
//	@Failure     500 {object} map[string]string
//	@Router      /tokens [post]
func (a *API) createTokens(w http.ResponseWriter, r *http.Request) {
	var in createPATReq
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		writeCode(w, http.StatusBadRequest, "pat_name_required", "name is required")
		return
	}
	if len(in.Name) > 100 {
		writeCode(w, http.StatusBadRequest, "pat_name_too_long", "name must be at most 100 characters")
		return
	}
	scopeStr, ok := store.NormalizePATScopes(in.Scopes)
	if !ok {
		writeCode(w, http.StatusBadRequest, "invalid_scope", "scopes must be repo/inbox/keys")
		return
	}
	uid, err := a.store.UserID(userFrom(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	token, pat, err := a.store.CreatePAT(uid, in.Name, scopeStr)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, store.CreatedPAT{Token: token, PAT: pat})
}

// deleteToken 删除本人指定的个人访问令牌；他人或不存在返回 404。
//
//	@Summary     删除个人访问令牌
//	@Description 按 id 删除当前用户的 PAT，令牌立即失效。返回 204。
//	@Tags        tokens
//	@Produce     json
//	@Security    BearerAuth
//	@Param       id path int true "PAT id"
//	@Success     204 {object} nil
//	@Failure     400 {object} map[string]string
//	@Failure     401 {object} map[string]string
//	@Failure     404 {object} map[string]string
//	@Router      /tokens/{id} [delete]
func (a *API) deleteToken(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_id", "invalid id")
		return
	}
	uid, err := a.store.UserID(userFrom(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if errors.Is(a.store.DeletePAT(uid, id), store.ErrNotFound) {
		writeNotFound(w, "token")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
