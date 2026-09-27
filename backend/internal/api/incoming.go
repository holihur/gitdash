package api

import (
	"errors"
	"gitdash/backend/internal/logx"
	"gitdash/backend/internal/pipeline"
	"gitdash/backend/internal/store"
	"net/http"
	"strconv"
	"strings"
)

// incomingWebhookPath 返回入站 webhook 的调用路径（前端展示用）。
func incomingWebhookPath(owner, name string) string {
	return "/api/hooks/incoming/" + owner + "/" + name
}

// listIncomingWebhooks 列出仓库的入站 webhook（仅 maintain 及以上）。
//
//	@Summary     列出入站 webhook
//	@Description 返回仓库已配置的入站 webhook（不返回 token 明文）与调用路径。
//	@Tags        webhooks
//	@Produce     json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Success     200 {object} map[string]any "webhooks / path"
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/incoming-webhooks [get]
func (a *API) listIncomingWebhooks(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireRole(w, r, "maintain")
	if !ok {
		return
	}
	hooks, err := a.store.ListIncomingWebhooks(owner, name)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"webhooks": hooks,
		"path":     incomingWebhookPath(owner, name),
	})
}

// createIncomingWebhook 新增一个入站 webhook token（仅 maintain 及以上）。
//
//	@Summary     新增入站 webhook
//	@Description 生成一个新的用于创建 issue 的入站 webhook token；明文 token 仅此一次返回。可配置多个。
//	@Tags        webhooks
//	@Accept      json
//	@Produce     json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Param       body  body incomingWebhookCreateReq false "名称（可空，默认 default）"
//	@Success     201 {object} map[string]any "webhook / token / path"
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/incoming-webhooks [post]
func (a *API) createIncomingWebhook(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireRole(w, r, "maintain")
	if !ok {
		return
	}
	var in struct {
		Name string `json:"name"`
	}
	if err := readOptionalJSON(w, r, &in); err != nil {
		return
	}
	hookName := strings.TrimSpace(in.Name)
	if hookName == "" {
		hookName = "default"
	}
	if len([]rune(hookName)) > 100 {
		writeCode(w, http.StatusBadRequest, "name_too_long", "name must be at most 100 characters")
		return
	}
	token, hook, err := a.store.CreateIncomingWebhook(owner, name, hookName)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"webhook": hook,
		"token":   token,
		"path":    incomingWebhookPath(owner, name),
	})
}

