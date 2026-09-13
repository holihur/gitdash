package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"gitdash/backend/internal/copilot"
	"gitdash/backend/internal/store"
)

// SetCopilotManager 注入 copilot Docker 编排器（main 启动时注入）。
func (a *API) SetCopilotManager(m *copilot.Manager) { a.copilotMgr = m }

// ---- BYOK（bring your own key）----

type byokReq struct {
	Name     string `json:"name"`
	Provider string `json:"provider"`
	APIKey   string `json:"api_key"`
	BaseURL  string `json:"base_url"`
	Model    string `json:"model"`
}

func (in *byokReq) validate() (code, msg string) {
	in.Name = strings.TrimSpace(in.Name)
	in.Provider = strings.ToLower(strings.TrimSpace(in.Provider))
	if in.Provider == "" {
		in.Provider = "anthropic"
	}
	in.BaseURL = strings.TrimSpace(in.BaseURL)
	in.Model = strings.TrimSpace(in.Model)
	switch {
	case in.Name == "" || len(in.Name) > 64:
		return "invalid_name", "name is required (max 64)"
	case in.Provider != "anthropic":
		return "unsupported_provider", "only anthropic provider is supported"
	case len(in.APIKey) > 1024 || len(in.BaseURL) > 1024 || len(in.Model) > 255:
		return "invalid_value", "value too long"
	}
	return "", ""
}

// listByok 列出当前用户的 BYOK 密钥（不含明文）。
//
//	@Summary     列出 BYOK 密钥
//	@Description 返回当前用户配置的全部 BYOK（bring your own key）密钥，不含明文。
//	@Tags        byok
//	@Produce     json
//	@Security    BearerAuth
//	@Success     200 {array} store.ByokKey
//	@Router      /me/byok [get]
func (a *API) listByok(w http.ResponseWriter, r *http.Request) {
	keys, err := a.store.ListByokKeys(userFrom(r))
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, keys)
}

// createByok 创建 BYOK 密钥。
//
//	@Summary     创建 BYOK 密钥
//	@Tags        byok
//	@Accept      json
//	@Produce     json
//	@Param       body body byokReq true "name/provider/api_key/base_url/model"
//	@Success     201 {object} store.ByokKey
//	@Security    BearerAuth
//	@Router      /me/byok [post]
func (a *API) createByok(w http.ResponseWriter, r *http.Request) {
	me := userFrom(r)
	var in byokReq
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	if code, msg := in.validate(); code != "" {
		writeCode(w, http.StatusBadRequest, code, msg)
		return
	}
	if strings.TrimSpace(in.APIKey) == "" {
		writeCode(w, http.StatusBadRequest, "api_key_required", "api_key is required")
		return
	}
	key, err := a.store.CreateByokKey(me, in.Name, in.Provider, strings.TrimSpace(in.APIKey), in.BaseURL, in.Model)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, key)
}

// updateByok 更新 BYOK 密钥；api_key 留空则保留原密钥。
//
//	@Summary     更新 BYOK 密钥
//	@Tags        byok
//	@Accept      json
//	@Produce     json
//	@Param       body body byokReq true "name/provider/api_key(留空保留)/base_url/model"
//	@Success     200 {object} store.ByokKey
//	@Security    BearerAuth
//	@Router      /me/byok/{id} [put]
func (a *API) updateByok(w http.ResponseWriter, r *http.Request) {
	me := userFrom(r)
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeNotFound(w, "byok")
		return
	}
	var in byokReq
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	if code, msg := in.validate(); code != "" {
		writeCode(w, http.StatusBadRequest, code, msg)
		return
	}
	key, err := a.store.UpdateByokKey(me, id, in.Name, in.Provider, strings.TrimSpace(in.APIKey), in.BaseURL, in.Model)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeNotFound(w, "byok")
			return
		}
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, key)
}

