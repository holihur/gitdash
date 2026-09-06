// Package notify SMTP 邮件通知：环境变量配置，未配置时一切为 no-op。
package notify

import (
	"fmt"
	"log"
	"net/smtp"
	"os"
	"strings"

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
		host: host,
		port: port,
		user: os.Getenv("GITDASH_SMTP_USER"),
		pass: os.Getenv("GITDASH_SMTP_PASS"),
		from: from,
	}
}

// Send 发送纯文本邮件。
func (s *Sender) Send(to, subject, body string) error {
	addr := s.host + ":" + s.port
	msg := buildMessage(s.from, to, subject, body)
	var auth smtp.Auth
	if s.user != "" {
		auth = smtp.PlainAuth("", s.user, s.pass, s.host)
	}
	return smtp.SendMail(addr, auth, s.from, []string{to}, msg)
}

func buildMessage(from, to, subject, body string) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", from)
	fmt.Fprintf(&b, "To: %s\r\n", to)
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

// EmailHandler 返回挂在 webhook 调度器上的消费者：给开启邮件通知的接收者发邮件。
// sender 为 nil 或 push 事件时直接跳过。
func EmailHandler(st *store.Store, sender *Sender) func(webhooks.Event) {
	if sender == nil {
		return func(webhooks.Event) {}
	}
	return func(ev webhooks.Event) {
		if ev.Event == "push" || ev.Event == "" {
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
			if err := sender.Send(t.Email, subject, body); err != nil {
				log.Printf("notify: email to %s: %v", t.Username, err)
			}
		}
	}
}
