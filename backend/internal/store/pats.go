package store

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/netip"
	"strings"
	"time"
)

// PAT DTO：不暴露 token hash；token 明文仅在创建时返回一次。
type PAT struct {
	ID         int64    `json:"id"`
	Name       string   `json:"name"`
	Scopes     []string `json:"scopes"`
	CIDRs      []string `json:"cidrs"`      // 来源 IP/CIDR 白名单；空 = 不限
	ExpiresAt  string   `json:"expires_at"` // RFC3339 UTC；空 = 永不过期
	CreatedAt  string   `json:"created_at"`
	LastUsedAt string   `json:"last_used_at"`
}

var (
	// ErrPATExpired token 已过期。
	ErrPATExpired = errors.New("pat expired")
	// ErrPATIPDenied 请求来源 IP 不在 token 的白名单内。
	ErrPATIPDenied = errors.New("pat source ip not allowed")
)

func patHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func validScope(s string) bool {
	switch s {
	case "repo", "inbox", "keys":
		return true
	}
	return false
}

func normalizeScopes(scopes []string) (string, bool) {
	if len(scopes) == 0 {
		scopes = []string{"repo"}
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(scopes))
	for _, s := range scopes {
		if !validScope(s) || seen[s] {
			return "", false
		}
		seen[s] = true
		out = append(out, s)
	}
	return joinScopes(out), true
}

func joinScopes(parts []string) string {
	s := ""
	for i, p := range parts {
		if i > 0 {
			s += ","
		}
		s += p
	}
	return s
}

func splitList(s string) []string {
	if s == "" {
		return []string{}
	}
	out := []string{}
	cur := ""
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			out = append(out, cur)
			cur = ""
			continue
		}
		cur += string(s[i])
	}
	out = append(out, cur)
	return out
}

func splitScopes(s string) []string { return splitList(s) }

// splitCIDRs 解析逗号分隔的 IP/CIDR 白名单。
func splitCIDRs(s string) []string { return splitList(s) }

// NormalizePATCIDRs 校验并规范化来源 IP/CIDR 白名单；
// 单 IP 转为主机掩码前缀，CIDR 统一掩码并去重；空列表返回空串（不限来源）。
func NormalizePATCIDRs(cidrs []string) (string, bool) {
	if len(cidrs) == 0 {
		return "", true
	}
	seen := map[string]bool{}
	out := []string{}
	for _, c := range cidrs {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		var p netip.Prefix
		if strings.Contains(c, "/") {
			pp, err := netip.ParsePrefix(c)
			if err != nil {
				return "", false
			}
			p = pp.Masked()
		} else {
			a, err := netip.ParseAddr(c)
			if err != nil {
				return "", false
			}
			p = netip.PrefixFrom(a, a.BitLen())
		}
		key := p.String()
		if !seen[key] {
			seen[key] = true
			out = append(out, key)
		}
	}
	return joinScopes(out), true
}

// CreatePAT 生成 64 位 hex 明文 token，仅存 sha256 hash；明文只此一次返回。
// cidrs 为已规范化的 IP/CIDR 白名单（逗号分隔），expiresAt 为 RFC3339 UTC（空 = 永不过期）。
func (s *Store) CreatePAT(userID int64, name, scopes, cidrs, expiresAt string) (string, PAT, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", PAT{}, err
	}
	token := hex.EncodeToString(raw)
	row := patRow{UserID: userID, Name: name, TokenHash: patHash(token), Scopes: scopes, CIDRs: cidrs, ExpiresAt: expiresAt, CreatedAt: now()}
	if err := s.db.Create(&row).Error; err != nil {
		return "", PAT{}, err
	}
	return token, PAT{ID: row.ID, Name: name, Scopes: splitScopes(scopes), CIDRs: splitCIDRs(cidrs), ExpiresAt: expiresAt, CreatedAt: row.CreatedAt}, nil
}

// NormalizePATScopes 校验并规范化 scope 列表；空列表默认 ["repo"]。
func NormalizePATScopes(scopes []string) (string, bool) {
	return normalizeScopes(scopes)
}

func (s *Store) ListPATs(userID int64) ([]PAT, error) {
	var rows []patRow
	err := s.db.Where("user_id = ?", userID).Order("id DESC").Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := []PAT{}
	for _, r := range rows {
		out = append(out, PAT{ID: r.ID, Name: r.Name, Scopes: splitScopes(r.Scopes), CIDRs: splitCIDRs(r.CIDRs), ExpiresAt: r.ExpiresAt, CreatedAt: r.CreatedAt, LastUsedAt: r.LastUsedAt})
	}
	return out, nil
}

func (s *Store) DeletePAT(userID, id int64) error {
	res := s.db.Where("id = ? AND user_id = ?", id, userID).Delete(&patRow{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// ValidatePAT 按明文 token 查询；命中且未过期、来源 IP 符合白名单时
// 返回 username 与 scopes（best-effort 更新 last_used_at）。
// ip 为空时跳过来源 IP 校验（供 admin 检测 PAT 的场景）。
func (s *Store) ValidatePAT(plainToken, ip string) (string, []string, error) {
	var row struct {
		ID        int64
		Username  string
		TokenHash string
		Scopes    string
		Cidrs     string `gorm:"column:cidrs"`
		ExpiresAt string
	}
	err := s.db.Table("pats").
		Select("pats.id, pats.token_hash, pats.scopes, pats.cidrs, pats.expires_at, users.username AS username").
		Joins("JOIN users ON users.id = pats.user_id").
		Where("pats.token_hash = ?", patHash(plainToken)).
		Scan(&row).Error
	if err != nil {
		return "", nil, err
	}
	if row.ID == 0 {
		return "", nil, ErrNotFound
	}
	if row.ExpiresAt != "" {
		exp, perr := time.Parse(time.RFC3339, row.ExpiresAt)
		if perr != nil || !time.Now().UTC().Before(exp) {
			return "", nil, ErrPATExpired
		}
	}
	if row.Cidrs != "" && ip != "" {
		if !cidrContains(row.Cidrs, ip) {
			return "", nil, ErrPATIPDenied
		}
	}
	_ = s.db.Model(&patRow{}).Where("id = ?", row.ID).Update("last_used_at", now()).Error
	return row.Username, splitScopes(row.Scopes), nil
}

// cidrContains 判断 ip 是否命中逗号分隔的 IP/CIDR 白名单。
func cidrContains(list, ip string) bool {
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return false
	}
	for _, c := range splitCIDRs(list) {
		if p, err := netip.ParsePrefix(c); err == nil && p.Contains(addr) {
			return true
		}
	}
	return false
}