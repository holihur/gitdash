package store

import "gorm.io/gorm/clause"

// MergeQueueEntry 合并队列中的一条记录（按 ID 顺序串行合并）。
type MergeQueueEntry struct {
	ID         int64  `json:"id"`
	Number     int64  `json:"number"`
	Branch     string `json:"branch"`
	Method     string `json:"method,omitempty"`
	EnqueuedBy string `json:"enqueued_by"`
	EnqueuedAt string `json:"enqueued_at"`
}

func mergeRowToDTO(r mergeQueueRow) MergeQueueEntry {
	return MergeQueueEntry{
		ID: r.ID, Number: r.Number, Branch: r.Branch,
		Method: r.Method, EnqueuedBy: r.EnqueuedBy, EnqueuedAt: r.EnqueuedAt,
	}
}

// EnqueueMerge 把 PR 加入其目标分支的合并队列（重复入队保持原顺序、更新合并方式）。
func (s *Store) EnqueueMerge(owner, repo, branch string, number int64, method, by string) error {
	row := mergeQueueRow{
		Owner: owner, Repo: repo, Branch: branch, Number: number,
		Method: method, EnqueuedBy: by, EnqueuedAt: now(),
	}
	return s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "owner"}, {Name: "repo"}, {Name: "number"}},
		DoUpdates: clause.Assignments(map[string]any{"method": method, "branch": branch}),
	}).Create(&row).Error
}

// ListMergeQueue 按入队顺序列出某分支的合并队列。
func (s *Store) ListMergeQueue(owner, repo, branch string) ([]MergeQueueEntry, error) {
	var rows []mergeQueueRow
	if err := s.db.Where("owner = ? AND repo = ? AND branch = ?", owner, repo, branch).
		Order("id").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]MergeQueueEntry, 0, len(rows))
	for _, r := range rows {
		out = append(out, mergeRowToDTO(r))
	}
	return out, nil
}

// RemoveMergeEntry 从队列移除某 PR；不存在返回 ErrNotFound。
func (s *Store) RemoveMergeEntry(owner, repo string, number int64) error {
	res := s.db.Where("owner = ? AND repo = ? AND number = ?", owner, repo, number).Delete(&mergeQueueRow{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// GetMergeEntry 查询某 PR 是否在队列中。
func (s *Store) GetMergeEntry(owner, repo string, number int64) (MergeQueueEntry, bool) {
	var r mergeQueueRow
	if err := s.db.Where("owner = ? AND repo = ? AND number = ?", owner, repo, number).
		First(&r).Error; err != nil {
		return MergeQueueEntry{}, false
	}
	return mergeRowToDTO(r), true
}
