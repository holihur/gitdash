package store

import (
	"errors"

	"gorm.io/gorm"
)

// ---- 分支保护 ----

// BranchProtection 分支保护规则。
type BranchProtection struct {
	Owner          string `json:"owner"`
	Repo           string `json:"repo"`
	Branch         string `json:"branch"`
	MinApprovals   int    `json:"min_approvals"`    // 合并门禁：需要的最少 approve 数（0 = 不设门禁）
	BlockDeletion  bool   `json:"block_deletion"`   // 禁止删除该分支
	BlockForcePush bool   `json:"block_force_push"` // 禁止非快进（force push）
	CreatedAt      string `json:"created_at"`
}

// SetBranchProtection 创建或更新分支保护。
func (s *Store) SetBranchProtection(bp *BranchProtection) error {
	if bp.Owner == "" || bp.Repo == "" || bp.Branch == "" {
		return errors.New("owner, repo and branch are required")
	}
	if bp.MinApprovals < 0 {
		bp.MinApprovals = 0
	}
	if bp.MinApprovals > 100 {
		bp.MinApprovals = 100
	}
	row := branchProtectionRow{
		Owner: bp.Owner, Repo: bp.Repo, Branch: bp.Branch,
		MinApprovals: bp.MinApprovals, BlockDeletion: bp.BlockDeletion,
		BlockForcePush: bp.BlockForcePush, CreatedAt: now(),
	}
	// upsert：SQLite/PG 通用写法（先删后插在事务里）会有唯一键间隙，用 gorm clause OnConflict
	res := s.db.Where("owner = ? AND repo = ? AND branch = ?", bp.Owner, bp.Repo, bp.Branch).
		Assign(map[string]any{
			"min_approvals":    row.MinApprovals,
			"block_deletion":   row.BlockDeletion,
			"block_force_push": row.BlockForcePush,
		}).FirstOrCreate(&row)
	return res.Error
}

// GetBranchProtection 读取某分支保护；未设置返回 ErrNotFound。
func (s *Store) GetBranchProtection(owner, repo, branch string) (BranchProtection, error) {
	var row branchProtectionRow
	err := s.db.Where("owner = ? AND repo = ? AND branch = ?", owner, repo, branch).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return BranchProtection{}, ErrNotFound
		}
		return BranchProtection{}, err
	}
	return BranchProtection(row), nil
}

// ListBranchProtections 列出仓库全部分支保护。
func (s *Store) ListBranchProtections(owner, repo string) ([]BranchProtection, error) {
	var rows []branchProtectionRow
	if err := s.db.Where("owner = ? AND repo = ?", owner, repo).Order("branch").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]BranchProtection, 0, len(rows))
	for _, r := range rows {
		out = append(out, BranchProtection(r))
	}
	return out, nil
}

// DeleteBranchProtection 移除某分支保护；不存在返回 ErrNotFound。
func (s *Store) DeleteBranchProtection(owner, repo, branch string) error {
	res := s.db.Where("owner = ? AND repo = ? AND branch = ?", owner, repo, branch).Delete(&branchProtectionRow{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}
