package api

import (
	"net/http"
	"time"

	"gitdash/backend/internal/gitsvc"
	"gitdash/backend/internal/store"
	"gitdash/backend/internal/webhooks"
)

// validReviewState PR review 合法状态集合。
var validReviewState = map[string]bool{
	"approve":         true,
	"request_changes": true,
	"comment":         true,
}

// createReview 提交 PR review（approve / request_changes / comment）。
//
//	@Summary     提交 PR review
//	@Description 需要仓库写权限；同一 reviewer 重复提交插入新行（保留历史）。成功返回 201 并向关注者推送通知与 webhook 事件。
//	@Tags        pulls
//	@Accept      json
//	@Produce     json
//	@Param       owner  path string true "仓库所有者"
//	@Param       name   path string true "仓库名"
//	@Param       number path int    true "PR 编号"
//	@Param       body   body createReviewReq true "review 状态与说明"
//	@Success     201 {object} store.PullReview
//	@Failure     400 {object} map[string]string
//	@Failure     404 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/pulls/{number}/reviews [post]
func (a *API) createReview(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, true)
	if !ok {
		return
	}
	pr, err := a.getPullOr404(w, owner, name, r.PathValue("number"))
	if err != nil {
		return
	}
	var in createReviewReq
	if jerr := readJSON(w, r, &in); jerr != nil {
		return
	}
	if !validReviewState[in.State] {
		writeCode(w, http.StatusBadRequest, "invalid_review_state", "state must be approve, request_changes or comment")
		return
	}
	// commit_sha 缺省取当前 head：用于合并门禁的过期判定（head 前进后旧 approve 失效）
	commitSHA := in.CommitSHA
	if commitSHA == "" {
		commitSHA = pr.HeadSHA
		if pr.State == "open" {
			if h, hErr := gitsvc.RevSHA(owner, name, "refs/heads/"+pr.SourceBranch); hErr == nil {
				commitSHA = h
			}
		}
	}
	me := userFrom(r)
	review, err := a.store.CreateReview(owner, name, pr.Number, me, in.State, in.Body, commitSHA)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	// 关注者通知 + webhook 事件（review:state）
	a.notify(owner, name, "pull", "reviewed", me, pr.Number, pr.Title, in.State)
	if a.Publish != nil {
		go a.Publish(webhooks.Event{
			Event:     "pull",
			Owner:     owner,
			Repo:      name,
			Kind:      "pull",
			Action:    "review:" + in.State,
			Number:    pr.Number,
			Title:     pr.Title,
			Actor:     me,
			Comment:   in.State,
			CreatedAt: time.Now().UTC().Format(time.RFC3339),
		})
	}
	writeJSON(w, http.StatusCreated, review)
}

// listReviews 列出 PR reviews 与各 reviewer 最新状态汇总。
//
//	@Summary     列出 PR reviews
//	@Tags        pulls
//	@Produce     json
//	@Param       owner  path string true "仓库所有者"
//	@Param       name   path string true "仓库名"
//	@Param       number path int    true "PR 编号"
//	@Success     200 {object} object "reviews 数组与 summary {approvals, request_changes}"
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/pulls/{number}/reviews [get]
func (a *API) listReviews(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, false)
	if !ok {
		return
	}
	pr, err := a.getPullOr404(w, owner, name, r.PathValue("number"))
	if err != nil {
		return
	}
	reviews, summary, err := a.store.ListReviews(owner, name, pr.Number)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	resp := map[string]any{"reviews": reviews, "summary": summary}
	// 合并门禁信息：目标分支保护规则要求的 approve 数 vs 有效 approve 数
	// （reviewer 非 PR 作者，且其最新 approve 针对当前 head —— head 前进后过期失效）
	if prot, pErr := a.store.GetBranchProtection(owner, name, pr.TargetBranch); pErr == nil && prot.MinApprovals > 0 {
		head := pr.HeadSHA
		if pr.State == "open" {
			if h, hErr := gitsvc.RevSHA(owner, name, "refs/heads/"+pr.SourceBranch); hErr == nil {
				head = h
			}
		}
		valid := 0
		latest := map[string]store.PullReview{}
		for _, rv := range reviews {
			prev, ok := latest[rv.Reviewer]
			if !ok || rv.ID > prev.ID {
				latest[rv.Reviewer] = rv
			}
		}
		for _, rv := range latest {
			if rv.State == "approve" && rv.Reviewer != pr.Author && rv.CommitSHA == head {
				valid++
			}
		}
		resp["gate"] = map[string]any{
			"required":  prot.MinApprovals,
			"approvals": valid,
			"mergeable": valid >= prot.MinApprovals,
		}
	}
	writeJSON(w, http.StatusOK, resp)
}
