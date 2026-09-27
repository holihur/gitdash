package updater

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"testing"

	"golang.org/x/crypto/blake2b"
)

// makeMinisignFixture 生成一个 minisign 公钥与 message 的合法签名（预哈希模式）。
func makeMinisignFixture(t *testing.T, message []byte) (pubKey, sigFile string, keyID []byte) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	keyID = make([]byte, 8)
	if _, err := rand.Read(keyID); err != nil {
		t.Fatal(err)
	}
	pubBlob := append([]byte("Ed"), append(keyID, pub...)...)
	pubKey = "untrusted comment: minisign public key TEST\n" + base64.StdEncoding.EncodeToString(pubBlob) + "\n"

	h := blake2b.Sum512(message)
	sig := ed25519.Sign(priv, h[:])
	sigBlob := append([]byte("ED"), append(keyID, sig...)...)
	sigFile = "untrusted comment: signature from minisign secret key\n" +
		base64.StdEncoding.EncodeToString(sigBlob) + "\n" +
		"trusted comment: timestamp:1\tfile:checksums.txt\n" +
		base64.StdEncoding.EncodeToString(make([]byte, 64)) + "\n"
	return pubKey, sigFile, keyID
}

func TestVerifyMinisignRoundTrip(t *testing.T) {
	msg := []byte("abc123  gitdash_0.9.0_linux_amd64.tar.gz\n")
	pub, sig, _ := makeMinisignFixture(t, msg)
	if err := VerifyMinisign(msg, []byte(sig), pub); err != nil {
		t.Fatalf("valid signature rejected: %v", err)
	}
	// 仅接受原始 base64 公钥
	rawPub := pub
	if i := len("untrusted comment: minisign public key TEST\n"); i < len(pub) {
		rawPub = pub[i:]
	}
	if err := VerifyMinisign(msg, []byte(sig), rawPub); err != nil {
		t.Fatalf("raw base64 pubkey rejected: %v", err)
	}
}

func TestVerifyMinisignRejectsTampering(t *testing.T) {
	msg := []byte("abc123  gitdash_0.9.0_linux_amd64.tar.gz\n")
	pub, sig, _ := makeMinisignFixture(t, msg)

	if err := VerifyMinisign([]byte("evil  gitdash_0.9.0_linux_amd64.tar.gz\n"), []byte(sig), pub); err == nil {
		t.Fatal("tampered message accepted")
	}

	// 另一个 key 签发的签名：key id 不匹配
	otherPub, otherSig, _ := makeMinisignFixture(t, msg)
	if err := VerifyMinisign(msg, []byte(otherSig), pub); err == nil {
		t.Fatal("signature from another key accepted")
	}
	if err := VerifyMinisign(msg, []byte(sig), otherPub); err == nil {
		t.Fatal("verification against another public key accepted")
	}

	if err := VerifyMinisign(msg, []byte("untrusted comment: x\nnot-base64\n"), pub); err == nil {
		t.Fatal("malformed signature accepted")
	}
}

func TestSignatureGate(t *testing.T) {
	t.Setenv("GITDASH_UPDATE_MINISIGN_PUBKEY", "")
	t.Setenv("GITDASH_UPDATE_REQUIRE_SIGNATURE", "")
	if signatureRequired() {
		t.Fatal("signature must not be required without a key or explicit flag")
	}
	t.Setenv("GITDASH_UPDATE_REQUIRE_SIGNATURE", "1")
	if !signatureRequired() {
		t.Fatal("explicit require flag must force signature verification")
	}
	t.Setenv("GITDASH_UPDATE_MINISIGN_PUBKEY", "RWQf6LRCGA9i53mlYecO4IzT51TGPpvWucNSCh1CBM0QTaLn73Y7GFO3")
	if !signatureRequired() {
		t.Fatal("configured minisign key must force signature verification")
	}
}
