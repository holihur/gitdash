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
	File      string `json:"file,omitempty"` // 流水线定义文件路径（多文件支持）
	SHA       string `json:"sha"`
	Ref       string `json:"ref"`
	TriggerBy string `json:"trigger_by"`
	// Event 触发事件：push|pull_request|schedule|workflow_dispatch|manual（旧记录为空）
	Event string `json:"event,omitempty"`
	// RunAt 延迟执行时间（RFC3339）；空 = 立即执行。
	RunAt      string            `json:"run_at,omitempty"`
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
		ID: r.ID, Owner: r.Owner, Repo: r.Repo, File: r.File,
		SHA: r.SHA, Ref: r.Ref, TriggerBy: r.TriggerBy, Event: r.Event, RunAt: r.RunAt, Status: r.Status,
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
// file 为流水线定义文件路径（单文件兼容传 ""，落库为空）。
// event 为触发事件；inputs 为 dispatch 传入的键值对（其余事件为 nil）。
func (s *Store) CreatePipelineRun(owner, repo, file, sha, ref, triggerBy, event string, inputs map[string]string, stepsTotal int) (PipelineRun, error) {
	inputsJSON := ""
	if len(inputs) > 0 {
		if b, err := json.Marshal(inputs); err == nil {
			inputsJSON = string(b)
		}
	}
	ts := now()
	r := PipelineRun{
		Owner: owner, Repo: repo, File: file, SHA: sha, Ref: ref, TriggerBy: triggerBy, Event: event,
		Inputs: inputs, Status: "pending", StepsTotal: stepsTotal, CreatedAt: ts,
	}
	row := pipelineRunRow{
		Owner: owner, Repo: repo, File: file, SHA: sha, Ref: ref, TriggerBy: triggerBy, Event: event, Inputs: inputsJSON,
		Status: "pending", StepsTotal: stepsTotal, StepsDone: 0, CreatedAt: ts,
	}
	if err := s.db.Create(&row).Error; err != nil {
		return r, err
	}
	r.ID = row.ID
	return r, nil
}

// StartPipelineRun 标记为 running。
// StartPipelineRun 标记为 running（仅当仍处于 pending；已取消/终态的不再改动）。
func (s *Store) StartPipelineRun(id int64) error {
	return s.db.Model(&pipelineRunRow{}).Where("id = ? AND status = ?", id, "pending").Update("status", "running").Error
}

