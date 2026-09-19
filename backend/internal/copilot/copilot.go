// Package copilot 是 gitdash 侧的 copilot 编排器：它把独立的 agent 运行时
// （实现 copilot-api/1，见 deps/agent/docs/copilot-api.md）作为会话工作区的
// 聊天后端，负责工作区克隆、进程生命周期、双向转发与闭环提交推送。
//
// 与 agent 的耦合只有 HTTP+SSE 协议本身：agent 可换实现，路径/命令均可经
// 环境变量配置，gitdash 不链接 agent 代码、不解析其内部存储格式。
package copilot

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gitdash/backend/internal/gitsvc"
	"gitdash/backend/internal/logx"
	"gitdash/backend/internal/store"
)

// ProtocolVersion 是 gitdash 期望的 agent 聊天协议版本。
const ProtocolVersion = "copilot-api/1"

// ErrByokMissing 会话没有可用的 BYOK 密钥。
var ErrByokMissing = errors.New("byok key not configured")

// ErrAgentUnavailable 找不到/起不来 agent 运行时。
var ErrAgentUnavailable = errors.New("agent runtime unavailable")

// 流式事件类型（与协议 copilot-api/1 一致）。
const (
	EventDelta     = "delta"
	EventToolStart = "tool_start"
	EventToolEnd   = "tool_end"
	EventDone      = "done"
	EventError     = "error"
	EventStatus    = "status"
	EventHistory   = "history"
)

// Event 是转发给前端的单条流式消息。
type Event struct {
	Type    string `json:"type"`
	Text    string `json:"text,omitempty"`
	Name    string `json:"name,omitempty"`
	Input   string `json:"input,omitempty"`
	Result  string `json:"result,omitempty"`
	Error   string `json:"error,omitempty"`
	IsError bool   `json:"is_error,omitempty"`
	Status  string `json:"status,omitempty"`
}

// ChatMessage 是回放给前端的单条对话。
type ChatMessage struct {
	Role  string      `json:"role"`
	Text  string      `json:"text,omitempty"`
	Tools []ToolEvent `json:"tools,omitempty"`
}

// ToolEvent 是 ChatMessage 中的一次工具调用。
type ToolEvent struct {
	Name    string `json:"name"`
	Input   string `json:"input,omitempty"`
	Result  string `json:"result,omitempty"`
	IsError bool   `json:"is_error,omitempty"`
}

var workspacesDir string

// Init 设置 copilot 工作区根目录（main 启动时调用）。
func Init(dataDir string) error {
	workspacesDir = filepath.Join(dataDir, "copilots")
	return os.MkdirAll(workspacesDir, 0o755)
}

// WorkspacePath 会话工作区目录（仓库克隆副本）。
func WorkspacePath(owner, repo string, id int64) string {
	return filepath.Join(workspacesDir, owner, repo, fmt.Sprintf("ws-%d", id))
}

// BranchName 会话工作区分支名。
func BranchName(id int64) string { return fmt.Sprintf("copilot/session-%d", id) }

// Manager 管理每个会话的 agent 运行时进程与工作区。
type Manager struct {
	st    *store.Store
	mu    sync.Mutex
	procs map[int64]*proc
	locks map[int64]*sync.Mutex
	// pullHook 在会话自动开 PR 后被调用（可为 nil），用于通知等副作用。
	pullHook PullHook
}

// PullHook 在 copilot 为关联 issue 的会话自动开出 PR 后触发。
type PullHook func(session store.CopilotSession, pr store.PullRequest)

type proc struct {
	cmd     *exec.Cmd
	baseURL string
	token   string // 访问 agent 的 bearer token（本地拉起时随机生成）
	ws      string
	branch  string
	dead    chan struct{}
}

// newRequest 构造访问本地 agent 的请求，并带上访问令牌（防浏览器 CSRF/未授权 RCE）。
func (p *proc) newRequest(ctx context.Context, method, url string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}
	if p.token != "" {
		req.Header.Set("Authorization", "Bearer "+p.token)
	}
	return req, nil
}

// NewManager 创建 Manager。
func NewManager(st *store.Store) *Manager {
	return &Manager{st: st, procs: map[int64]*proc{}, locks: map[int64]*sync.Mutex{}}
}

