package api

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"gitdash/backend/internal/runner"
	"gitdash/backend/internal/store"
)

// SetRunnerHub 注入 runner Hub。
func (a *API) SetRunnerHub(h *runner.Hub) { a.runnerHub = h }

// ---- 注册 token 签发 ----

// createRunnerToken 签发 runner 注册 token。
//
//	@Summary     签发 runner 注册 token
//	@Tags        runners
//	@Accept      json
//	@Produce     json
//	@Param       body body createRunnerTokenReq true "scope: user|org（org 需 body.org）"
//	@Success     201 {object} store.RunnerToken
//	@Security    BearerAuth
//	@Router      /runners/registration-token [post]
func (a *API) createRunnerToken(w http.ResponseWriter, r *http.Request) {
	me := userFrom(r)
	var in struct {
		Scope string `json:"scope"` // user | org
		Org   string `json:"org"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	scope := ""
	switch in.Scope {
	case "user":
		scope = "user:" + me
	case "org":
		org := strings.TrimSpace(in.Org)
		if org == "" || !a.store.IsOrg(org) || a.store.OrgRole(org, me) != "owner" {
			writeNotFound(w, "org")
			return
		}
		scope = "org:" + org
	default:
		writeCode(w, http.StatusBadRequest, "invalid_scope", "scope must be \"user\" or \"org\"")
		return
	}
	token, dto, err := a.store.CreateRunnerToken(scope)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"token":      token,
		"scope":      dto.Scope,
		"expires_at": dto.ExpiresAt,
	})
}

// createGlobalRunnerToken 管理端签发全局 scope 注册 token。
func (a *API) createGlobalRunnerToken(w http.ResponseWriter, r *http.Request) {
	token, dto, err := a.store.CreateRunnerToken("")
	if err != nil {
		internalError(w, err)
		return
	}
	log.Printf("runner audit: GLOBAL TOKEN issued by admin time=%s", time.Now().UTC().Format(time.RFC3339))
	writeJSON(w, http.StatusCreated, map[string]any{
		"token":      token,
		"scope":      dto.Scope,
		"expires_at": dto.ExpiresAt,
	})
}

// ---- 注册 ----

// registerRunner agent 用一次性注册 token 注册，换取长期 secret（只返回一次）。
//
//	@Summary     注册 runner
//	@Tags        runners
//	@Accept      json
//	@Produce     json
//	@Param       body body registerRunnerReq true "name/labels/token"
//	@Success     201 {object} object
//	@Failure     403 {object} map[string]string
//	@Router      /runner/register [post]
func (a *API) registerRunner(w http.ResponseWriter, r *http.Request) {
	if a.runnerHub == nil || !a.runnerHub.Enabled() {
		writeCode(w, http.StatusServiceUnavailable, "runner_disabled",
			"runner requires redis (GITDASH_QUEUE=redis)")
		return
	}
	var in struct {
		Name   string   `json:"name"`
		Labels []string `json:"labels"`
		Token  string   `json:"token"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if len(in.Name) < 2 || len(in.Name) > 64 || !runnerNameRe.MatchString(in.Name) {
		writeCode(w, http.StatusBadRequest, "invalid_name", "invalid runner name")
		return
	}
	if len(in.Labels) > 10 {
		writeCode(w, http.StatusBadRequest, "too_many_labels", "too many labels (max 10)")
		return
	}
	for _, l := range in.Labels {
		if l == "" || len(l) > 32 {
			writeCode(w, http.StatusBadRequest, "invalid_label", "invalid label")
			return
		}
	}
	scope, err := a.store.ConsumeRunnerToken(in.Token)
	if err != nil {
		writeCode(w, http.StatusForbidden, "invalid_token", "registration token invalid or expired")
		return
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		internalError(w, err)
		return
	}
	secret := hex.EncodeToString(raw)
	runnerDTO, err := a.store.CreateRunner(in.Name, secret, strings.Join(in.Labels, ","), scope)
	if err != nil {
		if errors.Is(err, store.ErrExists) {
			writeCode(w, http.StatusConflict, "name_taken", "runner name already registered")
			return
		}
		internalError(w, err)
		return
	}
	log.Printf("runner audit: REGISTER runner=%s scope=%q time=%s", in.Name, scope, time.Now().UTC().Format(time.RFC3339))
	writeJSON(w, http.StatusCreated, map[string]any{
		"runner": runnerDTO,
		"secret": secret, // 仅此一次
	})
}

// ---- 管理 ----

// listRunners 当前用户可见的 runner（个人 scope + 其拥有的组织 scope）。
//
//	@Summary     列出 runner
//	@Tags        runners
//	@Produce     json
//	@Success     200 {array} store.Runner
//	@Security    BearerAuth
//	@Router      /runners [get]
func (a *API) listRunners(w http.ResponseWriter, r *http.Request) {
	me := userFrom(r)
	orgs, err := a.store.MyOwnedOrgs(me)
	if err != nil {
		internalError(w, err)
		return
	}
	scopes := make([]string, 0, len(orgs)+1)
	scopes = append(scopes, "user:"+me)
	for _, o := range orgs {
		scopes = append(scopes, "org:"+o)
	}
	runners, err := a.store.ListRunnersByScopes(scopes)
	if err != nil {
		internalError(w, err)
		return
	}
	// 附上实时在线状态（Redis lastseen）
	for i := range runners {
		if a.runnerHub != nil && a.runnerHub.IsOnline(r.Context(), runners[i].Name) {
			runners[i].Status = "online"
		}
	}
	writeJSON(w, http.StatusOK, runners)
}

// deleteRunner 删除 runner（scope 命中才可删）。
func (a *API) deleteRunner(w http.ResponseWriter, r *http.Request) {
	me := userFrom(r)
	name := r.PathValue("name")
	runnerDTO, err := a.store.GetRunner(name)
	if err != nil {
		writeNotFound(w, "runner")
		return
	}
	if !a.runnerScopeAllowed(me, runnerDTO.Scope) {
		writeNotFound(w, "runner")
		return
	}
	if err := a.store.DeleteRunner(name); err != nil {
		internalError(w, err)
		return
	}
	log.Printf("runner audit: DELETE runner=%s by=%s time=%s", name, me, time.Now().UTC().Format(time.RFC3339))
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
}

// runnerScopeAllowed 当前用户是否可管理该 scope 的 runner。
func (a *API) runnerScopeAllowed(me, scope string) bool {
	switch {
	case scope == "":
		return false // 全局 runner 仅管理端可删
	case strings.HasPrefix(scope, "user:"):
		return scope == "user:"+me
	case strings.HasPrefix(scope, "org:"):
		return a.store.OrgRole(strings.TrimPrefix(scope, "org:"), me) == "owner"
	}
	return false
}

// adminListRunners 管理端全量列表。
func (a *API) adminListRunners(w http.ResponseWriter, r *http.Request) {
	runners, err := a.store.ListAllRunners()
	if err != nil {
		internalError(w, err)
		return
	}
	for i := range runners {
		if a.runnerHub != nil && a.runnerHub.IsOnline(r.Context(), runners[i].Name) {
			runners[i].Status = "online"
		}
	}
	writeJSON(w, http.StatusOK, runners)
}

// adminDeleteRunner 管理端删除任意 runner。
func (a *API) adminDeleteRunner(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := a.store.DeleteRunner(name); err != nil {
		writeNotFound(w, "runner")
		return
	}
	log.Printf("runner audit: DELETE runner=%s by=admin time=%s", name, time.Now().UTC().Format(time.RFC3339))
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
}

// ---- WS ----

// runnerWS agent WebSocket 端点（Bearer {name}:{secret} 认证，非用户 API）。
func (a *API) runnerWS(w http.ResponseWriter, r *http.Request) {
	a.runnerHub.HandleWS(w, r)
}

type (
	//nolint:unused // 仅供 swagger @Param 注解引用
	createRunnerTokenReq struct {
		Scope string `json:"scope"`
		Org   string `json:"org"`
	}
	//nolint:unused // 仅供 swagger @Param 注解引用
	registerRunnerReq struct {
		Name   string   `json:"name"`
		Labels []string `json:"labels"`
		Token  string   `json:"token"`
	}
)
