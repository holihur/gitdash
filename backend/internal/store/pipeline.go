package store

import (
	"encoding/json"
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
	ID        int64  `json:"id"`
	Owner     string `json:"-"`
	Repo      string `json:"-"`
	SHA       string `json:"sha"`
	Ref       string `json:"ref"`
	TriggerBy string `json:"trigger_by"`
	// Event 触发事件：push|pull_request|schedule|workflow_dispatch|manual（旧记录为空）
	Event      string            `json:"event,omitempty"`
	Inputs     map[string]string `json:"inputs,omitempty"`
	Status     string            `json:"status"` // pending | running | success | failed
	StepsTotal int               `json:"steps_total"`
	StepsDone  int               `json:"steps_done"`
	Error      string            `json:"error,omitempty"`
	RunnerName string            `json:"runner_name,omitempty"`
	CreatedAt  string            `json:"created_at"`
	FinishedAt *string           `json:"finished_at"`
	// Log 由 API 层按需从磁盘读取填充
	Log string `json:"log,omitempty"`
}

// runRowToDTO row → DTO 转换。
func runRowToDTO(r pipelineRunRow) PipelineRun {
	out := PipelineRun{
		ID: r.ID, Owner: r.Owner, Repo: r.Repo,
		SHA: r.SHA, Ref: r.Ref, TriggerBy: r.TriggerBy, Event: r.Event, Status: r.Status,
		StepsTotal: r.StepsTotal, StepsDone: r.StepsDone, Error: r.Error,
		RunnerName: r.RunnerName,
		CreatedAt:  r.CreatedAt, FinishedAt: r.FinishedAt,
	}
	if r.Inputs != "" {
		_ = json.Unmarshal([]byte(r.Inputs), &out.Inputs)
	}
	return out
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
// event 为触发事件；inputs 为 dispatch 传入的键值对（其余事件为 nil）。
func (s *Store) CreatePipelineRun(owner, repo, sha, ref, triggerBy, event string, inputs map[string]string, stepsTotal int) (PipelineRun, error) {
	inputsJSON := ""
	if len(inputs) > 0 {
		if b, err := json.Marshal(inputs); err == nil {
			inputsJSON = string(b)
		}
	}
	ts := now()
	r := PipelineRun{
		Owner: owner, Repo: repo, SHA: sha, Ref: ref, TriggerBy: triggerBy, Event: event,
		Inputs: inputs, Status: "pending", StepsTotal: stepsTotal, CreatedAt: ts,
	}
	row := pipelineRunRow{
		Owner: owner, Repo: repo, SHA: sha, Ref: ref, TriggerBy: triggerBy, Event: event, Inputs: inputsJSON,
		Status: "pending", StepsTotal: stepsTotal, StepsDone: 0, CreatedAt: ts,
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

// SetPipelineRunRunner 记录执行该 run 的远程 runner 名。
func (s *Store) SetPipelineRunRunner(id int64, runnerName string) error {
	return s.db.Model(&pipelineRunRow{}).Where("id = ?", id).Update("runner_name", runnerName).Error
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

// LatestPipelineRunForSHA 某提交最近一次流水线运行（PR 视图展示 CI 状态用）；无则返回 false。
func (s *Store) LatestPipelineRunForSHA(owner, repo, sha string) (PipelineRun, bool, error) {
	var row pipelineRunRow
	err := s.db.Where("owner = ? AND repo = ? AND sha = ?", owner, repo, sha).
		Order("id DESC").First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return PipelineRun{}, false, nil
	}
	if err != nil {
		return PipelineRun{}, false, err
	}
	return runRowToDTO(row), true, nil
}

// ---- 定时触发去重 ----

// GetScheduleLastFired 返回某 (repo, cron) 最近一次认领时间（RFC3339）；无记录时 ok=false。
func (s *Store) GetScheduleLastFired(owner, repo, expr string) (string, bool, error) {
	var row pipelineScheduleRow
	err := s.db.Where("owner = ? AND repo = ? AND expr = ?", owner, repo, expr).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return row.LastFired, true, nil
}

// ClaimSchedule 原子认领一次定时触发：仅当无记录或 last_fired < at 时成功。
// 多实例并发时依靠条件更新 + 主键唯一约束保证只有一个实例成功。
func (s *Store) ClaimSchedule(owner, repo, expr, at string) (bool, error) {
	res := s.db.Model(&pipelineScheduleRow{}).
		Where("owner = ? AND repo = ? AND expr = ? AND last_fired < ?", owner, repo, expr, at).
		Update("last_fired", at)
	if res.Error != nil {
		return false, res.Error
	}
	if res.RowsAffected > 0 {
		return true, nil
	}
	err := s.db.Create(&pipelineScheduleRow{Owner: owner, Repo: repo, Expr: expr, LastFired: at}).Error
	if err == nil {
		return true, nil
	}
	if isUniqueErr(err) {
		return false, nil
	}
	return false, err
}

// ListEnabledPipelines 已开启流水线的仓库列表（定时触发扫描用）。
func (s *Store) ListEnabledPipelines() ([]Pipeline, error) {
	var rows []pipelineCfgRow
	if err := s.db.Where("enabled = ?", true).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]Pipeline, 0, len(rows))
	for _, r := range rows {
		out = append(out, Pipeline(r))
	}
	return out, nil
}

// FailStalePipelineRuns 孤儿 run 回收：把早于 cutoff 仍停在 pending/running 的运行
// 标记为 failed（进程重启后 memory 队列不会再执行它们）。返回受影响行数。
func (s *Store) FailStalePipelineRuns(cutoff string) (int64, error) {
	res := s.db.Model(&pipelineRunRow{}).
		Where("status IN ('pending','running') AND created_at < ?", cutoff).
		Updates(map[string]any{
			"status":      "failed",
			"error":       "interrupted: server restarted",
			"finished_at": now(),
		})
	return res.RowsAffected, res.Error
}
