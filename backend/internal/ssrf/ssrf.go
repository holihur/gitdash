// Package ssrf 提供 SSRF 防护的 IP 分类逻辑：默认禁止回环/私有/链路本地/
// 云元数据等内网地址，仅允许公网目标；自托管部署可通过
// GITDASH_SSRF_ALLOW_PRIVATE=1 显式放开私有网段。
package ssrf

import (
	"net/netip"
	"os"
)

// allowPrivate 每次读取（测试用 t.Setenv 动态设置；环境变量读取开销可忽略）。
func allowPrivate() bool {
	return os.Getenv("GITDASH_SSRF_ALLOW_PRIVATE") != ""
}

// AllowPrivateReports 是否放开了私有网段（供日志/提示用）。
func AllowPrivate() bool { return allowPrivate() }

// IsDangerous 判断目标 IP 是否应被 SSRF 防护拦截。
func IsDangerous(addr netip.Addr) bool {
	addr = addr.Unmap()
	if !addr.IsValid() {
		return true
	}
	if addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() || addr.IsMulticast() {
		return true
	}
	if s := addr.String(); s == "100.100.100.200" { // 阿里云元数据
		return true
	}
	if allowPrivate() {
		return false
	}
	return addr.IsLoopback() || addr.IsPrivate() || addr.IsUnspecified()
}
