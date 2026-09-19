package api

import (
	"errors"
	"gitdash/backend/internal/logx"
	"gitdash/backend/internal/store"
	"net/http"
	"strings"
)

// incomingWebhookPath 返回入站 webhook 的调用路径（前端展示用）。
func incomingWebhookPath(owner, name string) string {
	return "/api/hooks/incoming/" + owner + "/" + name
}

// getIncomingWebhook 查询仓库入站 webhook 配置状态（仅 owner）。
//
//	@Summary     查看入站 webhook
//	@Description 返回该仓库是否已配置入站 webhook（不返回 token 明文）。
//	@Tags        webhooks
//	@Produce     json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Success     200 {object} map[string]any "enabled / created_at / last_used_at / path"
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/incoming-webhook [get]
func (a *API) getIncomingWebhook(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireOwner(w, r)
	if !ok {
		return
	}
	hook, ok, err := a.store.GetIncomingWebhook(owner, name)
	if err != nil {
		internalError(w, err)
		return
	}
	if !ok {
		writeJSON(w, http.StatusOK, map[string]any{"enabled": false})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled":      true,
		"created_at":   hook.CreatedAt,
		"last_used_at": hook.LastUsedAt,
		"path":         incomingWebhookPath(owner, name),
	})
}

// setIncomingWebhook 创建或轮换仓库入站 webhook token（仅 owner）。
//
//	@Summary     创建/轮换入站 webhook
//	@Description 生成（或轮换）用于创建 issue 的入站 webhook token；明文 token 仅此一次返回。
//	@Tags        webhooks
//	@Produce     json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Success     201 {object} map[string]any "token / path / created_at"
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/incoming-webhook [post]
func (a *API) setIncomingWebhook(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireOwner(w, r)
	if !ok {
		return
	}
	token, hook, err := a.store.SetIncomingWebhook(owner, name)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"enabled":    true,
		"token":      token,
		"path":       incomingWebhookPath(owner, name),
		"created_at": hook.CreatedAt,
	})
}

// deleteIncomingWebhook 删除（吊销）仓库入站 webhook（仅 owner）。
//
//	@Summary     删除入站 webhook
//	@Tags        webhooks
//	@Produce     json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Success     204 {object} nil
//	@Failure     404 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/incoming-webhook [delete]
func (a *API) deleteIncomingWebhook(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireOwner(w, r)
	if !ok {
		return
	}
	if errors.Is(a.store.DeleteIncomingWebhook(owner, name), store.ErrNotFound) {
		writeCode(w, http.StatusNotFound, "incoming_webhook_not_found", "incoming webhook not configured")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// createIssueFromIncomingWebhook 入站 webhook：外部系统携带 token 创建 issue。
// 该路由不经过登录鉴权，仅校验仓库级 token；创建成功后与网页端一样触发
// 收件箱通知与出站 webhook（a.newIssue → a.notify）。
//
//	@Summary     通过入站 webhook 创建 Issue
//	@Description 凭仓库入站 webhook token（X-Gitdash-Token 头或 ?token=）创建 issue。
//	@Tags        webhooks
//	@Accept      json
//	@Produce     json
//	@Param       owner path string true "仓库所有者"
//	@Param       repo  path string true "仓库名"
//	@Param       body  body createIssueReq true "标题与正文"
//	@Success     201 {object} store.Issue
//	@Failure     400 {object} map[string]string
//	@Failure     401 {object} map[string]string
//	@Failure     403 {object} map[string]string
//	@Failure     404 {object} map[string]string
//	@Router      /hooks/incoming/{owner}/{repo} [post]
func (a *API) createIssueFromIncomingWebhook(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	repo := r.PathValue("repo")
	token := strings.TrimSpace(r.Header.Get("X-Gitdash-Token"))
	if token == "" {
		token = bearerToken(r)
	}
	if token == "" {
		// 兼容 ?token=，但查询串会被代理/访问日志记录，建议改用 X-Gitdash-Token 头（§L4）。
		if q := strings.TrimSpace(r.URL.Query().Get("token")); q != "" {
			logx.Warnf("incoming webhook %s/%s used deprecated ?token= query; use the X-Gitdash-Token header", owner, repo)
			token = q
		}
	}
	ok, err := a.store.ResolveIncomingWebhook(owner, repo, token)
	if err != nil {
		internalError(w, err)
		return
	}
	if !ok {
		writeCode(w, http.StatusUnauthorized, "invalid_token", "invalid or missing incoming webhook token")
		return
	}
	info, err := a.store.GetRepo(owner, repo)
	if err != nil || info.Banned || a.store.IsOrgBanned(owner) {
		writeNotFound(w, "repo")
		return
	}
	var in createIssueReq
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	// 入站 webhook 无登录用户，issue 作者记为仓库 owner。
	issue, ok := a.newIssue(w, owner, repo, owner, in.Title, in.Body)
	if !ok {
		return
	}
	writeJSON(w, http.StatusCreated, issue)
}
