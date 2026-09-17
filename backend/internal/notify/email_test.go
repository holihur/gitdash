package notify

import (
	"strings"
	"testing"
)

func TestPushEmailsDefaultOff(t *testing.T) {
	t.Setenv("GITDASH_EMAIL_PUSH", "")
	if PushEmails() {
		t.Fatal("push emails should be off by default")
	}
	for _, v := range []string{"1", "true", "YES", "on"} {
		t.Setenv("GITDASH_EMAIL_PUSH", v)
		if !PushEmails() {
			t.Fatalf("GITDASH_EMAIL_PUSH=%q should enable push emails", v)
		}
	}
	t.Setenv("GITDASH_EMAIL_PUSH", "0")
	if PushEmails() {
		t.Fatal("GITDASH_EMAIL_PUSH=0 should disable push emails")
	}
}

func TestShortRef(t *testing.T) {
	cases := map[string]string{
		"refs/heads/main": "main",
		"refs/tags/v1.0":  "v1.0",
		"feature/x":       "feature/x",
		"":                "(unknown)",
	}
	for in, want := range cases {
		if got := shortRef(in); got != want {
			t.Errorf("shortRef(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBuildMessageHeaders(t *testing.T) {
	msg := string(buildMessage("a@b.c", "x@y.z", "subj", "body", Headers{
		MessageID: "<mid@x>",
		ReplyTo:   "reply+t@x",
	}))
	for _, want := range []string{"Message-ID: <mid@x>\r\n", "Reply-To: reply+t@x\r\n", "Subject: subj\r\n"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message missing %q:\n%s", want, msg)
		}
	}
	plain := string(buildMessage("a@b.c", "x@y.z", "subj", "body", Headers{}))
	if strings.Contains(plain, "Reply-To:") || strings.Contains(plain, "Message-ID:") {
		t.Errorf("plain message should not contain optional headers:\n%s", plain)
	}
}
