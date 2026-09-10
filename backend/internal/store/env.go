package store

import (
	"errors"
	"regexp"

	"gorm.io/gorm/clause"
)

// MaxRepoEnvVars 单个仓库最多可配置的流水线环境变量数。
const MaxRepoEnvVars = 50

// MaxRepoEnvValueLen 单个环境变量值（密文/令牌等）的最大长度。
const MaxRepoEnvValueLen = 8192

// RepoEnvVar 仓库级环境变量（注入到每次流水线运行的容器 / host 执行环境）。
// 仅在仓库设置页展示，供 owner 查看 / 编辑 / 删除。
type RepoEnvVar struct {
	Key       string `json:"key"`
	Value     string `json:"value"`
	CreatedAt string `json:"created_at"`
}

var repoEnvKeyRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// ListRepoEnvVars 返回仓库已配置的环境变量（按 key 排序）。
func (s *Store) ListRepoEnvVars(owner, repo string) ([]RepoEnvVar, error) {
	var rows []repoEnvVarRow
	if err := s.db.Where("owner = ? AND repo = ?", owner, repo).Order("key").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]RepoEnvVar, 0, len(rows))
	for _, r := range rows {
		out = append(out, RepoEnvVar{Key: r.Key, Value: r.Value, CreatedAt: r.CreatedAt})
	}
	return out, nil
}

// SetRepoEnvVar 新增或覆盖一个环境变量（key 唯一，upsert）。
func (s *Store) SetRepoEnvVar(owner, repo, key, value string) error {
	if !repoEnvKeyRe.MatchString(key) || len(key) > 128 {
		return errors.New("invalid env key (must match [A-Za-z_][A-Za-z0-9_]*)")
	}
	if len(value) > MaxRepoEnvValueLen {
		return errors.New("env value too long")
	}
	var n int64
	if err := s.db.Model(&repoEnvVarRow{}).
		Where("owner = ? AND repo = ? AND key <> ?", owner, repo, key).
		Count(&n).Error; err != nil {
		return err
	}
	if n >= MaxRepoEnvVars {
		return errors.New("too many env vars")
	}
	row := repoEnvVarRow{Owner: owner, Repo: repo, Key: key, Value: value, CreatedAt: now()}
	return s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "owner"}, {Name: "repo"}, {Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value"}),
	}).Create(&row).Error
}

// DeleteRepoEnvVar 删除一个环境变量；不存在返回 ErrNotFound。
func (s *Store) DeleteRepoEnvVar(owner, repo, key string) error {
	res := s.db.Where("owner = ? AND repo = ? AND key = ?", owner, repo, key).Delete(&repoEnvVarRow{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// RepoEnvVars 以 KEY=VALUE 形式返回仓库环境变量（用于注入流水线执行环境）。
func (s *Store) RepoEnvVars(owner, repo string) ([]string, error) {
	vars, err := s.ListRepoEnvVars(owner, repo)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(vars))
	for _, v := range vars {
		out = append(out, v.Key+"="+v.Value)
	}
	return out, nil
}
