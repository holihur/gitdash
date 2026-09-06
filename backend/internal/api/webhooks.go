package api

import (
	"errors"
	"gitdash/backend/internal/store"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func validWebhookURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

// listWebhooks 列出仓库的 webhook。
//
//	@Summary     列出 webhook
//	@Tags        webhooks
//	@Produce     json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Success     200 {array}  object
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/webhooks [get]
func (a *API) listWebhooks(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireOwner(w, r)
	if !ok {
		return
	}
	hooks, err := a.store.ListWebhooks(owner, name)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, hooks)
}

// createWebhook 创建 webhook（仅 owner）。
//
//	@Summary     创建 webhook
//	@Tags        webhooks
//	@Accept      json
//	@Produce     json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Param       body  body createWebhookReq true "url 与可选 secret（至少 16 字符）"
//	@Success     201 {object} object
//	@Failure     400 {object} map[string]string
//	@Failure     409 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/webhooks [post]
func (a *API) createWebhook(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireOwner(w, r)
	if !ok {
		return
	}
	var in struct {
		URL    string `json:"url"`
		Secret string `json:"secret"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	in.URL = strings.TrimSpace(in.URL)
	in.Secret = strings.TrimSpace(in.Secret)
	if !validWebhookURL(in.URL) {
		writeCode(w, http.StatusBadRequest, "invalid_url", "url must be a valid http(s) url")
		return
	}
	if len([]rune(in.URL)) > 2048 {
		writeCode(w, http.StatusBadRequest, "invalid_url", "url too long (max 2048 chars)")
		return
	}
	if in.Secret != "" && len(in.Secret) < 16 {
		writeCode(w, http.StatusBadRequest, "invalid_secret", "webhook secret must be at least 16 characters")
		return
	}
	hk, err := a.store.CreateWebhook(owner, name, in.URL, in.Secret)
	if errors.Is(err, store.ErrExists) {
		writeCode(w, http.StatusConflict, "webhook_exists", "webhook already registered")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, hk)
}

// deleteWebhook 删除 webhook（仅 owner）。
//
//	@Summary     删除 webhook
//	@Tags        webhooks
//	@Produce     json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Param       id    path int    true "webhook ID"
//	@Success     204 {object} nil
//	@Failure     404 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/webhooks/{id} [delete]
func (a *API) deleteWebhook(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireOwner(w, r)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeCode(w, http.StatusBadRequest, "invalid_id", "invalid id")
		return
	}
	if errors.Is(a.store.DeleteWebhook(owner, name, id), store.ErrNotFound) {
		writeCode(w, http.StatusNotFound, "webhook_not_found", "webhook not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// listWebhookDeliveries 列出 webhook 最近投递记录（仅 owner）。
//
//	@Summary     列出 webhook 投递记录
//	@Tags        webhooks
//	@Produce     json
//	@Param       owner path string  true "仓库所有者"
//	@Param       name  path string  true "仓库名"
//	@Param       id    path int     true "webhook ID"
//	@Param       limit query int    false "返回条数（默认 20，最大 100）"
//	@Success     200 {array}  object
//	@Failure     404 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/webhooks/{id}/deliveries [get]
func (a *API) listWebhookDeliveries(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireOwner(w, r)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeCode(w, http.StatusBadRequest, "invalid_id", "invalid id")
		return
	}
	limit := int64(0)
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, perr := strconv.ParseInt(v, 10, 64); perr == nil {
			limit = n
		}
	}
	deliveries, err := a.store.ListDeliveries(owner, name, id, limit)
	if errors.Is(err, store.ErrNotFound) {
		writeCode(w, http.StatusNotFound, "webhook_not_found", "webhook not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, deliveries)
}

// ---- ssh keys ----
