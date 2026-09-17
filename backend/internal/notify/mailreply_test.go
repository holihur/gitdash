package notify

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

func TestReplyTokenRoundTrip(t *testing.T) {
	tok := SignReplyToken("s3cret", "acme", "web", "pull", 42, "alice")
	if tok == "" {
		t.Fatal("expected token")
	}
	owner, repo, kind, number, username, err := ParseReplyToken("s3cret", tok)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if owner != "acme" || repo != "web" || kind != "pull" || number != 42 || username != "alice" {
		t.Fatalf("unexpected payload: %s/%s %s#%d %s", owner, repo, kind, number, username)
	}
}

func TestReplyTokenRejectsTampering(t *testing.T) {
	tok := SignReplyToken("s3cret", "acme", "web", "issue", 1, "alice")
	body, _, _ := strings.Cut(tok, ".")
	if _, _, _, _, _, err := ParseReplyToken("s3cret", body+".AAAAAAAAAAAAAA"); err == nil {
		t.Fatal("expected tampered signature to fail")
	}
	if _, _, _, _, _, err := ParseReplyToken("other", tok); err == nil {
		t.Fatal("expected wrong secret to fail")
	}
	if _, _, _, _, _, err := ParseReplyToken("s3cret", "garbage"); err == nil {
		t.Fatal("expected malformed token to fail")
	}
}

func TestReplyTokenExpiry(t *testing.T) {
	// 手工构造一个已过期的 token。
	tok := SignReplyToken("s3cret", "acme", "web", "issue", 1, "alice")
	body, _, _ := strings.Cut(tok, ".")
	// 找到 payload 里的 exp，替换为过去时间后重新签名。
	raw, err := base64.RawURLEncoding.DecodeString(body)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	parts := strings.Split(string(raw), "|")
	parts[5] = "1"
	expiredBody := base64.RawURLEncoding.EncodeToString([]byte(strings.Join(parts, "|")))
	expired := expiredBody + "." + signBody("s3cret", expiredBody)
	if _, _, _, _, _, err := ParseReplyToken("s3cret", expired); err == nil {
		t.Fatal("expected expired token to fail")
	}
}

func TestReplyAddressAndMessageID(t *testing.T) {
	if got := ReplyAddress("mail.example", "abc"); got != "reply+abc@mail.example" {
		t.Fatalf("reply address = %q", got)
	}
	if got := ReplyAddress("", "abc"); got != "" {
		t.Fatalf("empty domain should yield empty address, got %q", got)
	}
	mid := MessageID("mail.example", "acme", "web", "pull", 7)
	if !strings.HasPrefix(mid, "<gitdash.pull.acme.web.7.") || !strings.HasSuffix(mid, "@mail.example>") {
		t.Fatalf("message id = %q", mid)
	}
	if !strings.Contains(mid, "@") || strings.Contains(mid, " ") {
		t.Fatalf("message id malformed: %q", mid)
	}
}

func TestMailSecretFallsBackToSecretKey(t *testing.T) {
	t.Setenv("GITDASH_MAIL_SECRET", "")
	t.Setenv("GITDASH_SECRET_KEY", "fallback")
	if MailSecret() != "fallback" {
		t.Fatal("expected fallback to GITDASH_SECRET_KEY")
	}
	t.Setenv("GITDASH_MAIL_SECRET", "primary")
	if MailSecret() != "primary" {
		t.Fatal("expected GITDASH_MAIL_SECRET to win")
	}
}

func TestReplyTokenTTLIsFuture(t *testing.T) {
	if ReplyTokenTTL <= 0 || time.Now().Add(ReplyTokenTTL).Before(time.Now()) {
		t.Fatal("invalid TTL")
	}
}