// SetPullHook 设置会话自动开 PR 后的回调（在 main 里接线通知）。
func (m *Manager) SetPullHook(fn PullHook) {
	m.mu.Lock()
	m.pullHook = fn
	m.mu.Unlock()
}

func (m *Manager) sessionLock(id int64) *sync.Mutex {
	m.mu.Lock()
	defer m.mu.Unlock()
	l := m.locks[id]
	if l == nil {
		l = &sync.Mutex{}
		m.locks[id] = l
	}
	return l
}

// ---- 运行时配置（可替换，不写死）----

// newAgentToken 生成访问本地 agent 的随机 bearer token（32 字节 crypto/rand）。
func newAgentToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// writeAgentKeyFile 把 LLM key 写入 0600 临时文件（0600），由 agent 经
// `LLM_API_KEY_FILE` 读取。避免密钥出现在 agent 环境（/proc/<pid>/environ）
// 并被其派生的 git 等子进程继承。
func writeAgentKeyFile(key string) (string, func(), error) {
	f, err := os.CreateTemp("", "gitdash-agent-key-*")
	if err != nil {
		return "", nil, err
	}
	path := f.Name()
	if _, err := f.WriteString(key); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return "", nil, err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return "", nil, err
	}
	_ = os.Chmod(path, 0o600)
	return path, func() { _ = os.Remove(path) }, nil
}

// agentBinary 解析 agent 可执行文件：显式 env > 与 gitdash 同目录的 agent > PATH。
func agentBinary() (string, error) {
	if p := strings.TrimSpace(os.Getenv("GITDASH_COPILOT_AGENT_BIN")); p != "" {
		return p, nil
	}
	if exe, err := os.Executable(); err == nil {
		cand := filepath.Join(filepath.Dir(exe), "agent")
		if fi, err := os.Stat(cand); err == nil && !fi.IsDir() {
			return cand, nil
		}
	}
	if p, err := exec.LookPath("agent"); err == nil {
		return p, nil
	}
	return "", fmt.Errorf("%w: set GITDASH_COPILOT_AGENT_BIN or install `agent` in PATH", ErrAgentUnavailable)
}

// agentBaseURL 返回外部托管的 agent 地址（GITDASH_COPILOT_AGENT_URL）；
// 未配置时返回空串，表示由 gitdash 按会话拉起本地进程。
func agentBaseURL() string {
	return strings.TrimRight(strings.TrimSpace(os.Getenv("GITDASH_COPILOT_AGENT_URL")), "/")
}

// ---- 工作区 ----

// ensureWorkspace 确保会话工作区存在；首次克隆并切出会话分支。返回 (ws, branch)。
func (m *Manager) ensureWorkspace(session store.CopilotSession) (string, string, error) {
	ws := WorkspacePath(session.Owner, session.Repo, session.ID)
	branch := strings.TrimSpace(session.Branch)
	if branch == "" {
		branch = BranchName(session.ID)
	}
	if fi, err := os.Stat(ws); err == nil && fi.IsDir() {
		excludeAgentState(ws)
		return ws, branch, nil
	}
	if err := os.MkdirAll(filepath.Dir(ws), 0o755); err != nil {
		return "", "", fmt.Errorf("create workspace dir: %w", err)
	}
	if out, err := gitsvc.GitOut("", "clone", "--quiet", gitsvc.RepoPath(session.Owner, session.Repo), ws); err != nil {
		return "", "", fmt.Errorf("clone repo: %w: %s", err, strings.TrimSpace(out))
	}
	if out, err := gitsvc.GitOut(ws, "checkout", "-B", branch); err != nil {
		return "", "", fmt.Errorf("checkout %s: %w: %s", branch, err, strings.TrimSpace(out))
	}
	setGitIdentity(ws)
	excludeAgentState(ws)
	return ws, branch, nil
}

// excludeAgentState 让 agent 自己写在 `.agent/` 下的会话历史不进提交/推送。
func excludeAgentState(ws string) {
	p := filepath.Join(ws, ".git", "info", "exclude")
	data, _ := os.ReadFile(p)
	add := ""
	if !strings.Contains(string(data), ".agent/") {
		add += ".agent/\n"
	}
	if !strings.Contains(string(data), ".agents/") {
		add += ".agents/\n"
	}
	if add == "" {
		return
	}
	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	_, _ = f.WriteString(add)
	_ = f.Close()
}

