package api

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"gitdash/backend/internal/gitsvc"
	"gitdash/backend/internal/jobs"
	"gitdash/backend/internal/store"
)

// ---- third-party account connections ----

// connectionView 是返回给前端的绑定状态（不含令牌）。
type connectionView struct {
	Provider  string `json:"provider"`
	Label     string `json:"label"`
	Enabled   bool   `json:"enabled"`
	Connected bool   `json:"connected"`
	Login     string `json:"login,omitempty"`
	AvatarURL string `json:"avatar_url,omitempty"`
	BaseURL   string `json:"base_url,omitempty"`
}

// listConnections 列出支持的平台及其绑定状态。
//
//	@Summary     列出第三方账号绑定
//	@Tags        connections
//	@Produce     json
//	@Success     200 {array} connectionView
//	@Security    BearerAuth
//	@Router      /connections [get]
func (a *API) listConnections(w http.ResponseWriter, r *http.Request) {
	uid, err := a.store.UserID(userFrom(r))
	if err != nil {
		internalError(w, err)
		return
	}
	linked, err := a.store.LinkedAccounts(uid)
	if err != nil {
		internalError(w, err)
		return
	}
	byProvider := make(map[string]store.LinkedAccount, len(linked))
	for _, l := range linked {
		byProvider[l.Provider] = l
	}
	out := make([]connectionView, 0, len(forgeProviderNames))
	for _, name := range forgeProviderNames {
		cfg, _ := a.forgeConfigFor(name)
		v := connectionView{Provider: name, Label: cfg.Label, Enabled: cfg.Enabled}
		if acc, ok := byProvider[name]; ok {
			v.Connected = true
			v.Login = acc.Login
			v.AvatarURL = acc.AvatarURL
			v.BaseURL = acc.BaseURL
		}
		out = append(out, v)
	}
	writeJSON(w, http.StatusOK, out)
}

// connectStart 发起第三方账号绑定：跳转到平台授权页。
//
//	@Summary     开始绑定第三方账号
//	@Tags        connections
//	@Param       provider path string true "github|gitlab|gitea|bitbucket"
//	@Success     302
//	@Failure     404 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /connections/{provider}/start [get]
func (a *API) connectStart(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	cfg, ok := a.forgeConfigFor(provider)
	if !ok || !cfg.Enabled {
		writeCode(w, http.StatusNotFound, "connect_disabled", "this provider is not enabled")
		return
	}
	uid, err := a.store.UserID(userFrom(r))
	if err != nil {
		internalError(w, err)
		return
	}
	state, err := newSessionToken()
	if err != nil {
		internalError(w, err)
		return
	}
	if err := a.store.PutConnectState(state, uid, time.Now().Add(10*time.Minute).UTC().Format(time.RFC3339)); err != nil {
		internalError(w, err)
		return
	}
	redirect := reqBase(r) + "/api/connections/" + url.PathEscape(provider) + "/callback"
	http.Redirect(w, r, forgeAuthorizeURL(cfg, redirect, state), http.StatusFound)
}

// connectCallback 处理第三方授权回调，保存绑定。
//
//	@Summary     第三方账号绑定回调
//	@Tags        connections
//	@Param       provider path string true "provider"
//	@Success     302
//	@Router      /connections/{provider}/callback [get]
func (a *API) connectCallback(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	cfg, ok := a.forgeConfigFor(provider)
	if !ok || !cfg.Enabled {
		a.connectFail(w, r, "this provider is not enabled")
		return
	}
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	state := strings.TrimSpace(r.URL.Query().Get("state"))
	if code == "" || state == "" {
		a.connectFail(w, r, "invalid authorization response")
		return
	}
	uid, ok := a.store.TakeConnectState(state, time.Now().UTC().Format(time.RFC3339))
	if !ok {
		a.connectFail(w, r, "authorization state expired, please retry")
		return
	}
	tok, err := forgeExchangeToken(r.Context(), cfg, code, reqBase(r)+"/api/connections/"+url.PathEscape(provider)+"/callback")
	if err != nil {
		a.connectFail(w, r, err.Error())
		return
	}
	u, err := forgeFetchUser(r.Context(), cfg, tok.AccessToken)
	if err != nil {
		a.connectFail(w, r, "failed to fetch account profile")
		return
	}
	base := cfg.BaseURL
	if err := a.store.UpsertLinkedAccount(uid, provider, u.ExternalID, u.Login, u.AvatarURL, base, tok.AccessToken, tok.RefreshToken, tok.Scope); err != nil {
		a.connectFail(w, r, "failed to save connection")
		return
	}
	http.Redirect(w, r, "/profile?connected="+url.QueryEscape(provider), http.StatusFound)
}

