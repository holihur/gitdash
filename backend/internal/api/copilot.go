package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/coder/websocket"

	"gitdash/backend/internal/copilot"
	"gitdash/backend/internal/logx"
	"gitdash/backend/internal/store"
)

// SetCopilotManager 注入 copilot 编排器（main 启动时注入）。
func (a *API) SetCopilotManager(m *copilot.Manager) { a.copilotMgr = m }

// CopilotPullOpened 是 copilot 自动开 PR 后的回调：向仓库关注者推送通知。
func (a *API) CopilotPullOpened(session store.CopilotSession, pr store.PullRequest) {
	a.notify(session.Owner, session.Repo, "pull", "opened", session.CreatedBy, pr.Number, pr.Title, "")
}

// ---- BYOK（bring your own key）----

type byokReq struct {
	Name     string `json:"name"`
	Provider string `json:"provider"`
	APIKey   string `json:"api_key"`
	BaseURL  string `json:"base_url"`
	Model    string `json:"model"`
	// ID 仅用于测试连接：api_key 留空时回退到已保存密钥。
	ID int64 `json:"id,omitempty"`
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
	case !supportedProvider(in.Provider):
		return "unsupported_provider", "unsupported provider: " + in.Provider +
			" (supported: " + strings.Join(copilot.ProviderNames(), ", ") + ")"
	case len(in.APIKey) > 1024 || len(in.BaseURL) > 1024 || len(in.Model) > 255:
		return "invalid_value", "value too long"
	}
	return "", ""
}

func supportedProvider(name string) bool {
	_, ok := copilot.Provider(name)
	return ok
}

// normalizeByokKey 为不需要密钥的 provider 填一个占位密钥（agent 要求非空）。
func normalizeByokKey(provider, apiKey string) string {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey != "" {
		return apiKey
	}
	if spec, ok := copilot.Provider(provider); ok && !spec.KeyRequired {
		return "not-needed"
	}
	return ""
}

func byokKeyRequired(provider string) bool {
	spec, ok := copilot.Provider(provider)
	return !ok || spec.KeyRequired
}

// testByokReq 是测试连接的请求体（api_key 可留空并回退到已保存密钥）。
type testByokReq struct {
	ID       int64  `json:"id"`
	Provider string `json:"provider"`
	APIKey   string `json:"api_key"`
	BaseURL  string `json:"base_url"`
	Model    string `json:"model"`
}

// testByok 用给定（或已保存）的凭据向 LLM 端点发一次最小请求，验证配置可用。
//
//	@Summary     测试 BYOK 连接
//	@Description 向 Anthropic 兼容端点发送一次最小请求，返回是否连通及错误信息。
//	@Tags        byok
//	@Accept      json
//	@Produce     json
//	@Param       body body testByokReq true "provider/base_url/model/api_key(可留空回退到 id)"
//	@Success     200 {object} map[string]any
//	@Security    BearerAuth
//	@Router      /me/byok/test [post]
func (a *API) testByok(w http.ResponseWriter, r *http.Request) {
	me := userFrom(r)
	var in testByokReq
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	in.Provider = strings.TrimSpace(in.Provider)
	providerGiven := in.Provider != ""
	in.Provider = strings.ToLower(in.Provider)
	apiKey := strings.TrimSpace(in.APIKey)
	if in.ID > 0 && (apiKey == "" || strings.TrimSpace(in.BaseURL) == "" || strings.TrimSpace(in.Model) == "" || !providerGiven) {
		secret, err := a.store.GetByokSecret(me, in.ID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeNotFound(w, "byok")
				return
			}
			internalError(w, err)
			return
		}
		if !providerGiven {
			in.Provider = strings.ToLower(secret.Provider)
		}
		if apiKey == "" {
			apiKey = secret.APIKey
		}
		if strings.TrimSpace(in.BaseURL) == "" {
			in.BaseURL = secret.BaseURL
		}
		if strings.TrimSpace(in.Model) == "" {
			in.Model = secret.Model
		}
	}
	if in.Provider == "" {
		in.Provider = "anthropic"
	}
	if !supportedProvider(in.Provider) {
		writeCode(w, http.StatusBadRequest, "unsupported_provider", "unsupported provider: "+in.Provider)
		return
	}
	apiKey = normalizeByokKey(in.Provider, apiKey)
	if apiKey == "" {
		writeCode(w, http.StatusBadRequest, "api_key_required", "api_key is required")
		return
	}
	baseURL := copilot.EffectiveBaseURL(in.Provider, in.BaseURL)
	model := copilot.EffectiveModel(in.Provider, in.Model)
	if err := copilot.TestConnection(r.Context(), in.Provider, baseURL, apiKey, model); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "base_url": baseURL, "model": model})
}