// ClaimPipelineRun 原子认领运行：pending → running。返回是否成功认领，
// 用于避免延迟调度器与队列工人重复执行同一运行。
func (s *Store) ClaimPipelineRun(id int64) (bool, error) {
	res := s.db.Model(&pipelineRunRow{}).Where("id = ? AND status = ?", id, "pending").Update("status", "running")
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

// CancelPipelineRun 将 pending/running 的运行标记为 cancelled（带原因）；
// 返回是否真正发生了状态变更（已完成/已取消的运行返回 false）。
func (s *Store) CancelPipelineRun(id int64, reason string) (bool, error) {
	res := s.db.Model(&pipelineRunRow{}).
		Where("id = ? AND status IN ('pending','running')", id).
		Updates(map[string]any{"status": "cancelled", "error": reason, "finished_at": now()})
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

// SetPipelineRunRunner 记录执行该 run 的远程 runner 名。
func (s *Store) SetPipelineRunRunner(id int64, runnerName string) error {
	return s.db.Model(&pipelineRunRow{}).Where("id = ?", id).Update("runner_name", runnerName).Error
}

// ProgressPipelineRun 更新已完成步骤数。
func (s *Store) ProgressPipelineRun(id int64, stepsDone int) error {
	return s.db.Model(&pipelineRunRow{}).Where("id = ?", id).Update("steps_done", stepsDone).Error
}

// SetPipelineRunRunAt 设置运行的延迟执行时间（RFC3339）；仅对 pending 生效。
func (s *Store) SetPipelineRunRunAt(id int64, runAt string) error {
	return s.db.Model(&pipelineRunRow{}).Where("id = ? AND status = 'pending'", id).Update("run_at", runAt).Error
}

// DuePipelineRuns 返回已到执行时间的延迟运行（status=pending 且 run_at 非空且 <= now）。
// 仅返回设置了 run_at 的运行，立即运行（run_at 为空）由 Trigger 直接派发。
func (s *Store) DuePipelineRuns(now string, limit int) ([]PipelineRun, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var rows []pipelineRunRow
	if err := s.db.Where("status = 'pending' AND run_at <> '' AND run_at <= ?", now).
		Order("run_at ASC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]PipelineRun, 0, len(rows))
	for _, r := range rows {
		out = append(out, runRowToDTO(r))
	}
	return out, nil
}

// FinishPipelineRun 终态：success / failed / cancelled（带错误信息）。
// 仅当仍处于 pending/running 时生效，避免已取消的运行被后续成功/失败覆盖。
func (s *Store) FinishPipelineRun(id int64, status, errMsg string) error {
	ts := now()
	return s.db.Model(&pipelineRunRow{}).Where("id = ? AND status IN ('pending','running')", id).
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

// RunningPipelineRunIDs 仍在进行中的运行（用于避免同仓库/同文件并发排队过多）。
// file 为空时统计仓库全部文件。
func (s *Store) RunningPipelineRunIDs(owner, repo, file string) ([]int64, error) {
	var ids []int64
	db := s.db.Model(&pipelineRunRow{}).Where("owner = ? AND repo = ? AND status IN ('pending','running')", owner, repo)
	if file != "" {
		db = db.Where("file = ?", file)
	}
	err := db.Pluck("id", &ids).Error
	return ids, err
}

// AggregatePipelineStatusForSHA 汇总某提交上所有流水线文件的运行状态（PR CI 门禁用）：
// 任一 pending/running → running；否则任一 failed → failed；全部 success → success。
// RunID 为最近一次运行（供前端跳转）；无运行时 ok=false。
func (s *Store) AggregatePipelineStatusForSHA(owner, repo, sha string) (PipelineCIStatus, bool, error) {
	var rows []pipelineRunRow
	err := s.db.Where("owner = ? AND repo = ? AND sha = ?", owner, repo, sha).
		Order("id DESC").Find(&rows).Error
	if err != nil {
		return PipelineCIStatus{}, false, err
	}
	if len(rows) == 0 {
		return PipelineCIStatus{}, false, nil
	}
	status := "success"
	for _, r := range rows {
		switch r.Status {
		case "pending", "running":
			status = "running"
		case "failed", "cancelled":
			if status != "running" {
				status = "failed"
			}
		}
	}
	return PipelineCIStatus{RunID: rows[0].ID, Status: status}, true, nil
}

// ---- 定时触发去重 ----

// GetScheduleLastFired 返回某 (repo, file, cron) 最近一次认领时间（RFC3339）；无记录时 ok=false。
func (s *Store) GetScheduleLastFired(owner, repo, file, expr string) (string, bool, error) {
	var row pipelineScheduleRow
	err := s.db.Where("owner = ? AND repo = ? AND file = ? AND expr = ?", owner, repo, file, expr).First(&row).Error
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
func (s *Store) ClaimSchedule(owner, repo, file, expr, at string) (bool, error) {
	res := s.db.Model(&pipelineScheduleRow{}).
		Where("owner = ? AND repo = ? AND file = ? AND expr = ? AND last_fired < ?", owner, repo, file, expr, at).
		Update("last_fired", at)
	if res.Error != nil {
		return false, res.Error
	}
	if res.RowsAffected > 0 {
		return true, nil
	}
	err := s.db.Create(&pipelineScheduleRow{Owner: owner, Repo: repo, File: file, Expr: expr, LastFired: at}).Error
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
// 尚未到点的延迟运行（run_at > now）不在回收范围。
func (s *Store) FailStalePipelineRuns(cutoff string) (int64, error) {
	res := s.db.Model(&pipelineRunRow{}).
		Where("status IN ('pending','running') AND created_at < ? AND (run_at = '' OR run_at <= ?)", cutoff, now()).
		Updates(map[string]any{
			"status":      "failed",
			"error":       "interrupted: server restarted",
			"finished_at": now(),
		})
	return res.RowsAffected, res.Error
}
