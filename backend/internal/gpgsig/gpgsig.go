// Package gpgsig 解析并校验 git 提交的 GPG 签名。
package gpgsig

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"github.com/ProtonMail/go-crypto/openpgp"
)

// Key 一条注册的用户公钥。
type Key struct {
	Username    string
	Fingerprint string // 大写十六进制（无空格）
	Armor       string
}

// ParseArmoredKey 解析 ASCII-armored 公钥，返回主密钥指纹（大写十六进制）。
func ParseArmoredKey(armor string) (string, error) {
	ring, err := openpgp.ReadArmoredKeyRing(strings.NewReader(armor))
	if err != nil {
		return "", fmt.Errorf("invalid armored key: %w", err)
	}
	if len(ring) == 0 || ring[0].PrimaryKey == nil {
		return "", errors.New("no primary key found")
	}
	return fingerprintHex(ring[0].PrimaryKey.Fingerprint), nil
}

func fingerprintHex(fp []byte) string {
	var b strings.Builder
	for _, x := range fp {
		fmt.Fprintf(&b, "%02X", x)
	}
	return b.String()
}

// Split 把原始 commit 对象拆成“待验内容”与 PGP 签名 armor。
// git 的 gpgsig 头：首行 "gpgsig <b64 第一行>"，续行以单个空格开头。
// 与 git 保持一致：签名内容去掉 gpgsig 相关行、每行去尾随空白、以 \n 结尾。
func Split(raw []byte) (message []byte, sigArmor string, ok bool) {
	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	var msg, sig []string
	inSig := false
	for _, ln := range lines {
		if inSig {
			if strings.HasPrefix(ln, " ") {
				sig = append(sig, strings.TrimPrefix(ln, " "))
				continue
			}
			inSig = false
		}
		if strings.HasPrefix(ln, "gpgsig ") {
			inSig = true
			sig = append(sig, strings.TrimPrefix(ln, "gpgsig "))
			continue
		}
		msg = append(msg, ln)
	}
	if len(sig) == 0 {
		return nil, "", false
	}
	var b strings.Builder
	for _, ln := range msg {
		b.WriteString(strings.TrimRight(ln, " \t"))
		b.WriteByte('\n')
	}
	return []byte(b.String()), strings.Join(sig, "\n") + "\n", true
}

// VerifyCommit 校验提交是否由已注册用户公钥签名。
// 返回 (注册用户名或 "", 指纹, 是否有效且 key 已注册)。
func VerifyCommit(raw []byte, keys []Key) (string, string, bool) {
	msg, sigArmor, ok := Split(raw)
	if !ok {
		return "", "", false
	}
	trusted := openpgp.EntityList{}
	for _, k := range keys {
		el, err := openpgp.ReadArmoredKeyRing(strings.NewReader(k.Armor))
		if err != nil {
			continue
		}
		trusted = append(trusted, el...)
	}
	if len(trusted) == 0 {
		return "", "", false
	}
	signer, err := openpgp.CheckArmoredDetachedSignature(trusted, bytes.NewReader(msg), strings.NewReader(sigArmor), nil)
	if err != nil {
		return "", "", false
	}
	fp := fingerprintHex(signer.PrimaryKey.Fingerprint)
	for _, k := range keys {
		if strings.EqualFold(k.Fingerprint, fp) {
			return k.Username, fp, true
		}
	}
	return "", fp, false
}
