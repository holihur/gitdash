package store

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
)

// PAT DTO：不暴露 token hash；token 明文仅在创建时返回一次。
type PAT struct {
	ID         int64    `json:"id"`
	Name       string   `json:"name"`
	Scopes     []string `json:"scopes"`
	CreatedAt  string   `json:"created_at"`
	LastUsedAt string   `json:"last_used_at"`
}

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

func splitScopes(s string) []string {
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

// CreatePAT 生成 64 位 hex 明文 token，仅存 sha256 hash；明文只此一次返回。
func (s *Store) CreatePAT(userID int64, name, scopes string) (string, PAT, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", PAT{}, err
	}
	token := hex.EncodeToString(raw)
	row := patRow{UserID: userID, Name: name, TokenHash: patHash(token), Scopes: scopes, CreatedAt: now()}
	if err := s.db.Create(&row).Error; err != nil {
		return "", PAT{}, err
	}
	return token, PAT{ID: row.ID, Name: name, Scopes: splitScopes(scopes), CreatedAt: row.CreatedAt}, nil
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
		out = append(out, PAT{ID: r.ID, Name: r.Name, Scopes: splitScopes(r.Scopes), CreatedAt: r.CreatedAt, LastUsedAt: r.LastUsedAt})
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

// ValidatePAT 按明文 token 查询；命中则返回 username 与 scopes（best-effort 更新 last_used_at）。
func (s *Store) ValidatePAT(plainToken string) (string, []string, error) {
	var row struct {
		ID        int64
		Username  string
		TokenHash string
		Scopes    string
	}
	err := s.db.Table("pats").
		Select("pats.id, pats.token_hash, pats.scopes, users.username AS username").
		Joins("JOIN users ON users.id = pats.user_id").
		Where("pats.token_hash = ?", patHash(plainToken)).
		Scan(&row).Error
	if err != nil {
		return "", nil, err
	}
	if row.ID == 0 {
		return "", nil, ErrNotFound
	}
	_ = s.db.Model(&patRow{}).Where("id = ?", row.ID).Update("last_used_at", now()).Error
	return row.Username, splitScopes(row.Scopes), nil
}