// composeIssuePrompt 把 issue 的标题/正文与用户附加要求拼成 agent 的起始提示词。
func composeIssuePrompt(issue store.Issue, extra string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "请修复以下 issue，完成后确保改动可提交：\n\n# %s\n\n%s", strings.TrimSpace(issue.Title), strings.TrimSpace(issue.Body))
	if e := strings.TrimSpace(extra); e != "" {
		b.WriteString("\n\n附加要求：\n")
		b.WriteString(e)
	}
	return b.String()
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
	if strings.TrimSpace(in.APIKey) == "" && byokKeyRequired(in.Provider) {
		writeCode(w, http.StatusBadRequest, "api_key_required", "api_key is required")
		return
	}
	key, err := a.store.CreateByokKey(me, in.Name, in.Provider, normalizeByokKey(in.Provider, in.APIKey), in.BaseURL, in.Model)
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
	key, err := a.store.UpdateByokKey(me, id, in.Name, in.Provider, normalizeByokKey(in.Provider, in.APIKey), in.BaseURL, in.Model)
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
	ByokID      int64  `json:"byok_id"`
	Prompt      string `json:"prompt"`
	IssueNumber int64  `json:"issue_number"`
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
	writeJSON(w, http.StatusOK, sessions)
}

// createCopilot 创建一个 copilot 会话（嵌入式 agent，无需镜像）。
//
//	@Summary     创建 copilot 会话
//	@Tags        copilot
//	@Accept      json
//	@Produce     json
//	@Param       body body copilotCreateReq true "byok_id/prompt"
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
	in.Prompt = strings.TrimSpace(in.Prompt)
	if in.IssueNumber > 0 {
		issue, err := a.store.GetIssue(owner, name, in.IssueNumber)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeCode(w, http.StatusBadRequest, "issue_not_found", "issue not found")
				return
			}
			internalError(w, err)
			return
		}
		in.Prompt = composeIssuePrompt(issue, in.Prompt)
	}
	if len(in.Prompt) > 32<<10 {
		writeCode(w, http.StatusBadRequest, "too_long", "prompt too long")
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
	session, err := a.store.CreateCopilotSession(owner, name, me, in.ByokID, in.IssueNumber, in.Prompt)
	if err != nil {
		internalError(w, err)
		return
	}
	_ = a.store.SetCopilotSessionGit(owner, name, session.ID, copilot.BranchName(session.ID), "")
	session.Branch = copilot.BranchName(session.ID)
	writeJSON(w, http.StatusCreated, session)
}

// getCopilot 获取会话详情。
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
	writeJSON(w, http.StatusOK, session)
}

// copilotMessages 返回会话的对话历史（前端进入会话时回放）。
//
//	@Summary     获取 copilot 对话历史
//	@Tags        copilot
//	@Produce     json
//	@Security    BearerAuth
//	@Success     200 {array} copilot.ChatMessage
//	@Router      /users/{owner}/repos/{name}/copilots/{id}/messages [get]
func (a *API) copilotMessages(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, false)
	if !ok {
		return
	}
	session, ok := a.loadCopilotSession(w, r, owner, name)
	if !ok {
		return
	}
	if a.copilotMgr == nil {
		writeJSON(w, http.StatusOK, []copilot.ChatMessage{})
		return
	}
	msgs, err := a.copilotMgr.Messages(r.Context(), session)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, msgs)
}

type copilotChatIn struct {
	Type string `json:"type"` // user | cancel
	Text string `json:"text"`
}

