// Package gpgsig 解析并校验 git 提交的 GPG 签名。
package gpgsig

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/armor"
	"github.com/ProtonMail/go-crypto/openpgp/packet"
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

// 签名校验结果状态。
const (
	StatusUnsigned   = "unsigned"    // 提交无签名
	StatusInvalid    = "invalid"     // 有签名但密码学校验失败（被篡改）
	StatusUnknownKey = "unknown_key" // 签名有效，但签名密钥未在平台注册
	StatusVerified   = "verified"    // 签名有效且密钥已注册
)

// VerifyCommit 校验提交是否由已注册用户公钥签名。
// 返回 (注册用户名或 "", 指纹, 状态)；状态见上方常量。
func VerifyCommit(raw []byte, keys []Key) (string, string, string) {
	msg, sigArmor, ok := Split(raw)
	if !ok {
		return "", "", StatusUnsigned
	}
	trusted := openpgp.EntityList{}
	for _, k := range keys {
		el, err := openpgp.ReadArmoredKeyRing(strings.NewReader(k.Armor))
		if err != nil {
			continue
		}
		trusted = append(trusted, el...)
	}
	// 签名 issuer keyid 是否对应已注册密钥：用于区分 "已注册密钥但签名无效"
	// 与 "未注册密钥"。
	registeredKeyIDs := map[uint64]string{}
	for _, k := range keys {
		el, err := openpgp.ReadArmoredKeyRing(strings.NewReader(k.Armor))
		if err != nil {
			continue
		}
		for _, e := range el {
			registeredKeyIDs[e.PrimaryKey.KeyId] = k.Username
		}
	}
	signer, err := openpgp.CheckArmoredDetachedSignature(trusted, bytes.NewReader(msg), strings.NewReader(sigArmor), nil)
	if err != nil {
		if issuerOf(sigArmor) != 0 {
			if _, registered := registeredKeyIDs[issuerOf(sigArmor)]; registered {
				return "", "", StatusInvalid
			}
		}
		return "", "", StatusUnknownKey
	}
	fp := fingerprintHex(signer.PrimaryKey.Fingerprint)
	for _, k := range keys {
		if strings.EqualFold(k.Fingerprint, fp) {
			return k.Username, fp, StatusVerified
		}
	}
	return "", fp, StatusUnknownKey
}

// issuerOf 从签名 armor 中提取 issuer keyid（解析失败返回 0）。
func issuerOf(sigArmor string) uint64 {
	block, err := armor.Decode(strings.NewReader(sigArmor))
	if err != nil {
		return 0
	}
	r := packet.NewReader(block.Body)
	for {
		p, err := r.Next()
		if err != nil {
			return 0
		}
		if sig, ok := p.(*packet.Signature); ok && sig.IssuerKeyId != nil {
			return *sig.IssuerKeyId
		}
	}
}
