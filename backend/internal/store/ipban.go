package store

import (
	"errors"
	"net/netip"
	"strings"
	"time"
)

// ErrInvalidCIDR 表示 IP / CIDR 格式非法。
var ErrInvalidCIDR = errors.New("invalid ip or cidr")

// ---- IP / CIDR 黑名单（admin 专用）----
//
// 命中黑名单的来源 IP 会被 HTTP 中间件与 SSH 服务在建立连接前直接拒绝。
// 条目为单个 IP 或 CIDR，统一规范化为掩码后的前缀字符串。

// ipBanCacheTTL 控制多实例部署下黑名单生效的最大延迟；
// 同进程的增删会立即失效缓存，无需等待 TTL。
const ipBanCacheTTL = 15 * time.Second

// IPBan 黑名单条目的公开 DTO。
type IPBan struct {
	ID        int64  `json:"id"`
	CIDR      string `json:"cidr"`
	Note      string `json:"note"`
	CreatedBy string `json:"created_by"`
	CreatedAt string `json:"created_at"`
}

// ipBanCache 是解析后的前缀快照。
type ipBanCache struct {
	prefixes []netip.Prefix
	loadedAt time.Time
}

// NormalizeIPCIDR 校验并规范化单个 IP / CIDR：
// 单 IP 转为主机掩码前缀，CIDR 统一套用网络掩码（如 10.0.0.1/8 → 10.0.0.0/8）。
func NormalizeIPCIDR(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	if strings.Contains(raw, "/") {
		p, err := netip.ParsePrefix(raw)
		if err != nil {
			return "", false
		}
		return p.Masked().String(), true
	}
	a, err := netip.ParseAddr(raw)
	if err != nil {
		return "", false
	}
	return netip.PrefixFrom(a.Unmap(), a.Unmap().BitLen()).String(), true
}

// ListIPBans 返回全部黑名单条目（按创建顺序）。
func (s *Store) ListIPBans() ([]IPBan, error) {
	var rows []ipBanRow
	if err := s.db.Order("id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]IPBan, 0, len(rows))
	for _, r := range rows {
		out = append(out, IPBan(r))
	}
	return out, nil
}

// AddIPBan 新增一条黑名单；cidr 会被规范化，重复条目返回 ErrExists。
func (s *Store) AddIPBan(cidr, note, createdBy string) (IPBan, error) {
	normalized, ok := NormalizeIPCIDR(cidr)
	if !ok {
		return IPBan{}, ErrInvalidCIDR
	}
	row := ipBanRow{
		CIDR:      normalized,
		Note:      strings.TrimSpace(note),
		CreatedBy: strings.TrimSpace(createdBy),
		CreatedAt: now(),
	}
	if err := s.db.Create(&row).Error; err != nil {
		if isUniqueErr(err) {
			return IPBan{}, ErrExists
		}
		return IPBan{}, err
	}
	s.invalidateIPBanCache()
	return IPBan(row), nil
}

// DeleteIPBan 按 ID 删除黑名单条目，不存在返回 ErrNotFound。
func (s *Store) DeleteIPBan(id int64) error {
	res := s.db.Where("id = ?", id).Delete(&ipBanRow{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	s.invalidateIPBanCache()
	return nil
}

// IsIPBanned 判断来源 IP 是否命中黑名单。ip 可为模糊地址（含 IPv6 zone）。
func (s *Store) IsIPBanned(ip string) bool {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return false
	}
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return false
	}
	addr = addr.Unmap()

	s.ipBanMu.RLock()
	c := s.ipBanCache
	fresh := c != nil && time.Since(c.loadedAt) <= ipBanCacheTTL
	s.ipBanMu.RUnlock()

	if !fresh {
		s.ipBanMu.Lock()
		if s.ipBanCache == nil || time.Since(s.ipBanCache.loadedAt) > ipBanCacheTTL {
			s.ipBanCache = s.loadIPBanCache()
		}
		c = s.ipBanCache
		s.ipBanMu.Unlock()
	}
	for _, p := range c.prefixes {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

// loadIPBanCache 从数据库重建前缀快照；查询失败返回空快照（fail-open，避免误锁全站）。
func (s *Store) loadIPBanCache() *ipBanCache {
	c := &ipBanCache{loadedAt: time.Now()}
	var rows []ipBanRow
	if err := s.db.Select("cidr").Find(&rows).Error; err != nil {
		return c
	}
	for _, r := range rows {
		if p, err := netip.ParsePrefix(r.CIDR); err == nil {
			c.prefixes = append(c.prefixes, p)
		}
	}
	return c
}

// invalidateIPBanCache 使缓存立即失效（下次查询重载）。
func (s *Store) invalidateIPBanCache() {
	s.ipBanMu.Lock()
	s.ipBanCache = nil
	s.ipBanMu.Unlock()
}