// updateIncomingWebhook 启用 / 禁用某个入站 webhook（仅 maintain 及以上）。
//
//	@Summary     启用/禁入站 webhook
//	@Description 禁用后该 token 立即失效，但配置保留，可再次启用。
//	@Tags        webhooks
//	@Accept      json
//	@Produce     json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Param       id    path int    true "webhook id"
//	@Param       body  body incomingWebhookUpdateReq true "enabled"
//	@Success     200 {object} map[string]any
//	@Failure     404 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/incoming-webhooks/{id} [patch]
func (a *API) updateIncomingWebhook(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireRole(w, r, "maintain")
	if !ok {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_id", "invalid webhook id")
		return
	}
	var in struct {
		Enabled *bool `json:"enabled"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	if in.Enabled == nil {
		writeCode(w, http.StatusBadRequest, "no_changes", "'enabled' is required")
		return
	}
	if err := a.store.SetIncomingWebhookEnabled(owner, name, id, *in.Enabled); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeNotFound(w, "incoming webhook")
			return
		}
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"enabled": *in.Enabled})
}

// deleteIncomingWebhook 删除（吐销）某个入站 webhook（仅 maintain 及以上）。
//
//	@Summary     删除入站 webhook
//	@Tags        webhooks
//	@Produce     json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Param       id    path int    true "webhook id"
//	@Success     204 {object} nil
//	@Failure     404 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/incoming-webhooks/{id} [delete]
func (a *API) deleteIncomingWebhook(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireRole(w, r, "maintain")
	if !ok {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_id", "invalid webhook id")
		return
	}
	if err := a.store.DeleteIncomingWebhook(owner, name, id); err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			internalError(w, err)
			return
		}
		writeCode(w, http.StatusNotFound, "incoming_webhook_not_found", "incoming webhook not found")
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
	var in struct {
		Title    string `json:"title"`
		Body     string `json:"body"`
		Priority string `json:"priority"` // critical|high|medium|low（可选）
		// Pipeline 非空时触发一次流水线（on 需包含 workflow_dispatch）；可与 issue 同时使用。
		Pipeline *struct {
			File   string            `json:"file"`
			Ref    string            `json:"ref"`
			Inputs map[string]string `json:"inputs"`
		} `json:"pipeline"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	if in.Pipeline == nil && strings.TrimSpace(in.Title) == "" {
		writeCode(w, http.StatusBadRequest, "title_required", "title is required (or provide a pipeline block)")
		return
	}

	resp := map[string]any{}
	if in.Pipeline != nil {
		// 入站 webhook 无登录用户，触发者记为仓库 owner。
		runs, code, msg := a.triggerPipelineFromWebhook(owner, repo, owner, in.Pipeline.File, in.Pipeline.Ref, in.Pipeline.Inputs)
		if code != "" {
			writeCode(w, http.StatusBadRequest, code, msg)
			return
		}
		resp["runs"] = runs
	}
	if strings.TrimSpace(in.Title) != "" {
		issue, ok := a.newIssue(w, owner, repo, owner, in.Title, in.Body, in.Priority, "webhook")
		if !ok {
			return
		}
		// 仅创建 issue 时保持原有响应形状（直接返回 issue 对象）。
		if in.Pipeline == nil {
			writeJSON(w, http.StatusCreated, issue)
			return
		}
		resp["issue"] = issue
	}
	writeJSON(w, http.StatusCreated, resp)
}

// triggerPipelineFromWebhook 以 workflow_dispatch 语义触发流水线（供入站 webhook 使用）。
// 返回 (runs, 错误码, 错误信息)；错误码为空表示成功。
func (a *API) triggerPipelineFromWebhook(owner, name, actor, file, ref string, inputs map[string]string) ([]store.PipelineRun, string, string) {
	resolvedRef, sha, code, msg := resolveRunTarget(owner, name, ref, "")
	if code != "" {
		return nil, code, msg
	}
	file = strings.TrimSpace(file)
	if !a.pipelineDispatchEnabled(owner, name, sha, file) {
		return nil, "dispatch_not_enabled", "add \"workflow_dispatch\" to on: in the pipeline file to enable dispatch"
	}
	resolved := sanitizeInputs(inputs)
	if cfg := a.pipelineConfig(owner, name, sha, file); cfg != nil && len(cfg.Params) > 0 {
		resolvedInputs, perr := pipeline.ResolveInputs(cfg, resolved)
		if perr != nil {
			return nil, "invalid_inputs", perr.Error()
		}
		resolved = resolvedInputs
	}
	runs, err := pipeline.TriggerAll(a.store, pipeline.TriggerOpts{
		Owner: owner, Repo: name, File: file, SHA: sha, Ref: resolvedRef,
		By: actor, Event: "workflow_dispatch", Inputs: resolved,
	})
	switch {
	case err == nil:
		return runs, "", ""
	case errors.Is(err, pipeline.ErrNoPipeline):
		return nil, "pipeline_file_missing", "no pipeline definition found at the target commit"
	case errors.Is(err, pipeline.ErrTriggerDisabled):
		return nil, "trigger_disabled", "this pipeline does not enable the requested trigger (see on: in the pipeline file)"
	case errors.Is(err, pipeline.ErrTooManyRuns):
		return nil, "too_many_runs", "too many active pipeline runs"
	default:
		return nil, "internal", "internal server error"
	}
}
