package api

import (
	"crypto/subtle"
	"errors"
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
	// 以下为可选的邮件线程/幂等字段（MTA 适配层尽量回传）。
	MessageID  string `json:"message_id"`
	InReplyTo  string `json:"in_reply_to"`
	References string `json:"references"`
}

// lastMessageRef 从 References 头（空白分隔的 Message-ID 列表）取最后一个作为线程父节点。
func lastMessageRef(refs string) string {
	fields := strings.Fields(refs)
	if len(fields) == 0 {
		return ""
	}
	return fields[len(fields)-1]
}

// patchAddressParts 解析 patch 邮件地址 `patches+<owner>+<repo>@domain`。
// 不匹配时返回 ok=false。
func patchAddressParts(addr string) (owner, repo string, ok bool) {
	at := strings.LastIndexByte(addr, '@')
	if at <= 0 {
		return "", "", false
	}
	parts := strings.Split(addr[:at], "+")
	if len(parts) != 3 || !strings.EqualFold(parts[0], "patches") {
		return "", "", false
	}
	owner, repo = strings.TrimSpace(parts[1]), strings.TrimSpace(parts[2])
	if owner == "" || repo == "" {
		return "", "", false
	}
	return owner, repo, true
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
	addr := inboundMailAddress(in.To)
	// patch-by-email：`patches+<owner>+<repo>@domain` 且 Subject 含 [PATCH]。
	if owner, repo, ok := patchAddressParts(addr); ok {
		a.inboundPatch(w, r, owner, repo, in)
		return
	}
	token := replyTokenFromAddress(addr)
	if token == "" {
		writeCode(w, http.StatusBadRequest, "invalid_to", "to address is not a reply or patch address")
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
	// 线程/幂等：优先用入站邮件自身的 Message-ID；重复投递直接返回已有评论。
	messageID := strings.TrimSpace(in.MessageID)
	if messageID == "" {
		messageID = notify.MessageID(notify.MailReplyDomain(), owner, repo, kind, number)
	}
	if existing, ok := a.store.CommentByMessageID(messageID); ok {
		writeJSON(w, http.StatusOK, existing)
		return
	}
	inReplyTo := strings.TrimSpace(in.InReplyTo)
	if inReplyTo == "" {
		inReplyTo = lastMessageRef(in.References)
	}
	comment, err := a.store.CreateCommentMeta(owner, repo, kind, number, username, body, nil, messageID, inReplyTo)
	if err != nil {
		internalError(w, err)
		return
	}
	summary := []rune(body)
	if len(summary) > 200 {
		summary = summary[:200]
	}
	a.notifyMessage(owner, repo, kind, "commented", username, number, title, string(summary), messageID)
	logx.Infof("mail: reply comment by %s on %s/%s#%d", username, owner, repo, number)
	writeJSON(w, http.StatusCreated, comment)
}

// inboundPatch 处理发往 `patches+<owner>+<repo>@domain` 的补丁邮件：
// 发件人邮箱映射为有写权限的用户，正文作为 mbox 应用并自动开 PR。
// 发件人身份依赖 MTA 对 From 的解析与共享密钥保护，不做 DKIM 校验。
func (a *API) inboundPatch(w http.ResponseWriter, r *http.Request, owner, repo string, in mailInboundReq) {
	if !strings.Contains(strings.ToUpper(in.Subject), "[PATCH") {
		writeCode(w, http.StatusBadRequest, "not_a_patch", "subject does not contain [PATCH]")
		return
	}
	from := strings.TrimSpace(inboundMailAddress(in.From))
	if from == "" {
		writeCode(w, http.StatusBadRequest, "invalid_from", "from address is required")
		return
	}
	user, err := a.store.GetByEmail(from)
	if err != nil {
		writeCode(w, http.StatusBadRequest, "unknown_sender", "sender email does not match a user")
		return
	}
	if user.Banned {
		writeCode(w, http.StatusForbidden, "sender_banned", "sender is banned")
		return
	}
	if !user.EmailVerified {
		writeCode(w, http.StatusForbidden, "sender_unverified", "sender email is not verified")
		return
	}
	info, err := a.store.GetRepo(owner, repo)
	if err != nil || info.Banned || a.store.IsOrgBanned(owner) {
		writeNotFound(w, "repo")
		return
	}
	if !a.store.CanWrite(owner, repo, user.Username) {
		writeNotFound(w, "repo")
		return
	}
	data := []byte(in.Text)
	if len(strings.TrimSpace(string(data))) == 0 {
		writeCode(w, http.StatusBadRequest, "empty_patch", "patch series is empty")
		return
	}
	pr, err := a.createPatchPR(owner, repo, user.Username, "", "", data)
	if err != nil {
		if errors.Is(err, errEmptyPatch) {
			writeCode(w, http.StatusBadRequest, "empty_patch", err.Error())
			return
		}
		writeCode(w, http.StatusBadRequest, "patch_failed", err.Error())
		return
	}
	a.notify(owner, repo, "pull", "opened", user.Username, pr.Number, pr.Title, "")
	logx.Infof("mail: patch PR #%d by %s on %s/%s", pr.Number, user.Username, owner, repo)
	writeJSON(w, http.StatusCreated, pr)
}
