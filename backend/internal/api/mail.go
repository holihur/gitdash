package api

import (
	"crypto/subtle"
	"net/http"
	"regexp"
	"strings"

	"gitdash/backend/internal/logx"
	"gitdash/backend/internal/notify"
)

// mailInboundReq 入站邮件管道的请求体（MTA / 邮件服务商 webhook 适配层）。
type mailInboundReq struct {
	To      string `json:"to"`
	From    string `json:"from"`
	Subject string `json:"subject"`
	Text    string `json:"text"`
}

// addrAngleRe 从 "Name <a@b>" 中提取 <a@b>。
var addrAngleRe = regexp.MustCompile(`<([^>]+)>`)

// maxInboundBodyLen 入站评论正文的最大字符数（与网页端一致）。
const maxInboundBodyLen = 10000

// inboundMailAddress 从收件人字段提取邮箱地址（去掉显示名与尖括号）。
func inboundMailAddress(to string) string {
	if m := addrAngleRe.FindStringSubmatch(to); len(m) == 2 {
		return strings.TrimSpace(m[1])
	}
	return strings.TrimSpace(to)
}

// replyTokenFromAddress 从 reply+<token>@domain 中取出 token；不匹配返回空。
func replyTokenFromAddress(addr string) string {
	at := strings.LastIndexByte(addr, '@')
	if at <= 0 {
		return ""
	}
	local := addr[:at]
	if !strings.HasPrefix(local, "reply+") {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(local, "reply+"))
}

// cleanReplyBody 清洗邮件正文：去掉引用行、署名与 "On ... wrote:" 引导行，
// 压缩多余空行并去除首尾空白。返回清洗后的正文。
func cleanReplyBody(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))
	for _, ln := range lines {
		trimmed := strings.TrimSpace(ln)
		if trimmed == "--" || strings.HasPrefix(trimmed, "-- ") {
			break // 签名开始
		}
		if strings.HasPrefix(trimmed, ">") {
			continue // 引用行
		}
		// 常见回复引导行："On ... wrote:" / "在 ... 写道："
		if (strings.HasPrefix(trimmed, "On ") || strings.HasPrefix(trimmed, "在 ")) &&
			(strings.HasSuffix(trimmed, "wrote:") || strings.HasSuffix(trimmed, "写道：") || strings.HasSuffix(trimmed, "写道:")) {
			continue
		}
		out = append(out, strings.TrimRight(ln, " \t"))
	}
	// 去掉尾部空行
	for len(out) > 0 && strings.TrimSpace(out[len(out)-1]) == "" {
		out = out[:len(out)-1]
	}
	// 去掉头部空行
	for len(out) > 0 && strings.TrimSpace(out[0]) == "" {
		out = out[1:]
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

// inboundMailAuthorized 校验入站端点共享密钥（常量时间比较）。
// 密钥未配置时端点视为未启用。
func (a *API) inboundMailAuthorized(r *http.Request) bool {
	secret := notify.MailInboundSecret()
	if secret == "" {
		return false
	}
	got := strings.TrimSpace(r.Header.Get("X-Gitdash-Mail-Secret"))
	if got == "" {
		got = bearerToken(r)
	}
	if got == "" {
		got = strings.TrimSpace(r.URL.Query().Get("secret"))
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(secret)) == 1
}

// inboundMail 接收邮件服务商/MTA 管道投递的回复邮件，验签后落库为评论。
//
//	@Summary     入站邮件（reply-by-email）
//	@Description 邮件提供商/MTA 管道调用；凭 X-Gitdash-Mail-Secret 头鉴权，
//	@Description 从收件地址 reply+<token>@domain 还原仓库与 issue/PR，
//	@Description 清洗正文后以 token 绑定用户身份创建评论。
//	@Tags        mail
//	@Accept      json
//	@Produce     json
//	@Param       body body mailInboundReq true "收件人/发件人/主题/正文"
//	@Success     201 {object} store.Comment
//	@Failure     400 {object} map[string]string
//	@Failure     401 {object} map[string]string
//	@Failure     404 {object} map[string]string
//	@Router      /mail/inbound [post]
func (a *API) inboundMail(w http.ResponseWriter, r *http.Request) {
	if !a.inboundMailAuthorized(r) {
		writeCode(w, http.StatusUnauthorized, "unauthorized", "invalid or missing mail inbound secret")
		return
	}
	var in mailInboundReq
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	token := replyTokenFromAddress(inboundMailAddress(in.To))
	if token == "" {
		writeCode(w, http.StatusBadRequest, "invalid_to", "to address is not a reply address")
		return
	}
	owner, repo, kind, number, username, err := notify.ParseReplyToken(notify.MailSecret(), token)
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_token", "reply token is invalid or expired")
		return
	}
	body := cleanReplyBody(in.Text)
	if body == "" {
		writeCode(w, http.StatusBadRequest, "empty_body", "email body is empty after cleanup")
		return
	}
	if len([]rune(body)) > maxInboundBodyLen {
		writeCode(w, http.StatusBadRequest, "body_too_long", "email body too long (max 10000 chars)")
		return
	}
	// 宿主必须存在且未被封禁
	info, err := a.store.GetRepo(owner, repo)
	if err != nil || info.Banned || a.store.IsOrgBanned(owner) {
		writeNotFound(w, "repo")
		return
	}
	var title string
	if kind == "issue" {
		it, e := a.store.GetIssue(owner, repo, number)
		if e != nil {
			writeNotFound(w, "issue")
			return
		}
		title = it.Title
	} else {
		pr, e := a.store.GetPull(owner, repo, number)
		if e != nil {
			writeNotFound(w, "pull")
			return
		}
		title = pr.Title
	}
	comment, err := a.store.CreateComment(owner, repo, kind, number, username, body, nil)
	if err != nil {
		internalError(w, err)
		return
	}
	summary := []rune(body)
	if len(summary) > 200 {
		summary = summary[:200]
	}
	a.notify(owner, repo, kind, "commented", username, number, title, string(summary))
	logx.Infof("mail: reply comment by %s on %s/%s#%d", username, owner, repo, number)
	writeJSON(w, http.StatusCreated, comment)
}
