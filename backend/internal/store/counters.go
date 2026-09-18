package store

import "fmt"

// 仓库级编号种类。
const (
	counterIssue = "issue"
	counterPR    = "pr"
)

// counterSeedTable 返回各类编号首次分配时用于取最大值的业务表名（白名单，防注入）。
func counterSeedTable(kind string) (string, error) {
	switch kind {
	case counterIssue:
		return "issues", nil
	case counterPR:
		return "pull_requests", nil
	default:
		return "", fmt.Errorf("unknown counter kind %q", kind)
	}
}

// nextNumber 原子地为 (owner, repo, kind) 分配下一个编号。
//
// 计数器独立于业务表持久化，因此删除 issue/PR 后编号不会回退、不复用：
//   - 首次分配（计数器行不存在）以业务表当前 MAX(number) 为基准 +1，兼容存量数据；
//   - 之后每次 ON CONFLICT 自增，保证同一仓库内单调递增。
//
// 借助 INSERT ... ON CONFLICT ... RETURNING，SQLite 与 PostgreSQL 均能原子完成，无并发竞态。
func (s *Store) nextNumber(owner, repo, kind string) (int64, error) {
	seed, err := counterSeedTable(kind)
	if err != nil {
		return 0, err
	}
	q := `INSERT INTO repo_counters (owner, repo, kind, value)
VALUES (?, ?, ?, (SELECT COALESCE(MAX(number), 0) FROM ` + seed + ` WHERE owner = ? AND repo = ?) + 1)
ON CONFLICT(owner, repo, kind) DO UPDATE SET value = repo_counters.value + 1
RETURNING value`
	var next int64
	if err := s.db.Raw(q, owner, repo, kind, owner, repo).Scan(&next).Error; err != nil {
		return 0, err
	}
	return next, nil
}
