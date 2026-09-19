package gitsvc

import (
	"net"
	"net/netip"
	"net/url"
	"strings"

	"gitdash/backend/internal/ssrf"
)

// RemoteURLBlocked 判断 git 远端地址是否指向被 SSRF 防护拦截的主机
// （回环/私有/链路本地/云元数据）。无法解析的远端地址一律视为拦截（fail-closed）。
//
// 支持 http(s)://、ssh://、git:// 以及 scp-like（[user@]host:path）。
// 本地文件系统路径不算远端，直接放行。
//
// 该函数在 API 校验（注册导入/镜像时）与执行点（ImportRepo/PushMirror 真正拨号前）
// 都会被调用：执行点复查可缩小（但无法完全消除，git 子进程会自行重新解析）
// DNS 重绑定 TOCTOU 窗口。
func RemoteURLBlocked(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return true
	}
	if !strings.Contains(raw, "://") {
		if isLocalPath(raw) {
			return false // 本地路径：git 直接操作本地仓库，无 SSRF 面
		}
		host, ok := scpHost(raw)
		if !ok {
			return true
		}
		return ssrf.HostBlocked(host)
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return true
	}
	// git:// 无认证，API 只允许回环/私有（本地/内网 git daemon）。执行点沿用同一
	// 策略：只接受回环/私有，公网 git:// 一律拒绝。
	if u.Scheme == "git" {
		return !hostLoopbackOrPrivate(u.Hostname())
	}
	return ssrf.HostBlocked(u.Hostname())
}

// hostLoopbackOrPrivate 判断主机是否解析到回环/私有地址（不受
// GITDASH_SSRF_ALLOW_PRIVATE 影响，用于 git:// 的固定策略）。
func hostLoopbackOrPrivate(host string) bool {
	ips, err := net.LookupIP(host)
	if err != nil {
		return false
	}
	for _, ip := range ips {
		if addr, ok := netip.AddrFromSlice(ip); ok {
			addr = addr.Unmap()
			if addr.IsLoopback() || addr.IsPrivate() {
				return true
			}
		}
	}
	return false
}

// isLocalPath 判断无 scheme 的 git 目标是否是本地文件系统路径（而非 scp-like 远端）。
func isLocalPath(raw string) bool {
	switch {
	case strings.HasPrefix(raw, "/"), strings.HasPrefix(raw, "./"), strings.HasPrefix(raw, "../"),
		strings.HasPrefix(raw, "~"), strings.HasPrefix(raw, `.\`), strings.HasPrefix(raw, `..\`),
		strings.HasPrefix(raw, `\\`):
		return true
	case len(raw) >= 2 && raw[1] == ':' && isASCIIAlpha(raw[0]):
		return true // Windows 盘符，如 C:\repos
	}
	// 无冒号即普通相对本地路径；含冒号按 scp-like 处理。
	return !strings.Contains(raw, ":")
}

func isASCIIAlpha(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

// scpHost 从 scp-like 地址提取主机名（支持 [user@]host:path 与 [::1]:path）。
func scpHost(raw string) (string, bool) {
	s := raw
	if i := strings.LastIndexByte(s, '@'); i >= 0 {
		s = s[i+1:]
	}
	if strings.HasPrefix(s, "[") {
		end := strings.IndexByte(s, ']')
		if end <= 0 {
			return "", false
		}
		return s[1:end], true
	}
	i := strings.IndexByte(s, ':')
	if i <= 0 {
		return "", false
	}
	return s[:i], true
}
