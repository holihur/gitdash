package totp

import (
	"strings"
	"testing"
	"time"
)

// RFC 6238 SHA1 测试向量（ASCII "12345678901234567890"，
// base32 后与官方文档中 secret 一致），8 位输出取后 6 位。
func TestRFC6238Vectors(t *testing.T) {
	secret := "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
	cases := []struct {
		unix int64
		want string
	}{
		{59, "287082"},         // 94287082
		{1111111109, "081804"}, // 07081804
		{1111111111, "050471"}, // 14050471
		{1234567890, "005924"}, // 89005924
		{2000000000, "279037"}, // 69279037
		{20000000000, "353130"},
	}
	for _, c := range cases {
		got, err := Code(secret, time.Unix(c.unix, 0))
		if err != nil {
			t.Fatalf("Code(%d): %v", c.unix, err)
		}
		if got != c.want {
			t.Errorf("t=%d: got %s want %s", c.unix, got, c.want)
		}
	}
}

func TestVerifyAndSecretRoundtrip(t *testing.T) {
	secret, err := GenerateSecret()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	code, err := Code(secret, now)
	if err != nil {
		t.Fatal(err)
	}
	if !Verify(secret, code, 1) {
		t.Fatal("valid code rejected")
	}
	if Verify(secret, "000000", 1) {
		t.Fatal("invalid code accepted")
	}
	if Verify(secret, "12345", 0) || Verify("", code, 1) {
		t.Fatal("malformed inputs accepted")
	}
	uri := URI("gitdash", "alice", secret)
	if uri[:15] != "otpauth://totp/" || !strings.Contains(uri, "issuer=gitdash") || !strings.Contains(uri, "secret=") {
		t.Fatalf("bad uri: %s", uri)
	}
}
