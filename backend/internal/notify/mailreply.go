// mailreply.go 邮件回复（reply-by-email）的无状态路由 token。
//
// 出站通知邮件的 Reply-To 形如：
//
//	reply+<payload>.<sig>@<GITDASH_MAIL_REPLY_DOMAIN>
//
// 其中 payload 是 base64url(无填充) 编码的
// `owner|repo|kind|number|username|exp`，sig 是 HMAC-SHA256(secret, payload)
// 截断前 10 字节后的 base64url(无填充)。入站侧只用 token 即可还原
// owner/repo/kind/number/username，无需服务端存储，故可水平扩展。
package notify

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"os"
	"strconv"
	"strings"
	"time"
)

// ReplyTokenTTL 回复 token 的有效期。
const ReplyTokenTTL = 90 * 24 * time.Hour

const replyLocalPrefix = "reply+"

// ErrBadReplyToken 表示 token 缺失、被篡改或已过期。
var ErrBadReplyToken = errors.New("invalid reply token")

// MailSecret 返回邮件回复 token 的签名密钥。
// 优先 GITDASH_MAIL_SECRET，其次复用 GITDASH_SECRET_KEY；都未设置时返回空
// （此时不生成 Reply-To，reply-by-email 视为未启用）。
func MailSecret() string {
	if s := strings.TrimSpace(os.Getenv("GITDASH_MAIL_SECRET")); s != "" {
		return s
	}
	return strings.TrimSpace(os.Getenv("GITDASH_SECRET_KEY"))
}

// MailInboundSecret 返回入站邮件端点的共享密钥。
// 优先 GITDASH_MAIL_INBOUND_SECRET，其次回退到 MailSecret()（便于单密钥部署）。
func MailInboundSecret() string {
	if s := strings.TrimSpace(os.Getenv("GITDASH_MAIL_INBOUND_SECRET")); s != "" {
		return s
	}
	return MailSecret()
}

// MailReplyDomain 返回生成 Reply-To 使用的域名（未配置时为空）。
func MailReplyDomain() string {
	return strings.TrimSpace(os.Getenv("GITDASH_MAIL_REPLY_DOMAIN"))
}

func signBody(secret, body string) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(body))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil)[:10])
}

// SignReplyToken 为一条通知生成回复路由 token；secret 为空时返回空串。
func SignReplyToken(secret, owner, repo, kind string, number int64, username string) string {
	if secret == "" || owner == "" || repo == "" || username == "" {
		return ""
	}
	if kind != "issue" && kind != "pull" {
		return ""
	}
	payload := strings.Join([]string{
		owner, repo, kind, strconv.FormatInt(number, 10), username,
		strconv.FormatInt(time.Now().Add(ReplyTokenTTL).Unix(), 10),
	}, "|")
	body := base64.RawURLEncoding.EncodeToString([]byte(payload))
	return body + "." + signBody(secret, body)
}

// ParseReplyToken 验签并还原路由信息；任何异常都返回 ErrBadReplyToken。
func ParseReplyToken(secret, token string) (owner, repo, kind string, number int64, username string, err error) {
	if secret == "" {
		return "", "", "", 0, "", ErrBadReplyToken
	}
	i := strings.LastIndexByte(token, '.')
	if i <= 0 || i == len(token)-1 {
		return "", "", "", 0, "", ErrBadReplyToken
	}
	body, sig := token[:i], token[i+1:]
	if !hmac.Equal([]byte(signBody(secret, body)), []byte(sig)) {
		return "", "", "", 0, "", ErrBadReplyToken
	}
	raw, decErr := base64.RawURLEncoding.DecodeString(body)
	if decErr != nil {
		return "", "", "", 0, "", ErrBadReplyToken
	}
	parts := strings.Split(string(raw), "|")
	if len(parts) != 6 {
		return "", "", "", 0, "", ErrBadReplyToken
	}
	number, convErr := strconv.ParseInt(parts[3], 10, 64)
	if convErr != nil || number < 1 {
		return "", "", "", 0, "", ErrBadReplyToken
	}
	exp, convErr := strconv.ParseInt(parts[5], 10, 64)
	if convErr != nil || time.Now().Unix() > exp {
		return "", "", "", 0, "", ErrBadReplyToken
	}
	kind = parts[2]
	if kind != "issue" && kind != "pull" {
		return "", "", "", 0, "", ErrBadReplyToken
	}
	return parts[0], parts[1], kind, number, parts[4], nil
}

// ReplyAddress 拼出 reply+token@domain；token 或 domain 为空时返回空串。
func ReplyAddress(domain, token string) string {
	if token == "" || domain == "" {
		return ""
	}
	return replyLocalPrefix + token + "@" + domain
}

// MessageID 为一条通知生成 RFC 5322 Message-ID（同一主题的邮件共享前缀，便于线程）。
func MessageID(domain, owner, repo, kind string, number int64) string {
	if domain == "" {
		domain = "gitdash.local"
	}
	var rnd [6]byte
	if _, err := rand.Read(rnd[:]); err != nil {
		rnd = [6]byte{0xde, 0xad, 0xbe, 0xef, 0x00, 0x01}
	}
	return "<gitdash." + kind + "." + owner + "." + repo + "." +
		strconv.FormatInt(number, 10) + "." + hex.EncodeToString(rnd[:]) + "@" + domain + ">"
}
