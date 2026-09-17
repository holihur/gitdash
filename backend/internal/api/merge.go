package api

import (
	"fmt"
	"net/http"
	"strings"

	"gitdash/backend/internal/gitsvc"
	"gitdash/backend/internal/store"
)

// mergeGateErr 合并门禁/执行失败：HTTP 状态 + 错误码 + 消息。
type mergeGateErr struct {
	status   int
	code     string
	msg      string
	internal bool // true 用 writeErr（内部错误），否则 writeCode（业务错误）
}

// mergeGateError 校验 PR 是否满足合并门禁；nil 表示可合并。
func (a *API) mergeGateError(owner, name string, pr store.PullRequest) *mergeGateErr {
	if pr.State != "open" {
		return &mergeGateErr{http.StatusConflict, "pull_not_mergeable", "only open pull requests can be merged", false}
	}
	if pr.Draft {
		return &mergeGateErr{http.StatusConflict, "pull_is_draft", "draft pull requests cannot be merged; mark it ready for review first", false}
	}
	prot, pErr := a.store.GetBranchProtection(owner, name, pr.TargetBranch)
	if pErr != nil {
		return nil // 未设置保护规则
	}
	// CODEOWNERS：变更文件声明的 owner 需逐个批准。
	if prot.RequireCodeowners {
		_, _, _, missing, coErr := a.codeownersStatus(owner, name, pr)
		if coErr != nil {
			return &mergeGateErr{http.StatusInternalServerError, "internal", coErr.Error(), true}
		}
		if len(missing) > 0 {
			return &mergeGateErr{http.StatusConflict, "codeowners_required",
				fmt.Sprintf("merge blocked: code owner approval required from %s", strings.Join(missing, ", ")), false}
		}
	}
	head := pr.HeadSHA
	if h, hErr := gitsvc.RevSHA(owner, name, "refs/heads/"+pr.SourceBranch); hErr == nil {
		head = h
	}
	if prot.RequireCI {
		status := "missing"
		passed := false
		if ci, has, cErr := a.store.AggregatePipelineStatusForSHA(owner, name, head); cErr != nil {
			return &mergeGateErr{http.StatusInternalServerError, "internal", cErr.Error(), true}
		} else if has {
			status = ci.Status
			passed = ci.Status == "success"
		}
		if !passed {
			return &mergeGateErr{http.StatusConflict, "ci_required",
				fmt.Sprintf("merge blocked: branch %q requires a successful CI run on the current head (current: %s)", pr.TargetBranch, status), false}
		}
	}
	// 有效 approve = reviewer 最新状态为 approve、reviewer 非 PR 作者、针对当前 head（head 前进后过期失效）。
	reviews, _, lErr := a.store.ListReviews(owner, name, pr.Number)
	if lErr != nil {
		return &mergeGateErr{http.StatusInternalServerError, "internal", lErr.Error(), true}
	}
	latest := map[string]store.PullReview{}
	for _, rv := range reviews {
		if prev, ok := latest[rv.Reviewer]; !ok || rv.ID > prev.ID {
			latest[rv.Reviewer] = rv
		}
	}
	valid := 0
	blocked := false
	for _, rv := range latest {
		switch {
		case rv.State == "approve" && rv.Reviewer != pr.Author && rv.CommitSHA == head:
			valid++
		case rv.State == "request_changes" && rv.Reviewer != pr.Author && rv.CommitSHA == head:
			blocked = true
		}
	}
	if blocked {
		return &mergeGateErr{http.StatusConflict, "changes_requested",
			"merge blocked: a reviewer requested changes (a new approve or a new head commit clears it)", false}
	}
	if valid < prot.MinApprovals {
		return &mergeGateErr{http.StatusConflict, "review_required",
			fmt.Sprintf("branch %q requires %d approval(s) from reviewers other than the PR author (current: %d; approvals on an older head do not count)",
				pr.TargetBranch, prot.MinApprovals, valid), false}
	}
	return nil
}

// executeMerge 通过门禁后按 method 执行合并并记录；用于手动与自动合并。
func (a *API) executeMerge(owner, name string, pr store.PullRequest, method, actor string) (store.PullRequest, *mergeGateErr) {
	if ge := a.mergeGateError(owner, name, pr); ge != nil {
		return store.PullRequest{}, ge
	}
	var headSHA string
	switch method {
	case "", "fast-forward":
		h, mErr := gitsvc.MergeFastForward(owner, name, pr.TargetBranch, pr.SourceBranch)
		if mErr != nil {
			return store.PullRequest{}, &mergeGateErr{http.StatusConflict, "merge_not_ff",
				"branches diverged; merge with method \"merge\", \"squash\" or \"rebase\", or merge locally: " + mErr.Error(), false}
		}
		headSHA = h
	case "merge", "squash":
		msg := fmt.Sprintf("Merge pull request #%d from %s: %s", pr.Number, pr.SourceBranch, pr.Title)
		h, mErr := gitsvc.MergeNonFF(owner, name, pr.TargetBranch, pr.SourceBranch, msg, actor, method)
		if mErr != nil {
			return store.PullRequest{}, &mergeGateErr{http.StatusConflict, "merge_conflict", mErr.Error(), false}
		}
		headSHA = h
	case "rebase":
		h, mErr := gitsvc.MergeRebase(owner, name, pr.TargetBranch, pr.SourceBranch, actor)
		if mErr != nil {
			return store.PullRequest{}, &mergeGateErr{http.StatusConflict, "merge_conflict", mErr.Error(), false}
		}
		headSHA = h
	default:
		return store.PullRequest{}, &mergeGateErr{http.StatusBadRequest, "invalid_merge_method",
			"method must be 'fast-forward', 'merge', 'squash' or 'rebase'", false}
	}
	merged, err := a.store.MarkPullMerged(owner, name, pr.Number, headSHA, actor)
	if err != nil {
		return store.PullRequest{}, &mergeGateErr{http.StatusInternalServerError, "internal", err.Error(), true}
	}
	a.notify(owner, name, "pull", "merged", actor, merged.Number, merged.Title, "")
	return merged, nil
}

// tryAutoMerge 若 PR 开启自动合并且门禁已满足，则按记录方式合并（尽力而为，失败保持开启）。
func (a *API) tryAutoMerge(owner, name string, number int64) {
	pr, err := a.store.GetPull(owner, name, number)
	if err != nil || pr.State != "open" || !pr.AutoMerge {
		return
	}
	_, _ = a.executeMerge(owner, name, pr, pr.AutoMergeMethod, pr.Author)
}

// autoMergeForRepo 尝试合并仓库内全部已开启自动合并的 PR（如 CI 成功后触发）。
func (a *API) autoMergeForRepo(owner, name string) {
	pulls, err := a.store.ListAutoMergePulls(owner, name)
	if err != nil {
		return
	}
	for _, pr := range pulls {
		_, _ = a.executeMerge(owner, name, pr, pr.AutoMergeMethod, pr.Author)
	}
}

// AutoMergeForRepo 供流水线成功回调使用：对仓库内开启自动合并的 PR 再评估一次门禁。
func (a *API) AutoMergeForRepo(owner, repo, _ string) { a.autoMergeForRepo(owner, repo) }
