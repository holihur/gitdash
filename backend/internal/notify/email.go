// Package notify SMTP 邮件通知：环境变量配置，未配置时一切为 no-op。
package notify

import (
	"fmt"
	"net/smtp"
	"os"
	"strings"

	"gitdash/backend/internal/logx"
	"gitdash/backend/internal/store"
	"gitdash/backend/internal/webhooks"
)

// Sender SMTP 发送器（nil 表示未配置，no-op）。
type Sender struct {
	host string
	port string
	user string
	pass string
	from string

	// replyDomain 配置后，通知邮件带上 Reply-To，支持 reply-by-email。
	replyDomain string
	// mailSecret 用于签名回复路由 token（为空则不带 Reply-To）。
	mailSecret string
}

// NewSender 从环境变量构建发送器；GITDASH_SMTP_HOST 未设置时返回 nil（no-op）。
func NewSender() *Sender {
	host := strings.TrimSpace(os.Getenv("GITDASH_SMTP_HOST"))
	if host == "" {
		return nil
	}
	port := os.Getenv("GITDASH_SMTP_PORT")
	if port == "" {
		port = "587"
	}
	from := os.Getenv("GITDASH_SMTP_FROM")
	if from == "" && os.Getenv("GITDASH_SMTP_USER") != "" {
		from = os.Getenv("GITDASH_SMTP_USER")
	}
	return &Sender{
		host:        host,
		port:        port,
		user:        os.Getenv("GITDASH_SMTP_USER"),
		pass:        os.Getenv("GITDASH_SMTP_PASS"),
		from:        from,
		replyDomain: MailReplyDomain(),
		mailSecret:  MailSecret(),
	}
}

// Headers 可选邮件头（Message-ID / Reply-To），零值表示不写入。
type Headers struct {
	MessageID string
	ReplyTo   string
}

// Send 发送纯文本邮件（无附加头）。
func (s *Sender) Send(to, subject, body string) error {
	return s.SendWithHeaders(to, subject, body, Headers{})
}

// SendWithHeaders 发送纯文本邮件，可携带 Message-ID / Reply-To 头。
func (s *Sender) SendWithHeaders(to, subject, body string, h Headers) error {
	addr := s.host + ":" + s.port
	msg := buildMessage(s.from, to, subject, body, h)
	var auth smtp.Auth
	if s.user != "" {
		auth = smtp.PlainAuth("", s.user, s.pass, s.host)
	}
	return smtp.SendMail(addr, auth, s.from, []string{to}, msg)
}

func buildMessage(from, to, subject, body string, h Headers) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", from)
	fmt.Fprintf(&b, "To: %s\r\n", to)
	if h.MessageID != "" {
		fmt.Fprintf(&b, "Message-ID: %s\r\n", h.MessageID)
	}
	if h.ReplyTo != "" {
		fmt.Fprintf(&b, "Reply-To: %s\r\n", h.ReplyTo)
	}
	fmt.Fprintf(&b, "Subject: %s\r\n", subject)
	b.WriteString("MIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n")
	b.WriteString(body)
	b.WriteString("\r\n")
	return []byte(b.String())
}

// actionText 把动作翻译成邮件文案动词。
func actionText(ev webhooks.Event) string {
	switch ev.Action {
	case "commented":
		return "评论了"
	case "closed":
		return "关闭了"
	case "reopened":
		return "重新打开了"
	case "merged":
		return "合并了"
	default:
		return "打开了"
	}
}

// PushEmails 是否对 push 事件发送邮件（默认关闭；GITDASH_EMAIL_PUSH=1 开启）。
// push 事件没有 issue/PR 编号，因此不生成回复 token。
func PushEmails() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("GITDASH_EMAIL_PUSH"))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// shortRef 把 refs/heads/main 简化为 main（邮件文案用）。
func shortRef(ref string) string {
	ref = strings.TrimPrefix(ref, "refs/heads/")
	ref = strings.TrimPrefix(ref, "refs/tags/")
	if ref == "" {
		return "(unknown)"
	}
	return ref
}

// EmailHandler 返回挂在 webhook 调度器上的消费者：给开启邮件通知的接收者发邮件。
// sender 为 nil 或 push 事件未开启（GITDASH_EMAIL_PUSH）时直接跳过。
func EmailHandler(st *store.Store, sender *Sender) func(webhooks.Event) {
	if sender == nil {
		return func(webhooks.Event) {}
	}
	return func(ev webhooks.Event) {
		if ev.Event == "" {
			return
		}
		if ev.Event == "push" {
			if !PushEmails() {
				return
			}
			sender.sendPush(st, ev)
			return
		}
		users := st.NotifyRecipients(ev.Owner, ev.Repo, ev.Actor)
		targets := st.EmailTargets(users)
		if len(targets) == 0 {
			return
		}
		subject := fmt.Sprintf("[%s/%s#%d] %s", ev.Owner, ev.Repo, ev.Number, ev.Title)
		body := fmt.Sprintf("%s 在 %s/%s#%d %s「%s」", ev.Actor, ev.Owner, ev.Repo, ev.Number, actionText(ev), ev.Title)
		if ev.Comment != "" {
			body += "\n\n" + ev.Comment
		}
		for _, t := range targets {
			h := Headers{}
			if sender.replyDomain != "" && sender.mailSecret != "" {
				// 回复 token 绑定收件人身份：谁收到邮件，谁就能以此身份回复评论。
				tok := SignReplyToken(sender.mailSecret, ev.Owner, ev.Repo, ev.Kind, ev.Number, t.Username)
				h.ReplyTo = ReplyAddress(sender.replyDomain, tok)
				h.MessageID = MessageID(sender.replyDomain, ev.Owner, ev.Repo, ev.Kind, ev.Number)
			}
			if err := sender.SendWithHeaders(t.Email, subject, body, h); err != nil {
				logx.Infof("notify: email to %s: %v", t.Username, err)
			}
		}
	}
}

// sendPush 给 push 事件的关注者发邮件（无回复 token）。
func (s *Sender) sendPush(st *store.Store, ev webhooks.Event) {
	users := st.NotifyRecipients(ev.Owner, ev.Repo, ev.User)
	targets := st.EmailTargets(users)
	if len(targets) == 0 {
		return
	}
	branch := shortRef(ev.Ref)
	subject := fmt.Sprintf("[%s/%s] push to %s", ev.Owner, ev.Repo, branch)
	body := fmt.Sprintf("%s pushed to %s in %s/%s", ev.User, branch, ev.Owner, ev.Repo)
	if ev.New != "" {
		body += "\nhead: " + ev.New
	}
	for _, t := range targets {
		if err := s.Send(t.Email, subject, body); err != nil {
			logx.Infof("notify: push email to %s: %v", t.Username, err)
		}
	}
}