func setGitIdentity(ws string) {
	_, _ = gitsvc.GitOut(ws, "config", "user.name", "gitdash-copilot")
	_, _ = gitsvc.GitOut(ws, "config", "user.email", "copilot@gitdash.local")
}

// syncUpstream 拉取远端并在工作区干净时把会话分支 rebase 到默认分支之上。
func (m *Manager) syncUpstream(session store.CopilotSession, ws string) {
	if _, err := gitsvc.GitOut(ws, "fetch", "--quiet", "origin"); err != nil {
		return
	}
	def, err := gitsvc.HeadBranch(session.Owner, session.Repo)
	if err != nil || def == "" {
		return
	}
	if out, err := gitsvc.GitOut(ws, "status", "--porcelain"); err != nil || strings.TrimSpace(out) != "" {
		return
	}
	if _, err := gitsvc.GitOut(ws, "rebase", "--quiet", "origin/"+def); err != nil {
		_, _ = gitsvc.GitOut(ws, "rebase", "--abort")
	}
}

// commitAndPush 提交工作区改动并推送到会话分支（闭环）。
func (m *Manager) commitAndPush(ws, branch, prompt string) (string, error) {
	status, err := gitsvc.GitOut(ws, "status", "--porcelain")
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(status) != "" {
		if _, err := gitsvc.GitOut(ws, "add", "-A"); err != nil {
			return "", err
		}
		if _, err := gitsvc.GitOut(ws, "commit", "-m", commitMessage(prompt)); err != nil {
			return "", err
		}
	}
	sha, _ := gitsvc.GitOut(ws, "rev-parse", "HEAD")
	sha = strings.TrimSpace(sha)
	if _, err := gitsvc.GitOut(ws, "push", "--quiet", "origin", "HEAD:refs/heads/"+branch); err != nil {
		return sha, fmt.Errorf("push %s: %w", branch, err)
	}
	return sha, nil
}

// ---- 运行时生命周期 ----

// ensureRuntime 返回会话的 agent 运行时（必要时拉起本地进程并健康检查）。
func (m *Manager) ensureRuntime(ctx context.Context, session store.CopilotSession) (*proc, error) {
	if p := m.getProc(session.ID); p != nil {
		return p, nil
	}
	ws, branch, err := m.ensureWorkspace(session)
	if err != nil {
		return nil, err
	}

	// 外部托管：直接使用给定地址，不管理工作区进程。
	if base := agentBaseURL(); base != "" {
		p := &proc{baseURL: base, ws: ws, branch: branch, dead: make(chan struct{})}
		m.setProc(session.ID, p)
		return p, nil
	}

	secret, err := m.st.GetByokSecret(session.CreatedBy, session.ByokID)
	if err != nil || strings.TrimSpace(secret.APIKey) == "" {
		return nil, ErrByokMissing
	}
	bin, err := agentBinary()
	if err != nil {
		return nil, err
	}
	port, err := freePort()
	if err != nil {
		return nil, err
	}
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	// 进程级随机访问令牌：只经环境变量下发，浏览器无法读取，阻断 CSRF/本地未授权驱动 shell/fs。
	agentToken, err := newAgentToken()
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(bin, "-C", ws, "-api-addr", addr)
	cmd.Dir = ws
	provider := firstNonEmpty(secret.Provider, "anthropic")
	// 防止已保存/被篡改的 base_url 指向内网：会话启动前再做一次 SSRF 校验。
	baseURL, err := ValidateBaseURL(provider, secret.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrByokMissing, err)
	}
	authStyle := "both"
	if spec, ok := Provider(provider); ok && spec.AuthStyle != "" {
		authStyle = spec.AuthStyle
	}
	keyFile, cleanupKey, err := writeAgentKeyFile(secret.APIKey)
	if err != nil {
		return nil, err
	}
	defer cleanupKey()
	cmd.Env = append(os.Environ(),
		"AGENT_API_TOKEN="+agentToken,
		"LLM_API_KEY_FILE="+keyFile,
		"LLM_BASE_URL="+baseURL,
		"LLM_MODEL="+EffectiveModel(provider, secret.Model),
		"LLM_PROVIDER="+provider,
		"LLM_AUTH_STYLE="+authStyle,
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("%w: start agent: %w", ErrAgentUnavailable, err)
	}

	p := &proc{cmd: cmd, baseURL: "http://" + addr, token: agentToken, ws: ws, branch: branch, dead: make(chan struct{})}
	go func() {
		_ = cmd.Wait()
		close(p.dead)
		m.mu.Lock()
		if m.procs[session.ID] == p {
			delete(m.procs, session.ID)
		}
		m.mu.Unlock()
	}()

	if err := waitHealthy(ctx, p.baseURL, agentToken, 20*time.Second); err != nil {
		_ = cmd.Process.Kill()
		return nil, fmt.Errorf("%w: %w: %s", ErrAgentUnavailable, err, strings.TrimSpace(stderr.String()))
	}
	logx.Infof("copilot audit: AGENT START session=%d repo=%s/%s addr=%s bin=%s",
		session.ID, session.Owner, session.Repo, addr, bin)
	m.setProc(session.ID, p)
	return p, nil
}

