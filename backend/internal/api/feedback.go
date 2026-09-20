package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"gitdash/backend/internal/logx"
)

const (
	feedbackMaxBody        = 8000
	feedbackMaxTitle       = 200
	feedbackMaxPageURL     = 2000
	feedbackMaxSubmissions = 10
	feedbackWindow         = 15 * time.Minute
	feedbackHTTPTimeout    = 15 * time.Second
)

// feedbackSettings 返回管理员配置的反馈开关、目标仓库地址与访问令牌。
func (a *API) feedbackSettings() (enabled bool, repo, token string) {
	return a.store.GetSetting("feedback_enabled") == "1",
		a.store.GetSetting("feedback_repo"),
		a.store.GetSetting("feedback_token")
}

// feedbackRepoFromURL 从管理员配置的仓库 / Issue 地址解析 owner、repo 与 API 基址。
//
// 支持 github.com（→ api.github.com）以及 Gitea / Forgejo 等兼容服务
// （→ <scheme>://<host>/api/v1）。路径只需以 /owner/repo 开头，后缀（如 /issues/1）
// 会被忽略，因此管理员可以粘贴仓库地址或任意 Issue 地址。
func feedbackRepoFromURL(raw string) (owner, repo, apiBase string, ok bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", "", "", false
	}
	segs := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(segs) < 2 || segs[0] == "" || segs[1] == "" {
		return "", "", "", false
	}
	owner = segs[0]
	repo = strings.TrimSuffix(segs[1], ".git")
	if repo == "" {
		return "", "", "", false
	}
	if strings.EqualFold(u.Hostname(), "github.com") {
		apiBase = "https://api.github.com"
	} else {
		apiBase = u.Scheme + "://" + u.Host + "/api/v1"
	}
	return owner, repo, apiBase, true
}

// feedbackConfig 公开反馈功能是否可用（不泄漏仓库地址与令牌）。
//
//	@Summary     反馈功能状态
//	@Description 返回反馈组件是否已由管理员启用并完成配置。
//	@Tags        feedback
//	@Produce     json
//	@Success     200 {object} map[string]any
//	@Router      /feedback [get]
func (a *API) feedbackConfig(w http.ResponseWriter, r *http.Request) {
	enabled, repo, token := a.feedbackSettings()
	_, _, _, valid := feedbackRepoFromURL(repo)
	writeJSON(w, http.StatusOK, map[string]any{"enabled": enabled && valid && token != ""})
}

// feedbackBodyReq 反馈提交体。
type feedbackBodyReq struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	URL   string `json:"url"`
}

// submitFeedback 提交用户反馈：在管理员配置的远端仓库创建 Issue。
//
//	@Summary     提交反馈
//	@Description 使用管理员配置的仓库地址与令牌，在对应仓库创建 Issue（GitHub / Gitea 兼容 API）。登录用户会附带身份信息。
//	@Tags        feedback
//	@Accept      json
//	@Produce     json
//	@Param       body body feedbackBodyReq true "反馈内容"
//	@Success     201 {object} map[string]any
//	@Failure     400 {object} map[string]string
//	@Failure     404 {object} map[string]string
//	@Failure     429 {object} map[string]string
//	@Failure     502 {object} map[string]string
//	@Router      /feedback [post]
func (a *API) submitFeedback(w http.ResponseWriter, r *http.Request) {
	enabled, rawRepo, token := a.feedbackSettings()
	owner, repo, apiBase, ok := feedbackRepoFromURL(rawRepo)
	if !enabled || !ok || token == "" {
		writeCode(w, http.StatusNotFound, "feedback_disabled", "feedback is not enabled")
		return
	}
	var in feedbackBodyReq
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	content := strings.TrimSpace(in.Body)
	if content == "" {
		writeCode(w, http.StatusBadRequest, "empty_feedback", "feedback content is required")
		return
	}
	content = truncateRunes(content, feedbackMaxBody)

	key := a.rateKey("feedback", clientIP(r))
	if !rateLimitDisabled {
		if blocked, err := a.store.RateBlocked(key, feedbackMaxSubmissions); err == nil && blocked {
			writeCode(w, http.StatusTooManyRequests, "too_many_requests", "too many feedback submissions, try again later")
			return
		}
		if err := a.store.RateFail(key, feedbackWindow); err != nil {
			logx.Infof("feedback rate record %s: %v", key, err)
		}
	}

	title := truncateRunes(strings.TrimSpace(in.Title), feedbackMaxTitle)
	if title == "" {
		title = truncateRunes(firstLine(content), feedbackMaxTitle)
	}
	if title == "" {
		title = "Feedback"
	}
	page := truncateRunes(strings.TrimSpace(in.URL), feedbackMaxPageURL)

	issueURL, number, err := createRemoteIssue(r.Context(), apiBase, owner, repo, token, title,
		buildFeedbackBody(content, page, userFrom(r), r.UserAgent()))
	if err != nil {
		logx.Warnf("feedback issue creation failed (%s/%s): %v", owner, repo, err)
		writeCode(w, http.StatusBadGateway, "feedback_failed", "failed to create feedback issue")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"url": issueURL, "number": number})
}

// buildFeedbackBody 组装 Issue 正文：用户内容 + 来源元信息。
func buildFeedbackBody(content, page, user, ua string) string {
	var b strings.Builder
	b.WriteString(content)
	b.WriteString("\n\n---\n_Submitted via gitdash feedback_\n\n")
	if page != "" {
		fmt.Fprintf(&b, "- Page: %s\n", page)
	}
	if user != "" {
		fmt.Fprintf(&b, "- User: %s\n", user)
	} else {
		b.WriteString("- User: anonymous\n")
	}
	if ua != "" {
		fmt.Fprintf(&b, "- User agent: %s\n", ua)
	}
	fmt.Fprintf(&b, "- Time: %s\n", time.Now().UTC().Format(time.RFC3339))
	return b.String()
}

// createRemoteIssue 通过 GitHub / Gitea 兼容的 REST API 创建 Issue，返回其网页地址与编号。
func createRemoteIssue(ctx context.Context, apiBase, owner, repo, token, title, body string) (string, int, error) {
	payload, err := json.Marshal(map[string]any{"title": title, "body": body})
	if err != nil {
		return "", 0, err
	}
	endpoint := fmt.Sprintf("%s/repos/%s/%s/issues", strings.TrimRight(apiBase, "/"),
		url.PathEscape(owner), url.PathEscape(repo))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("User-Agent", "gitdash")

	client := &http.Client{Timeout: feedbackHTTPTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", 0, fmt.Errorf("remote issue API returned %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	var out struct {
		HTMLURL string `json:"html_url"`
		Number  int    `json:"number"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil {
		return "", 0, err
	}
	return out.HTMLURL, out.Number, nil
}

// firstLine 取文本首行（用于从反馈内容推导 Issue 标题）。
func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

// truncateRunes 按字符数截断，避免在多字节字符中间切断。
func truncateRunes(s string, max int) string {
	if max <= 0 {
		return s
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}
