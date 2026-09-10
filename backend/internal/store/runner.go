package store

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"gorm.io/gorm"
)

// Runner DTO：不暴露 secret；secret 明文仅在注册时返回一次。
type Runner struct {
	ID         int64     `json:"id"`
	Name       string    `json:"name"`
	Labels     []string  `json:"labels"`
	Scope      string    `json:"scope"` // ""=全局 | "user:{owner}" | "org:{org}"
	Status     string    `json:"status"`
	Mode       string    `json:"mode"` // ""=agent 主动外连（dial） | "reverse"=服务端主动拨号
	URL        string    `json:"url,omitempty"`
	LastSeen   *string   `json:"last_seen,omitempty"`
	CreatedAt  string    `json:"created_at"`
	LastSeenAt time.Time `json:"-"`
}

type RunnerToken struct {
	Scope     string `json:"scope"`
	ExpiresAt string `json:"expires_at"`
}

// ---- rows ----

type runnerRow struct {
	ID         int64  `gorm:"primaryKey;autoIncrement"`
	Name       string `gorm:"uniqueIndex;size:64"`
	SecretHash string `gorm:"not null;size:64"`
	Labels     string `gorm:"not null;default:''"`
	Scope      string `gorm:"not null;default:'';index"`
	Status     string `gorm:"not null;default:'offline'"`
	// Mode ""=agent 主动外连服务端（dial）；"reverse"=runner 监听、服务端主动拨号。
	Mode      string `gorm:"not null;default:''"`
	URL       string `gorm:"not null;default:''"`
	LastSeen  *string
	CreatedAt string `gorm:"not null"`
}

func (runnerRow) TableName() string { return "runners" }

type runnerTokenRow struct {
	ID        int64  `gorm:"primaryKey;autoIncrement"`
	TokenHash string `gorm:"uniqueIndex;size:64"`
	Scope     string `gorm:"not null;default:''"`
	ExpiresAt string `gorm:"not null;index"`
	CreatedAt string `gorm:"not null"`
}

func (runnerTokenRow) TableName() string { return "runner_registration_tokens" }

// ---- helpers ----

func runnerHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func splitLabels(s string) []string {
	out := []string{}
	cur := ""
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			if cur != "" {
				out = append(out, cur)
			}
			cur = ""
			continue
		}
		cur += string(s[i])
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

func runnerRowToDTO(r runnerRow) Runner {
	return Runner{
		ID: r.ID, Name: r.Name, Labels: splitLabels(r.Labels), Scope: r.Scope,
		Status: r.Status, Mode: r.Mode, URL: r.URL, LastSeen: r.LastSeen, CreatedAt: r.CreatedAt,
	}
}

// ---- registration tokens ----

// CreateRunnerToken 生成一次性注册 token（10 分钟有效），仅存 hash。
// scope 为空表示全局（仅管理员入口调用）。
func (s *Store) CreateRunnerToken(scope string) (string, RunnerToken, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", RunnerToken{}, err
	}
	token := hex.EncodeToString(raw)
	expires := time.Now().UTC().Add(10 * time.Minute).Format(time.RFC3339)
	row := runnerTokenRow{TokenHash: runnerHash(token), Scope: scope, ExpiresAt: expires, CreatedAt: now()}
	if err := s.db.Create(&row).Error; err != nil {
		return "", RunnerToken{}, err
	}
	return token, RunnerToken{Scope: scope, ExpiresAt: expires}, nil
}

// ConsumeRunnerToken 一次性校验注册 token，返回 scope；过期/无效返回 ErrNotFound。
func (s *Store) ConsumeRunnerToken(token string) (string, error) {
	var row runnerTokenRow
	if err := s.db.Where("token_hash = ?", runnerHash(token)).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", ErrNotFound
		}
		return "", err
	}
	if err := s.db.Delete(&row).Error; err != nil {
		return "", err
	}
	if exp, err := time.Parse(time.RFC3339, row.ExpiresAt); err != nil || exp.Before(time.Now().UTC()) {
		return "", ErrNotFound
	}
	return row.Scope, nil
}

// ---- runners ----

