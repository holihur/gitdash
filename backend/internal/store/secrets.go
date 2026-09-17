package store

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"os"
	"regexp"
	"strings"
	"sync"

	"gorm.io/gorm/clause"
)

// MaxRepoSecrets 单仓库最多可配置的 CI secrets 数。
const MaxRepoSecrets = 50

// MaxRepoSecretValueLen 单个 secret 值的最大长度。
const MaxRepoSecretValueLen = 8192

// RepoSecret 仓库 CI secret 元信息（永不包含明文值）。
type RepoSecret struct {
	Name      string `json:"name"`
	UpdatedAt string `json:"updated_at"`
}

var repoSecretNameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// ---- at-rest 加密 ----
// 优先使用 GITDASH_SECRET_KEY（64 位 hex 或 base64 的 32 字节）；否则使用
// SetSecretKeyFile 指定的密钥文件（首次自动生成，权限 0600）。
// 两者都不可用时退化为明文存储（与既有 BYOK 等字段一致，便于本地开发）。

var (
	secretKeyOnce sync.Once
	secretAEADVal cipher.AEAD
	secretKeyFile string
)

// SetSecretKeyFile 指定持久化密钥文件路径（main 在数据目录就绪后调用）。
func SetSecretKeyFile(path string) { secretKeyFile = path }

func secretAEAD() cipher.AEAD {
	secretKeyOnce.Do(func() {
		key := secretKeyFromEnv()
		if key == nil && secretKeyFile != "" {
			key = loadOrCreateKeyFile(secretKeyFile)
		}
		if key == nil {
			return
		}
		block, err := aes.NewCipher(key)
		if err != nil {
			return
		}
		gcm, err := cipher.NewGCM(block)
		if err != nil {
			return
		}
		secretAEADVal = gcm
	})
	return secretAEADVal
}

func secretKeyFromEnv() []byte {
	raw := strings.TrimSpace(os.Getenv("GITDASH_SECRET_KEY"))
	if raw == "" {
		return nil
	}
	if b, err := hex.DecodeString(raw); err == nil && len(b) == 32 {
		return b
	}
	if b, err := base64.StdEncoding.DecodeString(raw); err == nil && len(b) == 32 {
		return b
	}
	return nil
}

func loadOrCreateKeyFile(path string) []byte {
	if b, err := os.ReadFile(path); err == nil {
		if key, derr := hex.DecodeString(strings.TrimSpace(string(b))); derr == nil && len(key) == 32 {
			return key
		}
		return nil
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil
	}
	if err := os.WriteFile(path, []byte(hex.EncodeToString(key)), 0o600); err != nil {
		return nil
	}
	return key
}

// sealSecret 加密明文；无密钥时原样返回（明文落库）。
func sealSecret(plain string) string {
	a := secretAEAD()
	if a == nil {
		return plain
	}
	nonce := make([]byte, a.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return plain
	}
	ct := a.Seal(nil, nonce, []byte(plain), nil)
	return "v1:" + base64.StdEncoding.EncodeToString(append(nonce, ct...))
}

// openSecret 解密；非 "v1:" 前缀视为明文（兼容无密钥写入的历史数据）。
func openSecret(stored string) (string, error) {
	if !strings.HasPrefix(stored, "v1:") {
		return stored, nil
	}
	a := secretAEAD()
	if a == nil {
		return "", errors.New("secret is encrypted but GITDASH_SECRET_KEY / key file is not configured")
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(stored, "v1:"))
	if err != nil {
		return "", err
	}
	ns := a.NonceSize()
	if len(raw) < ns {
		return "", errors.New("corrupt secret")
	}
	pt, err := a.Open(nil, raw[:ns], raw[ns:], nil)
	if err != nil {
		return "", err
	}
	return string(pt), nil
}

// ListRepoSecrets 返回仓库已配置的 secret 元信息（按名称排序，不含明文）。
func (s *Store) ListRepoSecrets(owner, repo string) ([]RepoSecret, error) {
	var rows []repoSecretRow
	if err := s.db.Where("owner = ? AND repo = ?", owner, repo).Order("name").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]RepoSecret, 0, len(rows))
	for _, r := range rows {
		out = append(out, RepoSecret{Name: r.Name, UpdatedAt: r.UpdatedAt})
	}
	return out, nil
}

// SetRepoSecret 新增或覆盖一个 secret（名称唯一，upsert；值加密存储）。
func (s *Store) SetRepoSecret(owner, repo, name, value string) error {
	name = strings.TrimSpace(name)
	if !repoSecretNameRe.MatchString(name) || len(name) > 128 {
		return errors.New("invalid secret name (must match [A-Za-z_][A-Za-z0-9_]*)")
	}
	if len(value) > MaxRepoSecretValueLen {
		return errors.New("secret value too long")
	}
	var n int64
	if err := s.db.Model(&repoSecretRow{}).
		Where("owner = ? AND repo = ? AND name <> ?", owner, repo, name).
		Count(&n).Error; err != nil {
		return err
	}
	if n >= MaxRepoSecrets {
		return errors.New("too many secrets")
	}
	ts := now()
	row := repoSecretRow{Owner: owner, Repo: repo, Name: name, Value: sealSecret(value), CreatedAt: ts, UpdatedAt: ts}
	return s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "owner"}, {Name: "repo"}, {Name: "name"}},
		DoUpdates: clause.Assignments(map[string]any{"value": row.Value, "updated_at": ts}),
	}).Create(&row).Error
}

// DeleteRepoSecret 删除一个 secret；不存在返回 ErrNotFound。
func (s *Store) DeleteRepoSecret(owner, repo, name string) error {
	res := s.db.Where("owner = ? AND repo = ? AND name = ?", owner, repo, name).Delete(&repoSecretRow{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// RepoSecretValues 解析指定名称的 secret 明文（仅流水线执行时服务端内部使用）。
// 未命中的名称会被忽略（流水线使用不存在的 secret 不报错）。
func (s *Store) RepoSecretValues(owner, repo string, names []string) (map[string]string, error) {
	if len(names) == 0 {
		return map[string]string{}, nil
	}
	var rows []repoSecretRow
	if err := s.db.Where("owner = ? AND repo = ? AND name IN ?", owner, repo, names).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[string]string, len(rows))
	for _, r := range rows {
		v, err := openSecret(r.Value)
		if err != nil {
			return nil, err
		}
		out[r.Name] = v
	}
	return out, nil
}
