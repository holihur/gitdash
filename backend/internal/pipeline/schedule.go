package pipeline

import (
	"log"
	"strings"
	"time"

	"gitdash/backend/internal/gitsvc"
	"gitdash/backend/internal/store"
)

// StartScheduler 启动定时触发扫描（每 interval 一次；main 中 go 启动）。
// 扫描所有已开启流水线的仓库，读取默认分支的 .gitdash.yml 并按 cron 触发；
// 多实例部署时经 pipeline_schedules 条件更新做原子认领，避免重复触发。
func StartScheduler(st *store.Store, interval time.Duration) {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for range t.C {
		RunScheduled(st, time.Now().UTC())
	}
}

// RunScheduled 执行一轮扫描（导出便于测试）。
func RunScheduled(st *store.Store, now time.Time) {
	pipes, err := st.ListEnabledPipelines()
	if err != nil {
		return
	}
	for _, p := range pipes {
		runRepoSchedule(st, p.Owner, p.Repo, now)
	}
}

// runRepoSchedule 检查单仓库的 cron 是否到期并触发（基于默认分支的 DSL）。
func runRepoSchedule(st *store.Store, owner, repo string, now time.Time) {
	branch, err := gitsvc.HeadBranch(owner, repo)
	if err != nil || branch == "" {
		return
	}
	sha, err := gitsvc.RevSHA(owner, repo, "refs/heads/"+branch)
	if err != nil || sha == "" {
		return
	}
	blob, err := gitsvc.ReadBlob(owner, repo, sha, FileName)
	if err != nil || blob.Encoding != "utf-8" || strings.TrimSpace(blob.Content) == "" {
		return
	}
	cfg, perr := Parse([]byte(blob.Content))
	if perr != nil || !cfg.ScheduleEnabled() {
		return
	}
	nowStr := now.Format(time.RFC3339)
	for _, expr := range cfg.Schedule {
		sched, cerr := cronParser.Parse(expr)
		if cerr != nil {
			continue
		}
		lastFired, ok, gerr := st.GetScheduleLastFired(owner, repo, expr)
		if gerr != nil {
			continue
		}
		if !ok {
			// 首次观察：仅登记基准时间，不立即补触发（避免服务重启即触发）
			_, _ = st.ClaimSchedule(owner, repo, expr, nowStr)
			continue
		}
		base, perr := time.Parse(time.RFC3339, lastFired)
		if perr != nil {
			base = now
		}
		if sched.Next(base).After(now) {
			continue
		}
		claimed, cerr := st.ClaimSchedule(owner, repo, expr, nowStr)
		if cerr != nil || !claimed {
			continue
		}
		if _, terr := Trigger(st, TriggerOpts{
			Owner: owner, Repo: repo, SHA: sha, Ref: branch,
			By: "schedule", Event: "schedule",
		}); terr != nil && !ignorableTriggerErr(terr) {
			log.Printf("pipeline: schedule trigger %s/%s (%s): %v", owner, repo, expr, terr)
		}
	}
}