// connectFail 绑定失败时重定向回前台并附带错误信息。
func (a *API) connectFail(w http.ResponseWriter, r *http.Request, msg string) {
	http.Redirect(w, r, "/profile?connect_error="+url.QueryEscape(msg), http.StatusFound)
}

// disconnectProvider 解除第三方账号绑定。
//
//	@Summary     解除第三方账号绑定
//	@Tags        connections
//	@Param       provider path string true "provider"
//	@Success     204
//	@Failure     404 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /connections/{provider} [delete]
func (a *API) disconnectProvider(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	if _, ok := a.forgeConfigFor(provider); !ok {
		writeNotFound(w, "provider")
		return
	}
	uid, err := a.store.UserID(userFrom(r))
	if err != nil {
		internalError(w, err)
		return
	}
	if err := a.store.DeleteLinkedAccount(uid, provider); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeNotFound(w, "connection")
			return
		}
		internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// listProviderRepos 列出已绑定账号可访问的远程仓库（用于批量导入）。
//
//	@Summary     列出远程仓库
//	@Tags        connections
//	@Param       provider path string true "provider"
//	@Produce     json
//	@Success     200 {array} forgeRepo
//	@Failure     404 {object} map[string]string
//	@Failure     502 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /connections/{provider}/repos [get]
func (a *API) listProviderRepos(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	acc, ok := a.linkedAccount(w, r, provider)
	if !ok {
		return
	}
	cfg, _ := a.forgeConfigFor(provider)
	repos, err := forgeListRepos(r.Context(), cfg, acc.AccessToken, forgeUser{ExternalID: acc.ExternalID, Login: acc.Login})
	if err != nil {
		writeCode(w, http.StatusBadGateway, "provider_error", "failed to list repositories from provider")
		return
	}
	writeJSON(w, http.StatusOK, repos)
}

// linkedAccount 解析 provider 配置，并取当前用户的绑定；出错时已写响应。
func (a *API) linkedAccount(w http.ResponseWriter, r *http.Request, provider string) (*store.LinkedAccountSecret, bool) {
	cfg, ok := a.forgeConfigFor(provider)
	if !ok || !cfg.Enabled {
		writeCode(w, http.StatusNotFound, "connect_disabled", "this provider is not enabled")
		return nil, false
	}
	uid, err := a.store.UserID(userFrom(r))
	if err != nil {
		internalError(w, err)
		return nil, false
	}
	acc, err := a.store.GetLinkedAccount(uid, provider)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeCode(w, http.StatusNotFound, "not_connected", "connect this provider before importing")
			return nil, false
		}
		internalError(w, err)
		return nil, false
	}
	return acc, true
}

// ---- batch import ----

// batchImportReq 批量导入请求体。
type batchImportReq struct {
	Provider  string   `json:"provider"`  // 已绑定的 provider
	Namespace string   `json:"namespace"` // 可选：目标组织
	Private   *bool    `json:"private"`   // 是否私有，默认 true
	Repos     []string `json:"repos"`     // 远程仓库 full_name 列表
}

// batchImportResult 批量导入结果。
type batchImportResult struct {
	Imported []string          `json:"imported"`
	Skipped  map[string]string `json:"skipped"`
}