// copilotChat 是会话的双向聊天 WebSocket：客户端发送 user/cancel 消息，
// 服务端流式回传 delta / tool_start / tool_end / done / error / status。
//
//	@Summary     copilot 双向聊天（WebSocket）
//	@Tags        copilot
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/copilots/{id}/chat [get]
func (a *API) copilotChat(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, true)
	if !ok {
		return
	}
	session, ok := a.loadCopilotSession(w, r, owner, name)
	if !ok {
		return
	}
	mgr := a.copilotMgr
	if mgr == nil {
		writeCode(w, http.StatusServiceUnavailable, "copilot_unavailable", "copilot manager not configured")
		return
	}

	ws, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer func() { _ = ws.Close(websocket.StatusNormalClosure, "") }()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	incoming := make(chan string, 8)
	go readCopilotChat(ctx, ws, incoming, func() { mgr.Cancel(ctx, session) })

	if history, err := mgr.Messages(ctx, session); err == nil {
		_ = writeCopilotChat(ctx, ws, map[string]any{"type": copilot.EventHistory, "messages": history})
	}
	_ = writeCopilotChat(ctx, ws, copilot.Event{Type: copilot.EventStatus, Status: "idle"})

	for {
		select {
		case <-ctx.Done():
			return
		case text, open := <-incoming:
			if !open {
				return
			}
			text = strings.TrimSpace(text)
			if text == "" {
				continue
			}
			_ = a.store.SetCopilotSessionStatus(owner, name, session.ID, "running", "")
			_ = writeCopilotChat(ctx, ws, copilot.Event{Type: copilot.EventStatus, Status: "running"})
			runErr := mgr.RunTurn(ctx, session, text, func(ev copilot.Event) {
				_ = writeCopilotChat(ctx, ws, ev)
			})
			status, errMsg := "idle", ""
			if runErr != nil {
				if errors.Is(runErr, context.Canceled) {
					_ = writeCopilotChat(ctx, ws, copilot.Event{Type: copilot.EventError, Error: "canceled"})
				} else {
					status, errMsg = "failed", runErr.Error()
					_ = writeCopilotChat(ctx, ws, copilot.Event{Type: copilot.EventError, Error: errMsg})
				}
			}
			_ = a.store.SetCopilotSessionStatus(owner, name, session.ID, status, errMsg)
			_ = writeCopilotChat(ctx, ws, copilot.Event{Type: copilot.EventStatus, Status: status})
		}
	}
}

// stopCopilot 取消会话正在运行的一轮对话。
//
//	@Summary     取消 copilot 当前轮次
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
		a.copilotMgr.Cancel(r.Context(), session)
	}
	_ = a.store.SetCopilotSessionStatus(owner, name, session.ID, "idle", "")
	session.Status = "idle"
	session.Error = ""
	writeJSON(w, http.StatusOK, session)
}

// deleteCopilot 删除会话（工作区 + 对话历史 + 记录）。
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
		if err := a.copilotMgr.RemoveWorkspace(session); err != nil {
			logx.Warnf("copilot: remove workspace session %d: %v", session.ID, err)
		}
	}
	if err := a.store.DeleteCopilotSession(owner, name, session.ID); err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

// loadCopilotSession 加载会话。
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
	return session, true
}

// ---- WebSocket 助手 ----
// readCopilotChat 读取客户端消息；收到 user 文本投入 out，收到 cancel 调用 onCancel。
// 连接关闭或出错时关闭 out。
func readCopilotChat(ctx context.Context, ws *websocket.Conn, out chan<- string, onCancel func()) {
	defer close(out)
	for {
		typ, data, err := ws.Read(ctx)
		if err != nil {
			return
		}
		if typ != websocket.MessageText {
			continue
		}
		var in copilotChatIn
		if json.Unmarshal(data, &in) != nil {
			continue
		}
		switch in.Type {
		case "cancel":
			onCancel()
		case "user":
			select {
			case out <- in.Text:
			case <-ctx.Done():
				return
			}
		}
	}
}

// writeCopilotChat 序列化并写入一条下行消息（带写超时）。
func writeCopilotChat(ctx context.Context, ws *websocket.Conn, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	wctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return ws.Write(wctx, websocket.MessageText, b)
}
