package store

import (
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Pipeline 仓库流水线开关配置（独立表，避免侵入 repos 基础查询）。
type Pipeline struct {
	Owner     string `json:"owner"`
	Repo      string `json:"repo"`
	Enabled   bool   `json:"enabled"`
	CreatedAt string `json:"created_at"`
}

// PipelineRun 一次流水线执行（push 或手动触发）。
type PipelineRun struct {
	ID         int64   `json:"id"`
	Owner      string  `json:"-"`
	Repo       string  `json:"-"`
	SHA        string  `json:"sha"`
	Ref        string  `json:"ref"`
	TriggerBy  string  `json:"trigger_by"`
	Status     string  `json:"status"` // pending | running | success | failed
	StepsTotal int     `json:"steps_total"`
	StepsDone  int     `json:"steps_done"`
	Error      string  `json:"error,omitempty"`
	CreatedAt  string  `json:"created_at"`
	FinishedAt *string `json:"finished_at"`
	// Log 由 API 层按需从磁盘读取填充
	Log string `json:"log,omitempty"`
}

// runRowToDTO row → DTO 转换。
func runRowToDTO(r pipelineRunRow) PipelineRun {
	return PipelineRun{
		ID: r.ID, Owner: r.Owner, Repo: r.Repo,
		SHA: r.SHA, Ref: r.Ref, TriggerBy: r.TriggerBy, Status: r.Status,
		StepsTotal: r.StepsTotal, StepsDone: r.StepsDone, Error: r.Error,
		CreatedAt: r.CreatedAt, FinishedAt: r.FinishedAt,
	}
}

// GetPipeline 返回仓库流水线配置（未配置时 Enabled=false）。
func (s *Store) GetPipeline(owner, repo string) (Pipeline, error) {
	var row pipelineCfgRow
	err := s.db.Where("owner = ? AND repo = ?", owner, repo).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Pipeline{Owner: owner, Repo: repo}, nil
	}
	if err != nil {
		return Pipeline{}, err
	}
	return Pipeline(row), nil
}

// IsPipelineEnabled 未配置即视为关闭。
func (s *Store) IsPipelineEnabled(owner, repo string) bool {
	p, err := s.GetPipeline(owner, repo)
	return err == nil && p.Enabled
}

// SetPipeline 设置流水线开关（upsert）。
func (s *Store) SetPipeline(owner, repo string, enabled bool) error {
	row := pipelineCfgRow{Owner: owner, Repo: repo, Enabled: enabled, CreatedAt: now()}
	return s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "owner"}, {Name: "repo"}},
		DoUpdates: clause.AssignmentColumns([]string{"enabled"}),
	}).Create(&row).Error
}

// CreatePipelineRun 新建一次运行记录（初始 pending）。
func (s *Store) CreatePipelineRun(owner, repo, sha, ref, triggerBy string, stepsTotal int) (PipelineRun, error) {
	r := PipelineRun{
		Owner: owner, Repo: repo, SHA: sha, Ref: ref, TriggerBy: triggerBy,
		Status: "pending", StepsTotal: stepsTotal, CreatedAt: now(),
	}
	row := pipelineRunRow{
		Owner: owner, Repo: repo, SHA: sha, Ref: ref, TriggerBy: triggerBy,
		Status: "pending", StepsTotal: stepsTotal, StepsDone: 0, CreatedAt: r.CreatedAt,
	}
	if err := s.db.Create(&row).Error; err != nil {
		return r, err
	}
	r.ID = row.ID
	return r, nil
}

// StartPipelineRun 标记为 running。
func (s *Store) StartPipelineRun(id int64) error {
	return s.db.Model(&pipelineRunRow{}).Where("id = ?", id).Update("status", "running").Error
}

// ProgressPipelineRun 更新已完成步骤数。
func (s *Store) ProgressPipelineRun(id int64, stepsDone int) error {
	return s.db.Model(&pipelineRunRow{}).Where("id = ?", id).Update("steps_done", stepsDone).Error
}

// FinishPipelineRun 终态：success / failed（带错误信息）。
func (s *Store) FinishPipelineRun(id int64, status, errMsg string) error {
	ts := now()
	return s.db.Model(&pipelineRunRow{}).Where("id = ?", id).
		Updates(map[string]any{"status": status, "error": errMsg, "finished_at": ts}).Error
}

// ListPipelineRuns 最近 limit 条运行记录（新→旧）。
func (s *Store) ListPipelineRuns(owner, repo string, limit int) ([]PipelineRun, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var rows []pipelineRunRow
	if err := s.db.Where("owner = ? AND repo = ?", owner, repo).
		Order("id DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	runs := make([]PipelineRun, 0, len(rows))
	for _, r := range rows {
		runs = append(runs, runRowToDTO(r))
	}
	return runs, nil
}

// GetPipelineRun 单条运行记录（不存在返回 ErrNotFound）。
func (s *Store) GetPipelineRun(owner, repo string, id int64) (PipelineRun, error) {
	var row pipelineRunRow
	err := s.db.Where("owner = ? AND repo = ? AND id = ?", owner, repo, id).First(&row).Error
	if err != nil {
		return PipelineRun{}, notFoundErr(err)
	}
	return runRowToDTO(row), nil
}

// RunningPipelineRunIDs 仍在进行中的运行（用于避免同仓库并发排队过多）。
func (s *Store) RunningPipelineRunIDs(owner, repo string) ([]int64, error) {
	var ids []int64
	err := s.db.Model(&pipelineRunRow{}).
		Where("owner = ? AND repo = ? AND status IN ('pending','running')", owner, repo).
		Pluck("id", &ids).Error
	return ids, err
}
