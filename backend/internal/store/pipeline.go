package store

import (
	"database/sql"
	"errors"
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

// GetPipeline 返回仓库流水线配置（未配置时 Enabled=false）。
func (s *Store) GetPipeline(owner, repo string) (Pipeline, error) {
	var p Pipeline
	err := s.db.QueryRow(`SELECT owner, repo, enabled, created_at FROM repo_pipelines WHERE owner = ? AND repo = ?`, owner, repo).
		Scan(&p.Owner, &p.Repo, &p.Enabled, &p.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Pipeline{Owner: owner, Repo: repo}, nil
	}
	return p, err
}

// IsPipelineEnabled 未配置即视为关闭。
func (s *Store) IsPipelineEnabled(owner, repo string) bool {
	p, err := s.GetPipeline(owner, repo)
	return err == nil && p.Enabled
}

// SetPipeline 设置流水线开关（upsert）。
func (s *Store) SetPipeline(owner, repo string, enabled bool) error {
	ev := 0
	if enabled {
		ev = 1
	}
	_, err := s.db.Exec(`INSERT INTO repo_pipelines (owner, repo, enabled, created_at) VALUES (?, ?, ?, ?)
		ON CONFLICT(owner, repo) DO UPDATE SET enabled = excluded.enabled`, owner, repo, ev, now())
	return err
}

// CreatePipelineRun 新建一次运行记录（初始 pending）。
func (s *Store) CreatePipelineRun(owner, repo, sha, ref, triggerBy string, stepsTotal int) (PipelineRun, error) {
	r := PipelineRun{
		Owner: owner, Repo: repo, SHA: sha, Ref: ref, TriggerBy: triggerBy,
		Status: "pending", StepsTotal: stepsTotal, CreatedAt: now(),
	}
	res, err := s.db.Exec(`INSERT INTO pipeline_runs
		(owner, repo, sha, ref, trigger_by, status, steps_total, steps_done, created_at)
		VALUES (?, ?, ?, ?, ?, 'pending', ?, 0, ?)`,
		owner, repo, sha, ref, triggerBy, stepsTotal, r.CreatedAt)
	if err != nil {
		return r, err
	}
	r.ID, _ = res.LastInsertId()
	return r, nil
}

// StartPipelineRun 标记为 running。
func (s *Store) StartPipelineRun(id int64) error {
	_, err := s.db.Exec(`UPDATE pipeline_runs SET status = 'running' WHERE id = ?`, id)
	return err
}

// ProgressPipelineRun 更新已完成步骤数。
func (s *Store) ProgressPipelineRun(id int64, stepsDone int) error {
	_, err := s.db.Exec(`UPDATE pipeline_runs SET steps_done = ? WHERE id = ?`, stepsDone, id)
	return err
}

// FinishPipelineRun 终态：success / failed（带错误信息）。
func (s *Store) FinishPipelineRun(id int64, status, errMsg string) error {
	_, err := s.db.Exec(`UPDATE pipeline_runs SET status = ?, error = ?, finished_at = ? WHERE id = ?`,
		status, errMsg, now(), id)
	return err
}

// ListPipelineRuns 最近 limit 条运行记录（新→旧）。
func (s *Store) ListPipelineRuns(owner, repo string, limit int) ([]PipelineRun, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := s.db.Query(`SELECT id, sha, ref, trigger_by, status, steps_total, steps_done, error, created_at, finished_at
		FROM pipeline_runs WHERE owner = ? AND repo = ? ORDER BY id DESC LIMIT ?`, owner, repo, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	runs := []PipelineRun{}
	for rows.Next() {
		var r PipelineRun
		if err := rows.Scan(&r.ID, &r.SHA, &r.Ref, &r.TriggerBy, &r.Status, &r.StepsTotal, &r.StepsDone, &r.Error, &r.CreatedAt, &r.FinishedAt); err != nil {
			return nil, err
		}
		runs = append(runs, r)
	}
	return runs, rows.Err()
}

// GetPipelineRun 单条运行记录。
func (s *Store) GetPipelineRun(owner, repo string, id int64) (PipelineRun, error) {
	var r PipelineRun
	err := s.db.QueryRow(`SELECT id, sha, ref, trigger_by, status, steps_total, steps_done, error, created_at, finished_at
		FROM pipeline_runs WHERE owner = ? AND repo = ? AND id = ?`, owner, repo, id).
		Scan(&r.ID, &r.SHA, &r.Ref, &r.TriggerBy, &r.Status, &r.StepsTotal, &r.StepsDone, &r.Error, &r.CreatedAt, &r.FinishedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return r, ErrNotFound
	}
	return r, err
}

// RunningPipelineRunIDs 仍在进行中的运行（用于避免同仓库并发排队过多）。
func (s *Store) RunningPipelineRunIDs(owner, repo string) ([]int64, error) {
	rows, err := s.db.Query(`SELECT id FROM pipeline_runs WHERE owner = ? AND repo = ? AND status IN ('pending','running')`, owner, repo)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
