// Package envx 提供带默认值的环境变量解析小工具：空值 / 非法值 / 负数一律回退默认值。
package envx

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// Millis 读取以毫秒为单位的环境变量并返回 time.Duration。
func Millis(key string, defMS int) time.Duration {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return time.Duration(defMS) * time.Millisecond
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return time.Duration(defMS) * time.Millisecond
	}
	return time.Duration(n) * time.Millisecond
}

// Bool 读取布尔型环境变量：1/true/yes/on（大小写不敏感）为 true，
// 0/false/no/off 为 false，空值或非法值返回 def。
func Bool(key string, def bool) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return def
	}
}

// Int64 读取 64 位整型环境变量；空值/非法/负数均回退 def。
func Int64(key string, def int64) int64 {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n < 0 {
		return def
	}
	return n
}
