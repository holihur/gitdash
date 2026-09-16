package pipeline

import (
	"time"

	"gitdash/backend/internal/logx"
	"gitdash/backend/internal/store"
)

// StartDelayedRunner 启动延迟执行调度器：周期扫描已到 run_at 的 pending 运行并派发。
// 立即触发（无 delay）不经过此调度器。
func StartDelayedRunner(st *store.Store, interval time.Duration) {
	if interval <= 0 {
		interval = 15 * time.Second
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for range t.C {
		RunDelayed(st, time.Now().UTC())
	}
}

// RunDelayed 执行一轮到期扫描（导出便于测试）。
func RunDelayed(st *store.Store, now time.Time) {
	runs, err := st.DuePipelineRuns(now.Format(time.RFC3339), 50)
	if err != nil {
		return
	}
	for _, r := range runs {
		job := RunJob{
			RunID: r.ID, Owner: r.Owner, Repo: r.Repo, File: r.File,
			SHA: r.SHA, Ref: r.Ref, Event: r.Event, Inputs: r.Inputs,
		}
		if err := dispatchJob(st, job); err != nil {
			logx.Infof("pipeline: dispatch delayed run %d: %v", r.ID, err)
		}
	}
}