func (m *Manager) getProc(id int64) *proc {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := m.procs[id]
	if p == nil {
		return nil
	}
	select {
	case <-p.dead:
		delete(m.procs, id)
		return nil
	default:
		return p
	}
}

func (m *Manager) setProc(id int64, p *proc) {
	m.mu.Lock()
	m.procs[id] = p
	m.mu.Unlock()
}

// Stop 停止会话的 agent 进程（保留工作区与对话历史）。
func (m *Manager) Stop(session store.CopilotSession) {
	m.mu.Lock()
	p := m.procs[session.ID]
	delete(m.procs, session.ID)
	m.mu.Unlock()
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return
	}
	logx.Infof("copilot audit: AGENT STOP session=%d repo=%s/%s", session.ID, session.Owner, session.Repo)
	_ = p.cmd.Process.Kill()
}

// RemoveWorkspace 停止进程并删除工作区（删除会话时调用）。
func (m *Manager) RemoveWorkspace(session store.CopilotSession) error {
	m.Stop(session)
	m.mu.Lock()
	delete(m.locks, session.ID)
	m.mu.Unlock()
	if workspacesDir == "" {
		return nil
	}
	return os.RemoveAll(WorkspacePath(session.Owner, session.Repo, session.ID))
}

// Cancel 取消会话正在运行的一轮（经 agent 的 /api/cancel）。
func (m *Manager) Cancel(ctx context.Context, session store.CopilotSession) {
	p := m.getProc(session.ID)
	if p == nil {
		return
	}
	in, _ := json.Marshal(map[string]string{"session": sessionName(session.ID)})
	req, err := p.newRequest(ctx, http.MethodPost, p.baseURL+"/api/cancel", bytes.NewReader(in))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}
}

// ---- 对话 ----

// RunTurn 执行一轮对话：确保运行时 → 同步上游 → 转发 SSE → 收到 done 后提交推送。
func (m *Manager) RunTurn(ctx context.Context, session store.CopilotSession, text string, emit func(Event)) error {
	lock := m.sessionLock(session.ID)
	lock.Lock()
	defer lock.Unlock()

	p, err := m.ensureRuntime(ctx, session)
	if err != nil {
		return err
	}
	if p.cmd != nil {
		m.syncUpstream(session, p.ws)
	}

	body, _ := json.Marshal(map[string]string{"session": sessionName(session.ID), "text": text})
	req, err := p.newRequest(ctx, http.MethodPost, p.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("agent chat: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return fmt.Errorf("agent chat: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}

	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64<<10), 4<<20)
	sawDone := false
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var ev Event
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &ev); err != nil {
			continue
		}
		if ev.Type == EventDone {
			sawDone = true
			if sha, pushErr := m.commitAndPush(p.ws, p.branch, text); pushErr != nil {
				logx.Warnf("copilot: push session %d: %v", session.ID, pushErr)
				if emit != nil {
					emit(Event{Type: EventError, Error: "changes not pushed: " + pushErr.Error()})
				}
			} else {
				_ = m.st.SetCopilotSessionGit(session.Owner, session.Repo, session.ID, p.branch, sha)
				m.maybeOpenPull(session, p.branch)
			}
		}
		if emit != nil {
			emit(ev)
		}
	}
	if err := sc.Err(); err != nil {
		if ctx.Err() != nil {
			return context.Canceled
		}
		return fmt.Errorf("agent stream: %w", err)
	}
	if !sawDone {
		if ctx.Err() != nil {
			return context.Canceled
		}
		return errors.New("agent stream ended without done event")
	}
	return nil
}

