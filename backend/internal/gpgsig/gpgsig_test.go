package gpgsig

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/armor"
)

// genKey 生成一把测试公钥，返回 armor、指纹与实体。
func genKey(t *testing.T, name, email string) (string, string, *openpgp.Entity) {
	t.Helper()
	e, err := openpgp.NewEntity(name, "", email, nil)
	if err != nil {
		t.Fatalf("new entity: %v", err)
	}
	var buf bytes.Buffer
	w, err := armor.Encode(&buf, openpgp.PublicKeyType, nil)
	if err != nil {
		t.Fatalf("armor: %v", err)
	}
	if err := e.Serialize(w); err != nil {
		t.Fatalf("serialize: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return buf.String(), fingerprintHex(e.PrimaryKey.Fingerprint), e
}

// signedCommit 构造一个带 gpgsig 头的 git commit 原始对象。
func signedCommit(t *testing.T, e *openpgp.Entity, msg string) []byte {
	t.Helper()
	header := "tree " + strings.Repeat("0", 40) + "\n" +
		"author test <test@example.com> 0 +0000\n" +
		"committer test <test@example.com> 0 +0000\n"
	unsigned := header + "\n" + msg + "\n"
	var sig bytes.Buffer
	if err := openpgp.ArmoredDetachSign(&sig, e, strings.NewReader(unsigned), nil); err != nil {
		t.Fatalf("sign: %v", err)
	}
	lines := strings.Split(strings.TrimRight(sig.String(), "\n"), "\n")
	var b strings.Builder
	b.WriteString(header)
	b.WriteString("gpgsig " + lines[0] + "\n")
	for _, ln := range lines[1:] {
		b.WriteString(" " + ln + "\n")
	}
	b.WriteString("\n" + msg + "\n")
	return []byte(b.String())
}

func TestVerifyCommitRegistered(t *testing.T) {
	armor, fp, e := genKey(t, "alice", "alice@example.com")
	otherArmor, otherFP, _ := genKey(t, "bob", "bob@example.com")
	raw := signedCommit(t, e, "hello")
	keys := []Key{
		{Username: "bob", Fingerprint: otherFP, Armor: otherArmor},
		{Username: "alice", Fingerprint: fp, Armor: armor},
	}
	user, gotFP, status := VerifyCommit(raw, keys)
	if status != StatusVerified || user != "alice" || !strings.EqualFold(gotFP, fp) {
		t.Fatalf("got user=%q fp=%q status=%q", user, gotFP, status)
	}
}

func TestVerifyCommitUnknownKey(t *testing.T) {
	_, _, signer := genKey(t, "alice", "alice@example.com")
	armor, fp, _ := genKey(t, "bob", "bob@example.com")
	raw := signedCommit(t, signer, "hello")
	if _, _, status := VerifyCommit(raw, []Key{{Username: "bob", Fingerprint: fp, Armor: armor}}); status != StatusUnknownKey {
		t.Fatalf("status = %q, want unknown_key", status)
	}
}

func TestVerifyCommitTamperedIsInvalid(t *testing.T) {
	armor, fp, e := genKey(t, "alice", "alice@example.com")
	raw := signedCommit(t, e, "hello")
	tampered := bytes.Replace(raw, []byte("hello"), []byte("hacked"), 1)
	if _, _, status := VerifyCommit(tampered, []Key{{Username: "alice", Fingerprint: fp, Armor: armor}}); status != StatusInvalid {
		t.Fatalf("status = %q, want invalid", status)
	}
}

func TestVerifyCommitUnsigned(t *testing.T) {
	if _, _, status := VerifyCommit([]byte("tree x\n\nmsg\n"), nil); status != StatusUnsigned {
		t.Fatalf("status = %q, want unsigned", status)
	}
}