// deleteByok 删除 BYOK 密钥。
//
//	@Summary     删除 BYOK 密钥
//	@Tags        byok
//	@Produce     json
//	@Success     200 {object} map[string]bool
//	@Security    BearerAuth
//	@Router      /me/byok/{id} [delete]
func (a *API) deleteByok(w http.ResponseWriter, r *http.Request) {
	me := userFrom(r)
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeNotFound(w, "byok")
		return
	}
	if err := a.store.DeleteByokKey(me, id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeNotFound(w, "byok")
			return
		}
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

// ---- copilot sessions ----

type copilotCreateReq struct {
	ByokID  int64  `json:"byok_id"`
	Image   string `json:"image"`
	Prompt  string `json:"prompt"`
	Command string `json:"command"`
}

// listCopilots 列出仓库的 copilot 会话。
//
//	@Summary     列出 copilot 会话
//	@Tags        copilot
//	@Produce     json
//	@Success     200 {array} store.CopilotSession
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/copilots [get]
func (a *API) listCopilots(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, false)
	if !ok {
		return
	}
	sessions, err := a.store.ListCopilotSessions(owner, name)
	if err != nil {
		internalError(w, err)
		return
	}
	// 尽力而为地回读容器状态，保持 UI 准确（docker 不可用时保持 DB 状态）
	for i := range sessions {
		if a.copilotMgr == nil {
			continue
		}
		if s, err := a.copilotMgr.DockerStatus(r.Context(), sessions[i]); err == nil && s != "" {
			next := dockerToStatus(s)
			if next != "" && next != sessions[i].Status {
				_ = a.store.SetCopilotSessionStatus(owner, name, sessions[i].ID, next, sessions[i].Error)
				sessions[i].Status = next
			}
		}
	}
	writeJSON(w, http.StatusOK, sessions)
}

// createCopilot 创建并启动一个 copilot 会话（独立 Docker 容器）。
//
//	@Summary     创建 copilot 会话
//	@Tags        copilot
//	@Accept      json
//	@Produce     json
//	@Param       body body copilotCreateReq true "byok_id/image/prompt/command"
//	@Success     201 {object} store.CopilotSession
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/copilots [post]
func (a *API) createCopilot(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, true)
	if !ok {
		return
	}
	me := userFrom(r)
	var in copilotCreateReq
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	in.Image = strings.TrimSpace(in.Image)
	if in.Image == "" {
		in.Image = copilot.DefaultImage()
	}
	if in.Image == "" {
		writeCode(w, http.StatusBadRequest, "image_required", "image is required (set GITDASH_COPILOT_IMAGE on the server or provide image)")
		return
	}
	if len(in.Prompt) > 32<<10 || len(in.Command) > 32<<10 {
		writeCode(w, http.StatusBadRequest, "too_long", "prompt/command too long")
		return
	}
	byok, err := a.store.GetByokKey(me, in.ByokID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeCode(w, http.StatusBadRequest, "byok_not_found", "byok key not found")
			return
		}
		internalError(w, err)
		return
	}
	if !byok.KeySet {
		writeCode(w, http.StatusBadRequest, "byok_key_empty", "byok key has no api_key")
		return
	}

	session, err := a.store.CreateCopilotSession(owner, name, me, in.ByokID, in.Image, in.Prompt, in.Command)
	if err != nil {
		internalError(w, err)
		return
	}
	session = a.startCopilotSession(r, session)
	writeJSON(w, http.StatusCreated, session)
}