// Messages 拉取会话历史（经 agent 的 /api/messages）。
func (m *Manager) Messages(ctx context.Context, session store.CopilotSession) ([]ChatMessage, error) {
	p, err := m.ensureRuntime(ctx, session)
	if err != nil {
		return nil, err
	}
	url := p.baseURL + "/api/messages?session=" + sessionName(session.ID)
	req, err := p.newRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return nil, fmt.Errorf("agent messages: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	var out struct {
		Messages []ChatMessage `json:"messages"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out.Messages, nil
}

// ---- 小工具 ----

// maybeOpenPull 在会话关联了 issue、且尚未开出 PR 时，把会话分支自动开成 PR。
// 已有同源分支的 open PR 时复用它（幂等）。任何失败都只记日志，不影响闭环。
func (m *Manager) maybeOpenPull(session store.CopilotSession, branch string) {
	if session.IssueNumber <= 0 || session.PRNumber > 0 {
		return
	}
	owner, repo := session.Owner, session.Repo
	if existing, err := m.st.ListOpenPullsBySource(owner, repo, branch); err == nil && len(existing) > 0 {
		_ = m.st.SetCopilotSessionPR(owner, repo, session.ID, existing[0].Number)
		return
	}
	target, err := gitsvc.HeadBranch(owner, repo)
	if err != nil || target == "" || target == branch {
		return
	}
	baseSHA, err := gitsvc.RevSHA(owner, repo, "refs/heads/"+target)
	if err != nil {
		return
	}
	srcSHA, err := gitsvc.RevSHA(owner, repo, "refs/heads/"+branch)
	if err != nil || srcSHA == baseSHA {
		return
	}
	issue, err := m.st.GetIssue(owner, repo, session.IssueNumber)
	if err != nil {
		return
	}
	title := "fix: " + strings.TrimSpace(issue.Title)
	if r := []rune(title); len(r) > 120 {
		title = string(r[:120]) + "…"
	}
	body := fmt.Sprintf("Closes #%d\n\n由 Copilot 会话 #%d 自动生成。", issue.Number, session.ID)
	pr, err := m.st.CreatePull(owner, repo, session.CreatedBy, title, body, branch, target, baseSHA, srcSHA, false)
	if err != nil {
		logx.Warnf("copilot: auto PR session %d: %v", session.ID, err)
		return
	}
	_ = m.st.SetCopilotSessionPR(owner, repo, session.ID, pr.Number)
	logx.Infof("copilot audit: AUTO PR session=%d issue=#%d pr=#%d", session.ID, issue.Number, pr.Number)
	if hook := m.getPullHook(); hook != nil {
		hook(session, pr)
	}
}

func (m *Manager) getPullHook() PullHook {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.pullHook
}

func sessionName(id int64) string { return fmt.Sprintf("%d", id) }

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer func() { _ = l.Close() }()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// waitHealthy 轮询 /healthz 直到协议版本匹配或超时。
func waitHealthy(ctx context.Context, baseURL, token string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var last error
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/healthz", nil)
		if err != nil {
			return err
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			var h struct {
				OK       bool   `json:"ok"`
				Protocol string `json:"protocol"`
			}
			if decErr := json.NewDecoder(resp.Body).Decode(&h); decErr == nil {
				_ = resp.Body.Close()
				if h.OK && h.Protocol == ProtocolVersion {
					return nil
				}
				last = fmt.Errorf("unexpected healthz: ok=%v protocol=%q", h.OK, h.Protocol)
			} else {
				_ = resp.Body.Close()
				last = decErr
			}
		} else {
			last = err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
	if last == nil {
		last = errors.New("timeout")
	}
	return last
}

// commitMessage 由用户提示生成简洁的提交标题（单行，最长 72 字符）。
func commitMessage(prompt string) string {
	s := strings.TrimSpace(prompt)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimSpace(s)
	if s == "" {
		s = "copilot update"
	}
	r := []rune(s)
	if len(r) > 72 {
		s = string(r[:72]) + "…"
	}
	return "copilot: " + s
}
