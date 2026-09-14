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
