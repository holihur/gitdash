// Package notify SMTP 邮件通知：环境变量或管理端设置配置，未配置时一切为 no-op。
package notify

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"os"
	"strings"
	"time"

	"gitdash/backend/internal/logx"
	"gitdash/backend/internal/store"
	"gitdash/backend/internal/webhooks"
)

// 管理端 SMTP 设置键（settings 表）。
const (
	SettingEnabled = "smtp_enabled"
	SettingHost    = "smtp_host"
	SettingPort    = "smtp_port"
	SettingUser    = "smtp_user"
	SettingPass    = "smtp_pass"
	SettingFrom    = "smtp_from"
)

// Config SMTP 连接参数。
type Config struct {
	Host string
	Port string
	User string
	Pass string
	From string
}

// EnvConfig 从环境变量读取 SMTP 配置；GITDASH_SMTP_HOST 未设置时返回 ok=false。
func EnvConfig() (Config, bool) {
	host := strings.TrimSpace(os.Getenv("GITDASH_SMTP_HOST"))
	if host == "" {
		return Config{}, false
	}
	port := strings.TrimSpace(os.Getenv("GITDASH_SMTP_PORT"))
	if port == "" {
		port = "587"
	}
	user := os.Getenv("GITDASH_SMTP_USER")
	from := strings.TrimSpace(os.Getenv("GITDASH_SMTP_FROM"))
	if from == "" {
		from = user
	}
	return Config{Host: host, Port: port, User: user, Pass: os.Getenv("GITDASH_SMTP_PASS"), From: from}, true
}

// StoredConfig 读取管理端保存的 SMTP 配置；已启用且 host 非空时返回 ok=true。
func StoredConfig(st *store.Store) (Config, bool) {
	if st == nil || st.GetSetting(SettingEnabled) != "1" {
		return Config{}, false
	}
	cfg := Config{
		Host: strings.TrimSpace(st.GetSetting(SettingHost)),
		Port: strings.TrimSpace(st.GetSetting(SettingPort)),
		User: st.GetSetting(SettingUser),
		Pass: st.GetSetting(SettingPass),
		From: strings.TrimSpace(st.GetSetting(SettingFrom)),
	}
	if cfg.Host == "" {
		return Config{}, false
	}
	if cfg.Port == "" {
		cfg.Port = "587"
	}
	if cfg.From == "" {
		cfg.From = cfg.User
	}
	return cfg, true
}

// ActiveConfig 返回当前生效的 SMTP 配置：管理端启用配置优先，否则回退环境变量。
func ActiveConfig(st *store.Store) (Config, bool) {
	if cfg, ok := StoredConfig(st); ok {
		return cfg, true
	}
	return EnvConfig()
}

// Sender SMTP 发送器（nil 表示未配置，no-op）。
// store 非空时每次发送前解析生效配置，使管理端可运行时修改 SMTP。
type Sender struct {
	cfg    Config
	hasCfg bool
	store  *store.Store

	// replyDomain 配置后，通知邮件带上 Reply-To，支持 reply-by-email。
	replyDomain string
	// mailSecret 用于签名回复路由 token（为空则不带 Reply-To）。
	mailSecret string
}

// NewSender 从环境变量构建发送器；GITDASH_SMTP_HOST 未设置时返回 nil（no-op）。
func NewSender() *Sender {
	cfg, ok := EnvConfig()
	if !ok {
		return nil
	}
	return &Sender{cfg: cfg, hasCfg: true, replyDomain: MailReplyDomain(), mailSecret: MailSecret()}
}

// NewStoreSender 返回始终非 nil 的发送器：每次发送前从 settings 表解析配置
// （管理端可运行时启停 SMTP），未配置时回退环境变量。
func NewStoreSender(st *store.Store) *Sender {
	return &Sender{store: st, replyDomain: MailReplyDomain(), mailSecret: MailSecret()}
}

// resolve 返回当前生效的配置；未配置返回 ok=false。
func (s *Sender) resolve() (Config, bool) {
	if s == nil {
		return Config{}, false
	}
	if s.store != nil {
		return ActiveConfig(s.store)
	}
	return s.cfg, s.hasCfg
}

// Configured 报告当前是否有可用 SMTP 配置。
func (s *Sender) Configured() bool {
	_, ok := s.resolve()
	return ok
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
	cfg, ok := s.resolve()
	if !ok {
		return errors.New("smtp not configured")
	}
	return sendMail(cfg, to, subject, body, h)
}

// sendMail 通过显式 dial 发送邮件（带超时 + STARTTLS），避免不可达 SMTP 拖住请求。
func sendMail(cfg Config, to, subject, body string, h Headers) error {
	addr := net.JoinHostPort(cfg.Host, cfg.Port)
	conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(30 * time.Second))
	client, err := smtp.NewClient(conn, cfg.Host)
	if err != nil {
		return err
	}
	defer client.Close()
	if ok, _ := client.Extension("STARTTLS"); ok {
		if err := client.StartTLS(&tls.Config{ServerName: cfg.Host}); err != nil {
			return err
		}
	}
	if cfg.User != "" {
		if err := client.Auth(smtp.PlainAuth("", cfg.User, cfg.Pass, cfg.Host)); err != nil {
			return err
		}
	}
	if err := client.Mail(cfg.From); err != nil {
		return err
	}
	if err := client.Rcpt(to); err != nil {
		return err
	}
	wc, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := wc.Write(buildMessage(cfg.From, to, subject, body, h)); err != nil {
		return err
	}
	if err := wc.Close(); err != nil {
		return err
	}
	return client.Quit()
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
		if ev.Event == "" || !sender.Configured() {
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
				h.MessageID = ev.MessageID
				if h.MessageID == "" {
					h.MessageID = MessageID(sender.replyDomain, ev.Owner, ev.Repo, ev.Kind, ev.Number)
				}
			}
			if err := sender.SendWithHeaders(t.Email, subject, body, h); err != nil {
				logx.Infof("notify: email to %s: %v", t.Username, err)
			}
		}
	}
}

// sendPush 给 push 事件的关注者发邮件（无回复 token）。
func (s *Sender) sendPush(st *store.Store, ev webhooks.Event) {
	if !s.Configured() {
		return
	}
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