// importBatch 从已绑定账号批量导入若干仓库。
//
//	@Summary     批量导入仓库
//	@Description 使用已绑定的第三方账号，把选中的远程仓库批量创建并异步导入。单个仓库失败不影响其余。
//	@Tags        connections
//	@Accept      json
//	@Produce     json
//	@Param       body body batchImportReq true "provider 与 full_name 列表"
//	@Success     202 {object} batchImportResult
//	@Failure     400 {object} map[string]string
//	@Failure     403 {object} map[string]string
//	@Failure     404 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /imports/batch [post]
func (a *API) importBatch(w http.ResponseWriter, r *http.Request) {
	var in batchImportReq
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	provider := strings.TrimSpace(in.Provider)
	acc, ok := a.linkedAccount(w, r, provider)
	if !ok {
		return
	}
	if len(in.Repos) == 0 {
		writeCode(w, http.StatusBadRequest, "empty_selection", "select at least one repository")
		return
	}
	if len(in.Repos) > 100 {
		writeCode(w, http.StatusBadRequest, "too_many_repos", "import at most 100 repositories at a time")
		return
	}
	me := userFrom(r)
	targetOwner := me
	if ns := strings.TrimSpace(in.Namespace); ns != "" && ns != me {
		if !a.store.IsOrg(ns) || a.store.OrgRole(ns, me) == "" {
			writeCode(w, http.StatusForbidden, "org_forbidden", "you are not a member of this organization")
			return
		}
		targetOwner = ns
	}
	private := true
	if in.Private != nil {
		private = *in.Private
	}

	cfg, _ := a.forgeConfigFor(provider)
	remote, err := forgeListRepos(r.Context(), cfg, acc.AccessToken, forgeUser{ExternalID: acc.ExternalID, Login: acc.Login})
	if err != nil {
		writeCode(w, http.StatusBadGateway, "provider_error", "failed to list repositories from provider")
		return
	}
	byName := make(map[string]forgeRepo, len(remote))
	for _, rr := range remote {
		byName[rr.FullName] = rr
	}

	result := batchImportResult{Skipped: map[string]string{}}
	credential := forgeCredential(cfg, acc.Login, acc.AccessToken)
	for _, full := range in.Repos {
		full = strings.TrimSpace(full)
		if full == "" {
			continue
		}
		rr, found := byName[full]
		if !found {
			result.Skipped[full] = "not found in remote repositories"
			continue
		}
		clone := strings.TrimSpace(rr.CloneURL)
		if _, err := validImportURL(clone); err != nil {
			result.Skipped[full] = "invalid clone url"
			continue
		}
		name := rr.Name
		if name == "" {
			name = repoNameFromURL(clone)
		}
		if !gitsvc.ValidName(name) {
			result.Skipped[full] = "invalid repository name"
			continue
		}
		if _, err := a.store.GetRepo(targetOwner, name); err == nil || gitsvc.Exists(targetOwner, name) {
			result.Skipped[full] = "repository already exists"
			continue
		}
		repo, err := a.store.CreateRepo(targetOwner, name, "", private)
		if err != nil {
			result.Skipped[full] = "failed to create repository"
			continue
		}
		_ = a.store.WatchRepo(me, repo.Owner, repo.Name)
		if err := a.store.SetImportSource(repo.Owner, repo.Name, clone); err != nil {
			_ = a.store.DeleteRepo(repo.Owner, repo.Name)
			_ = gitsvc.Delete(repo.Owner, repo.Name)
			result.Skipped[full] = "failed to record import source"
			continue
		}
		if err := a.store.SetImportCredential(repo.Owner, repo.Name, credential); err != nil {
			result.Skipped[full] = "failed to store credentials"
			continue
		}
		if err := a.store.SetImportStatus(repo.Owner, repo.Name, jobs.StatusQueued, ""); err != nil {
			result.Skipped[full] = "failed to queue import"
			continue
		}
		if err := a.enqueueImport(repo.Owner, repo.Name, clone, "", credential); err != nil {
			_ = a.store.SetImportStatus(repo.Owner, repo.Name, jobs.StatusFailed, "enqueue: "+err.Error())
			result.Skipped[full] = "failed to queue import"
			continue
		}
		result.Imported = append(result.Imported, full)
	}
	writeJSON(w, http.StatusAccepted, result)
}
