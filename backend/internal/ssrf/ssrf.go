// Package ssrf 提供 SSRF 防护的 IP 分类逻辑：默认禁止回环/私有/链路本地/
// 云元数据等内网地址，仅允许公网目标；自托管部署可通过
// GITDASH_SSRF_ALLOW_PRIVATE=1 显式放开私有网段。
package ssrf

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"os"
	"time"
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

// HostBlocked 解析主机名并判断是否命中危险地址（注册时快速校验用）。
// 解析失败视为拦截（fail-closed）。
func HostBlocked(host string) bool {
	if host == "" {
		return true
	}
	ips, err := net.DefaultResolver.LookupNetIP(context.Background(), "ip", host)
	if err != nil || len(ips) == 0 {
		return true
	}
	for _, ip := range ips {
		if IsDangerous(ip) {
			return true
		}
	}
	return false
}

// DialContext 解析目标并拒绝危险地址，然后直连解析出的 IP。
// 直接拨 IP（而非交回系统重新解析）以消除 DNS 重绑定的 TOCTOU 窗口。
func DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return nil, err
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("no address for %s", host)
	}
	safe := make([]netip.Addr, 0, len(ips))
	for _, ip := range ips {
		if IsDangerous(ip) {
			return nil, fmt.Errorf("blocked address %s for %s", ip, host)
		}
		safe = append(safe, ip)
	}
	d := &net.Dialer{Timeout: 10 * time.Second}
	var lastErr error
	for _, ip := range safe {
		conn, derr := d.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if derr == nil {
			return conn, nil
		}
		lastErr = derr
	}
	return nil, lastErr
}
