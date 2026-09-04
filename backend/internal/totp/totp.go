// Package totp 实现 RFC 6238 TOTP（HMAC-SHA1 / 6 位 / 30s），用于 MFA。
package totp

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strings"
	"time"
)

const digits = 6
const period = 30

// GenerateSecret 生成 20 字节随机 base32 secret（无 padding、大写）。
func GenerateSecret() (string, error) {
	buf := make([]byte, 20)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return strings.TrimRight(base32.StdEncoding.EncodeToString(buf), "="), nil
}

// URI 生成 otpauth:// URI（供二维码 / 手动录入）。
func URI(issuer, account, secret string) string {
	q := url.Values{}
	q.Set("secret", secret)
	q.Set("issuer", issuer)
	q.Set("algorithm", "SHA1")
	q.Set("digits", fmt.Sprint(digits))
	q.Set("period", fmt.Sprint(period))
	return fmt.Sprintf("otpauth://totp/%s?%s", url.PathEscape(issuer+":"+account), q.Encode())
}

// Code 计算某个时刻的 TOTP 验证码。
func Code(secret string, t time.Time) (string, error) {
	key, err := decodeSecret(secret)
	if err != nil {
		return "", err
	}
	counter := uint64(t.Unix() / period)
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], counter)
	mac := hmac.New(sha1.New, key)
	mac.Write(msg[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	binCode := (uint32(sum[offset])&0x7f)<<24 |
		uint32(sum[offset+1])<<16 |
		uint32(sum[offset+2])<<8 |
		uint32(sum[offset+3])
	mod := uint32(1)
	for i := 0; i < digits; i++ {
		mod *= 10
	}
	return fmt.Sprintf("%0*d", digits, binCode%mod), nil
}

// Verify 校验验证码（允许 ±window 个时间步的时钟偏移）。
func Verify(secret, code string, window int) bool {
	if code == "" {
		return false
	}
	now := time.Now()
	for i := -window; i <= window; i++ {
		c, err := Code(secret, now.Add(time.Duration(i)*period*time.Second))
		if err != nil {
			return false
		}
		if hmac.Equal([]byte(c), []byte(code)) {
			return true
		}
	}
	return false
}

func decodeSecret(secret string) ([]byte, error) {
	s := strings.ToUpper(strings.TrimSpace(secret))
	s = strings.ReplaceAll(s, " ", "")
	if pad := len(s) % 8; pad != 0 {
		s += strings.Repeat("=", 8-pad)
	}
	return base32.StdEncoding.DecodeString(s)
}