// getCopilot 获取会话详情（含容器日志）。
//
//	@Summary     获取 copilot 会话
//	@Tags        copilot
//	@Produce     json
//	@Success     200 {object} store.CopilotSession
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/copilots/{id} [get]
func (a *API) getCopilot(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, false)
	if !ok {
		return
	}
	session, ok := a.loadCopilotSession(w, r, owner, name)
	if !ok {
		return
	}
	resp := map[string]any{
		"id": session.ID, "created_by": session.CreatedBy, "byok_id": session.ByokID,
		"image": session.Image, "prompt": session.Prompt, "command": session.Command,
		"status": session.Status, "error": session.Error,
		"created_at": session.CreatedAt, "updated_at": session.UpdatedAt,
	}
	if a.copilotMgr != nil {
		if log, err := a.copilotMgr.Logs(r.Context(), session); err == nil {
			resp["log"] = log
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

// startCopilot 启动会话容器。
//
//	@Summary     启动 copilot 会话
//	@Tags        copilot
//	@Produce     json
//	@Success     200 {object} store.CopilotSession
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/copilots/{id}/start [post]
func (a *API) startCopilot(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, true)
	if !ok {
		return
	}
	session, ok := a.loadCopilotSession(w, r, owner, name)
	if !ok {
		return
	}
	session = a.startCopilotSession(r, session)
	writeJSON(w, http.StatusOK, session)
}

// stopCopilot 停止会话容器（保留工作区）。
//
//	@Summary     停止 copilot 会话
//	@Tags        copilot
//	@Produce     json
//	@Success     200 {object} store.CopilotSession
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/copilots/{id}/stop [post]
func (a *API) stopCopilot(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, true)
	if !ok {
		return
	}
	session, ok := a.loadCopilotSession(w, r, owner, name)
	if !ok {
		return
	}
	if a.copilotMgr != nil {
		if err := a.copilotMgr.Stop(r.Context(), session); err != nil {
			internalError(w, err)
			return
		}
	}
	_ = a.store.SetCopilotSessionStatus(owner, name, session.ID, "stopped", "")
	session.Status = "stopped"
	session.Error = ""
	writeJSON(w, http.StatusOK, session)
}

// deleteCopilot 删除会话（容器 + 工作区 + 记录）。
//
//	@Summary     删除 copilot 会话
//	@Tags        copilot
//	@Produce     json
//	@Success     200 {object} map[string]bool
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/copilots/{id} [delete]
func (a *API) deleteCopilot(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, true)
	if !ok {
		return
	}
	session, ok := a.loadCopilotSession(w, r, owner, name)
	if !ok {
		return
	}
	if a.copilotMgr != nil {
		_ = a.copilotMgr.Stop(r.Context(), session)
		_ = a.copilotMgr.RemoveWorkspace(session)
	}
	if err := a.store.DeleteCopilotSession(owner, name, session.ID); err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

// loadCopilotSession 加载会话并按容器状态回写 DB。
func (a *API) loadCopilotSession(w http.ResponseWriter, r *http.Request, owner, name string) (store.CopilotSession, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeNotFound(w, "copilot")
		return store.CopilotSession{}, false
	}
	session, err := a.store.GetCopilotSession(owner, name, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeNotFound(w, "copilot")
			return store.CopilotSession{}, false
		}
		internalError(w, err)
		return store.CopilotSession{}, false
	}
	if a.copilotMgr != nil {
		if s, err := a.copilotMgr.DockerStatus(r.Context(), session); err == nil && s != "" {
			if next := dockerToStatus(s); next != "" && next != session.Status {
				_ = a.store.SetCopilotSessionStatus(owner, name, id, next, session.Error)
				session.Status = next
			}
		}
	}
	return session, true
}

// startCopilotSession 启动会话并回写状态（失败时标记 failed）；返回更新后的会话。
func (a *API) startCopilotSession(r *http.Request, session store.CopilotSession) store.CopilotSession {
	if a.copilotMgr == nil {
		_ = a.store.SetCopilotSessionStatus(session.Owner, session.Repo, session.ID, "failed", "copilot manager not configured")
		session.Status = "failed"
		session.Error = "copilot manager not configured"
		return session
	}
	if err := a.copilotMgr.Start(r.Context(), session); err != nil {
		msg := err.Error()
		_ = a.store.SetCopilotSessionStatus(session.Owner, session.Repo, session.ID, "failed", msg)
		session.Status = "failed"
		session.Error = msg
		return session
	}
	_ = a.store.SetCopilotSessionStatus(session.Owner, session.Repo, session.ID, "running", "")
	session.Status = "running"
	session.Error = ""
	return session
}

// dockerToStatus 把 docker inspect 的状态映射到会话状态。
func dockerToStatus(s string) string {
	switch s {
	case "running":
		return "running"
	case "exited", "created", "paused", "dead", "removing", "":
		return "stopped"
	}
	return ""
}
