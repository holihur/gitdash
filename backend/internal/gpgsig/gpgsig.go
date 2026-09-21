// Package gpgsig 解析并校验 git 提交的 GPG 签名。
package gpgsig

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"sync"

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

// parsedKeys 缓存 armor → 解析后的实体列表。GPG 公钥是只读的，而 commits 端点
// 会对同一批全站密钥在每条 commit 上重复校验；缓存解析结果可把
// O(提交数 × 密钥数) 的 armored key 解析降为一次性解析。
var parsedKeys sync.Map // string -> openpgp.EntityList

func parseKeyCached(k *Key) openpgp.EntityList {
	cacheKey := k.Fingerprint
	if cacheKey == "" {
		cacheKey = k.Armor
	}
	if v, ok := parsedKeys.Load(cacheKey); ok {
		return v.(openpgp.EntityList)
	}
	el, err := openpgp.ReadArmoredKeyRing(strings.NewReader(k.Armor))
	if err != nil {
		// 失败也缓存（nil），避免对坏密钥反复解析。
		el = nil
	}
	parsedKeys.Store(cacheKey, el)
	return el
}

// keyMatchesIssuer 判断实体（主钥或任一子钥）是否对应该签名 issuer keyid。
func keyMatchesIssuer(e *openpgp.Entity, issuer uint64) bool {
	if e.PrimaryKey != nil && e.PrimaryKey.KeyId == issuer {
		return true
	}
	for _, sk := range e.Subkeys {
		if sk.PublicKey != nil && sk.PublicKey.KeyId == issuer {
			return true
		}
	}
	return false
}

// VerifyCommit 校验提交是否由已注册用户公钥签名。
// 返回 (注册用户名或 "", 指纹, 状态)；状态见上方常量。
//
// 只把签名 issuer keyid 对应的那把公钥放进 trusted 列表：避免在每条 commit 上
// 对全站密钥逐个做签名尝试，把 CPU 放大从 O(提交数 × 密钥数) 收敛到 O(提交数)。
func VerifyCommit(raw []byte, keys []Key) (string, string, string) {
	msg, sigArmor, ok := Split(raw)
	if !ok {
		return "", "", StatusUnsigned
	}
	issuer := issuerOf(sigArmor)
	if issuer == 0 {
		// 无法从签名中解析 issuer：无法定位密钥，按未注册处理（与原行为一致）。
		return "", "", StatusUnknownKey
	}

	var trusted openpgp.EntityList
	registered := false
	for i := range keys {
		for _, e := range parseKeyCached(&keys[i]) {
			if e.PrimaryKey != nil && keyMatchesIssuer(e, issuer) {
				trusted = append(trusted, e)
				registered = true
			}
		}
	}
	signer, err := openpgp.CheckArmoredDetachedSignature(trusted, bytes.NewReader(msg), strings.NewReader(sigArmor), nil)
	if err != nil {
		if registered {
			return "", "", StatusInvalid
		}
		return "", "", StatusUnknownKey
	}
	fp := fingerprintHex(signer.PrimaryKey.Fingerprint)
	for i := range keys {
		if strings.EqualFold(keys[i].Fingerprint, fp) {
			return keys[i].Username, fp, StatusVerified
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
