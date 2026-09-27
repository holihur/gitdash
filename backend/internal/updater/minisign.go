package updater

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/blake2b"
)

// minisign 公钥 / 签名的二进制布局：
//
//	public key: [2]byte algorithm("Ed") | [8]byte key id | [32]byte ed25519 pubkey
//	signature:  [2]byte algorithm("ED"/"Ed") | [8]byte key id | [64]byte signature
//
// "ED" 表示预哈希模式（对 BLAKE2b-512(message) 签名），"Ed" 表示对原文签名。
// minisign 现代版本默认使用 "ED"。
const (
	minisignPubKeyAlg = "Ed"
	minisignSigAlgPre = "ED"
	minisignSigAlgRaw = "Ed"
)

// parseMinisignPublicKey 接受原始 base64 公钥或包含 "untrusted comment:" 行的
// 公钥文件内容，返回 key id 与 ed25519 公钥。
func parseMinisignPublicKey(s string) (keyID []byte, pub ed25519.PublicKey, err error) {
	b64 := ""
	for _, line := range strings.Split(strings.TrimSpace(s), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "untrusted comment:") || strings.HasPrefix(line, "trusted comment:") {
			continue
		}
		b64 = line
		break
	}
	if b64 == "" {
		return nil, nil, errors.New("minisign public key is empty")
	}
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, nil, fmt.Errorf("decode minisign public key: %w", err)
	}
	if len(raw) != 42 {
		return nil, nil, fmt.Errorf("minisign public key length = %d, want 42", len(raw))
	}
	if string(raw[0:2]) != minisignPubKeyAlg {
		return nil, nil, fmt.Errorf("unsupported minisign public key algorithm %q", raw[0:2])
	}
	keyID = make([]byte, 8)
	copy(keyID, raw[2:10])
	pub = make(ed25519.PublicKey, ed25519.PublicKeySize)
	copy(pub, raw[10:42])
	return keyID, pub, nil
}

// VerifyMinisign 用 minisign 公钥校验 message 的分离签名（.minisig 文件内容）。
// 只校验主签名，足以认证被签名文件的内容。
func VerifyMinisign(message, sigFile []byte, publicKey string) error {
	keyID, pub, err := parseMinisignPublicKey(publicKey)
	if err != nil {
		return err
	}
	lines := strings.Split(strings.TrimSpace(string(sigFile)), "\n")
	var sigLine string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "untrusted comment:") || strings.HasPrefix(line, "trusted comment:") {
			continue
		}
		sigLine = line
		break
	}
	if sigLine == "" {
		return errors.New("minisign signature file has no signature")
	}
	blob, err := base64.StdEncoding.DecodeString(sigLine)
	if err != nil {
		return fmt.Errorf("decode minisign signature: %w", err)
	}
	if len(blob) != 74 {
		return fmt.Errorf("minisign signature length = %d, want 74", len(blob))
	}
	alg := string(blob[0:2])
	if alg != minisignSigAlgPre && alg != minisignSigAlgRaw {
		return fmt.Errorf("unsupported minisign signature algorithm %q", alg)
	}
	if !bytes.Equal(blob[2:10], keyID) {
		return errors.New("minisign signature key id does not match public key")
	}
	sig := blob[10:74]
	var signed []byte
	if alg == minisignSigAlgPre {
		h := blake2b.Sum512(message)
		signed = h[:]
	} else {
		signed = message
	}
	if !ed25519.Verify(pub, signed, sig) {
		return errors.New("minisign signature verification failed")
	}
	return nil
}