// CreateRunner 注册 runner；secret 明文由调用方生成，只存 hash。
// mode 为 ""（dial）或 "reverse"；reverse 时 url 为服务端拨号的 WS 地址。
func (s *Store) CreateRunner(name, secret, labels, scope, mode, url string) (Runner, error) {
	row := runnerRow{Name: name, SecretHash: runnerHash(secret), Labels: labels, Scope: scope, Mode: mode, URL: url, Status: "offline", CreatedAt: now()}
	if err := s.db.Create(&row).Error; err != nil {
		if isUniqueErr(err) {
			return Runner{}, ErrExists
		}
		return Runner{}, err
	}
	return runnerRowToDTO(row), nil
}

// GetRunnerSecretHash 返回 runner 存储的凭证 hash（sha256(secret) 的 hex）。
// 反向模式服务端拨号时用它作为共享凭证（runner 端用本地 secret 重新计算同值）。
func (s *Store) GetRunnerSecretHash(name string) (string, error) {
	var row runnerRow
	if err := s.db.Select("secret_hash").Where("name = ?", name).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", ErrNotFound
		}
		return "", err
	}
	return row.SecretHash, nil
}

func (s *Store) GetRunner(name string) (Runner, error) {
	var row runnerRow
	if err := s.db.Where("name = ?", name).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Runner{}, ErrNotFound
		}
		return Runner{}, err
	}
	return runnerRowToDTO(row), nil
}

// ValidateRunnerSecret 校验 runner 凭证（sha256 比对）。
func (s *Store) ValidateRunnerSecret(name, secret string) (Runner, error) {
	var row runnerRow
	if err := s.db.Where("name = ? AND secret_hash = ?", name, runnerHash(secret)).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Runner{}, ErrNotFound
		}
		return Runner{}, err
	}
	return runnerRowToDTO(row), nil
}

// ListRunnersByScopes 列出 scope 命中集合的 runner（"" 为全局）。
func (s *Store) ListRunnersByScopes(scopes []string) ([]Runner, error) {
	var rows []runnerRow
	q := s.db
	if len(scopes) == 0 {
		q = q.Where("scope = ?", "")
	} else {
		q = q.Where("scope = ? OR scope IN ?", "", scopes)
	}
	if err := q.Order("id DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]Runner, 0, len(rows))
	for _, r := range rows {
		out = append(out, runnerRowToDTO(r))
	}
	return out, nil
}

// ListAllRunners 管理员/调试用全量列表。
func (s *Store) ListAllRunners() ([]Runner, error) {
	var rows []runnerRow
	if err := s.db.Order("id DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]Runner, 0, len(rows))
	for _, r := range rows {
		out = append(out, runnerRowToDTO(r))
	}
	return out, nil
}

// ListReverseRunners 反向模式（mode="reverse"）且配置了 url 的 runner（Hub 拨号对账用）。
func (s *Store) ListReverseRunners() ([]Runner, error) {
	var rows []runnerRow
	if err := s.db.Where("mode = ? AND url <> ''", "reverse").Order("id DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]Runner, 0, len(rows))
	for _, r := range rows {
		out = append(out, runnerRowToDTO(r))
	}
	return out, nil
}

// DeleteRunner 删除 runner。
func (s *Store) DeleteRunner(name string) error {
	res := s.db.Where("name = ?", name).Delete(&runnerRow{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// SetRunnerStatus 更新在线状态并记录 last_seen。
func (s *Store) SetRunnerStatus(name, status string) error {
	ts := time.Now().UTC().Format(time.RFC3339)
	return s.db.Model(&runnerRow{}).Where("name = ?", name).
		Updates(map[string]any{"status": status, "last_seen": ts}).Error
}

// OnlineRunnerNames 当前 DB 标记为 online 的 runner 名。
func (s *Store) OnlineRunnerNames() ([]string, error) {
	var names []string
	err := s.db.Model(&runnerRow{}).Where("status = ?", "online").Pluck("name", &names).Error
	return names, err
}

// RunningRunIDsByRunner 某 runner 名下仍在 running 状态的 run（offline 回收用）。
func (s *Store) RunningRunIDsByRunner(runnerName string) ([]int64, error) {
	var ids []int64
	err := s.db.Model(&pipelineRunRow{}).
		Where("runner_name = ? AND status = ?", runnerName, "running").
		Pluck("id", &ids).Error
	return ids, err
}
