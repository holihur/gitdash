package api

import (
	"errors"
	"fmt"
	"gitdash/backend/internal/gitsvc"
	"gitdash/backend/internal/store"
	"net/http"
	"strconv"
	"strings"
)

// listPulls 列出仓库的 pull request 列表。
//
//	@Summary     列出 PR
//	@Tags        pulls
//	@Produce     json
//	@Param       owner  path string true "仓库所有者"
//	@Param       name   path string true "仓库名"
//	@Param       state  query string false "状态过滤（open/closed/merged）"
//	@Param       limit  query int    false "每页数量"
//	@Param       offset query int    false "偏移量"
//	@Success     200 {array} store.PullRequest
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/pulls [get]
func (a *API) listPulls(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, false)
	if !ok {
		return
	}
	limit, offset := pageParams(r)
	state := r.URL.Query().Get("state")
	pulls, err := a.store.ListPulls(owner, name, state, limit, offset)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	total, err := a.store.CountPulls(owner, name, state)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	setTotal(w, total)
	writeJSON(w, http.StatusOK, pulls)
}

// createPull 创建 pull request。
//
//	@Summary     创建 PR
//	@Tags        pulls
//	@Accept      json
//	@Produce     json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Param       body  body createPullReq true "标题、正文、源分支与目标分支"
//	@Success     201 {object} store.PullRequest
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/pulls [post]
func (a *API) createPull(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, true)
	if !ok {
		return
	}
	var in struct {
		Title        string `json:"title"`
		Body         string `json:"body"`
		SourceBranch string `json:"source_branch"`
		TargetBranch string `json:"target_branch"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	in.Title = strings.TrimSpace(in.Title)
	in.SourceBranch = strings.TrimSpace(strings.TrimPrefix(in.SourceBranch, "refs/heads/"))
	in.TargetBranch = strings.TrimSpace(strings.TrimPrefix(in.TargetBranch, "refs/heads/"))
	if in.Title == "" {
		writeCode(w, http.StatusBadRequest, "title_required", "title is required")
		return
	}
	if in.SourceBranch == "" || in.TargetBranch == "" {
		writeCode(w, http.StatusBadRequest, "branch_not_found", "source and target branches are required")
		return
	}
	if in.SourceBranch == in.TargetBranch {
		writeCode(w, http.StatusBadRequest, "same_branch", "source and target branch must differ")
		return
	}
	srcSHA, err := gitsvc.RevSHA(owner, name, "refs/heads/"+in.SourceBranch)
	if err != nil {
		writeCode(w, http.StatusBadRequest, "branch_not_found", "source branch not found: "+in.SourceBranch)
		return
	}
	baseSHA, err := gitsvc.RevSHA(owner, name, "refs/heads/"+in.TargetBranch)
	if err != nil {
		writeCode(w, http.StatusBadRequest, "branch_not_found", "target branch not found: "+in.TargetBranch)
		return
	}
	pr, err := a.store.CreatePull(owner, name, userFrom(r), in.Title, in.Body, in.SourceBranch, in.TargetBranch, baseSHA, srcSHA)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.notify(owner, name, "pull", "opened", userFrom(r), pr.Number, pr.Title, "")
	writeJSON(w, http.StatusCreated, pr)
}

// getPull 获取单个 pull request。
//
//	@Summary     获取 PR
//	@Tags        pulls
//	@Produce     json
//	@Param       owner  path string true "仓库所有者"
//	@Param       name   path string true "仓库名"
//	@Param       number path int    true "PR 编号"
//	@Success     200 {object} store.PullRequest
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/pulls/{number} [get]
func (a *API) getPull(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, false)
	if !ok {
		return
	}
	pr, err := a.getPullOr404(w, owner, name, r.PathValue("number"))
	if err != nil {
		return
	}
	// 实时状态：base 是否仍可快进到 head（对 open 有意义）
	writeJSON(w, http.StatusOK, pr)
}

func (a *API) getPullOr404(w http.ResponseWriter, owner, name, num string) (store.PullRequest, error) {
	n, err := strconv.ParseInt(num, 10, 64)
	if err != nil || n < 1 {
		writeCode(w, http.StatusBadRequest, "invalid_issue_number", "invalid pull request number")
		return store.PullRequest{}, err
	}
	pr, err := a.store.GetPull(owner, name, n)
	if errors.Is(err, store.ErrNotFound) {
		writeCode(w, http.StatusNotFound, "pr_not_found", "pull request not found")
		return store.PullRequest{}, err
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return store.PullRequest{}, err
	}
	return pr, nil
}

// pullDiff 获取 pull request 的 diff（文件统计与补丁）。
//
//	@Summary     获取 PR diff
//	@Tags        pulls
//	@Produce     json
//	@Param       owner  path string true "仓库所有者"
//	@Param       name   path string true "仓库名"
//	@Param       number path int    true "PR 编号"
//	@Success     200 {object} object "files、patch、base_sha、head_sha"
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/pulls/{number}/diff [get]
func (a *API) pullDiff(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, false)
	if !ok {
		return
	}
	pr, err := a.getPullOr404(w, owner, name, r.PathValue("number"))
	if err != nil {
		return
	}
	base := pr.BaseSHA
	head := pr.HeadSHA
	// open 状态实时取分支 tip（分支可能继续演进）
	if pr.State == "open" {
		if h, err := gitsvc.RevSHA(owner, name, "refs/heads/"+pr.SourceBranch); err == nil {
			head = h
		}
	}
	files, err := gitsvc.DiffStats(owner, name, base, head)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	patch, _ := gitsvc.DiffPatch(owner, name, base, head)
	writeJSON(w, http.StatusOK, map[string]any{"files": files, "patch": patch, "base_sha": base, "head_sha": head})
}

// mergePull 合并 pull request。
//
//	@Summary     合并 PR
//	@Tags        pulls
//	@Accept      json
//	@Produce     json
//	@Param       owner  path string true "仓库所有者"
//	@Param       name   path string true "仓库名"
//	@Param       number path int    true "PR 编号"
//	@Param       body   body mergePullReq true "合并方式（fast-forward/merge/squash）"
//	@Success     200 {object} store.PullRequest
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/pulls/{number}/merge [post]
func (a *API) mergePull(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, true)
	if !ok {
		return
	}
	pr, err := a.getPullOr404(w, owner, name, r.PathValue("number"))
	if err != nil {
		return
	}
	if pr.State != "open" {
		writeCode(w, http.StatusConflict, "pull_not_mergeable", "only open pull requests can be merged")
		return
	}
	// 合并门禁：目标分支保护规则要求的最少 approve 数。
	// 有效 approve = reviewer 最新状态为 approve、reviewer 非 PR 作者、针对当前 head（head 前进后过期失效）。
	if prot, pErr := a.store.GetBranchProtection(owner, name, pr.TargetBranch); pErr == nil && prot.MinApprovals > 0 {
		head := pr.HeadSHA
		if h, hErr := gitsvc.RevSHA(owner, name, "refs/heads/"+pr.SourceBranch); hErr == nil {
			head = h
		}
		reviews, _, lErr := a.store.ListReviews(owner, name, pr.Number)
		if lErr != nil {
			writeErr(w, http.StatusInternalServerError, lErr.Error())
			return
		}
		latest := map[string]store.PullReview{}
		for _, rv := range reviews {
			if prev, ok := latest[rv.Reviewer]; !ok || rv.ID > prev.ID {
				latest[rv.Reviewer] = rv
			}
		}
		valid := 0
		for _, rv := range latest {
			if rv.State == "approve" && rv.Reviewer != pr.Author && rv.CommitSHA == head {
				valid++
			}
		}
		if valid < prot.MinApprovals {
			writeCode(w, http.StatusConflict, "review_required",
				fmt.Sprintf("branch %q requires %d approval(s) from reviewers other than the PR author (current: %d; approvals on an older head do not count)",
					pr.TargetBranch, prot.MinApprovals, valid))
			return
		}
	}
	var in struct {
		Method string `json:"method"` // ""(fast-forward) | merge | squash
	}
	if r.ContentLength > 0 {
		if rerr := readJSON(w, r, &in); rerr != nil {
			return
		}
	}
	var headSHA string
	switch in.Method {
	case "", "fast-forward":
		h, mErr := gitsvc.MergeFastForward(owner, name, pr.TargetBranch, pr.SourceBranch)
		if mErr != nil {
			writeCode(w, http.StatusConflict, "merge_not_ff",
				"branches diverged; merge with method \"merge\" or \"squash\", or rebase locally: "+mErr.Error())
			return
		}
		headSHA = h
	case "merge", "squash":
		msg := fmt.Sprintf("Merge pull request #%d from %s: %s", pr.Number, pr.SourceBranch, pr.Title)
		h, mErr := gitsvc.MergeNonFF(owner, name, pr.TargetBranch, pr.SourceBranch, msg, userFrom(r), in.Method)
		if mErr != nil {
			writeCode(w, http.StatusConflict, "merge_conflict", mErr.Error())
			return
		}
		headSHA = h
	default:
		writeCode(w, http.StatusBadRequest, "invalid_merge_method", "method must be 'merge' or 'squash'")
		return
	}
	merged, err := a.store.MarkPullMerged(owner, name, pr.Number, headSHA, userFrom(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.notify(owner, name, "pull", "merged", userFrom(r), merged.Number, merged.Title, "")
	writeJSON(w, http.StatusOK, merged)
}

// setPullState 修改 pull request 状态（open/closed）。
//
//	@Summary     修改 PR 状态
//	@Tags        pulls
//	@Accept      json
//	@Produce     json
//	@Param       owner  path string true "仓库所有者"
//	@Param       name   path string true "仓库名"
//	@Param       number path int    true "PR 编号"
//	@Param       body   body setPullStateReq true "状态（open/closed）"
//	@Success     200 {object} store.PullRequest
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/pulls/{number}/state [post]
func (a *API) setPullState(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, true)
	if !ok {
		return
	}
	pr, err := a.getPullOr404(w, owner, name, r.PathValue("number"))
	if err != nil {
		return
	}
	var in struct {
		State string `json:"state"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	if pr.State == "merged" || (in.State != "open" && in.State != "closed") {
		writeCode(w, http.StatusBadRequest, "invalid_state", "state must be open/closed and not merged")
		return
	}
	updated, err := a.store.SetPullState(owner, name, pr.Number, in.State)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	// 状态未变化（如重复 close）不重复发通知
	if pr.State != updated.State {
		action := "closed"
		if updated.State == "open" {
			action = "reopened"
		}
		a.notify(owner, name, "pull", action, userFrom(r), updated.Number, updated.Title, "")
	}
	writeJSON(w, http.StatusOK, updated)
}

// ---- issue labels & milestones ----
