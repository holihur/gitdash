package pipeline

import (
	"log"

	"gitdash/backend/internal/gitsvc"
	"gitdash/backend/internal/store"
	"gitdash/backend/internal/webhooks"
)

// PullHandler 消费 API 侧的 pulls 事件：PR opened/reopened 时按 pull_request 事件触发流水线。
// 挂到 API spool 消费者上（main 中与邮件通知并列）。
func PullHandler(st *store.Store) func(webhooks.Event) {
	return func(ev webhooks.Event) {
		if ev.Event != "pulls" || ev.Kind != "pull" {
			return
		}
		if ev.Action != "opened" && ev.Action != "reopened" {
			return
		}
		if !gitsvc.ValidName(ev.Owner) || !gitsvc.ValidName(ev.Repo) || !st.IsPipelineEnabled(ev.Owner, ev.Repo) {
			return
		}
		pr, err := st.GetPull(ev.Owner, ev.Repo, ev.Number)
		if err != nil {
			return
		}
		triggerPull(st, pr, ev.Actor)
	}
}

// TriggerOpenPRsForBranch 分支 push 后，为所有以该分支为源分支的 open PR 触发 pull_request 运行
// （对应 GitHub 的 synchronize 事件）。仅同仓库 PR 有效（fork PR 的 head 不在本仓库）。
func TriggerOpenPRsForBranch(st *store.Store, owner, repo, branch, _ string, actor string) {
	if !gitsvc.ValidName(owner) || !gitsvc.ValidName(repo) || !st.IsPipelineEnabled(owner, repo) {
		return
	}
	prs, err := st.ListOpenPullsBySource(owner, repo, branch)
	if err != nil || len(prs) == 0 {
		return
	}
	for _, pr := range prs {
		triggerPull(st, pr, actor)
	}
}

// triggerPull 以 PR 当前 head 提交触发 pull_request 事件。
func triggerPull(st *store.Store, pr store.PullRequest, by string) {
	head := pr.HeadSHA
	if h, err := gitsvc.RevSHA(pr.Owner, pr.Repo, "refs/heads/"+pr.SourceBranch); err == nil {
		head = h // open PR 的源分支可能已前进
	}
	_, err := Trigger(st, TriggerOpts{
		Owner: pr.Owner, Repo: pr.Repo, SHA: head,
		Ref: pr.SourceBranch, By: by, Event: "pull_request",
	})
	if err != nil && !ignorableTriggerErr(err) {
		log.Printf("pipeline: pull_request trigger %s/%s#%d: %v", pr.Owner, pr.Repo, pr.Number, err)
	}
}
