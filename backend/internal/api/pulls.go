package api

import (
	"errors"
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
		internalError(w, err)
		return
	}
	total, err := a.store.CountPulls(owner, name, state)
	if err != nil {
		internalError(w, err)
		return
	}
	setTotal(w, total)
	for i := range pulls {
		a.enrichPull(owner, name, &pulls[i])
	}
	writeJSON(w, http.StatusOK, pulls)
}

// enrichPull 为 open PR 附加实时信息：可合并性预检（mergeable/conflicted）与 head 提交的 CI 状态。
func (a *API) enrichPull(owner, name string, pr *store.PullRequest) {
	if pr.State != "open" {
		return
	}
	m, c := gitsvc.MergeCheck(owner, name, "refs/heads/"+pr.TargetBranch, "refs/heads/"+pr.SourceBranch)
	pr.Mergeable = &m
	pr.Conflicted = c
	if ci, ok, err := a.store.AggregatePipelineStatusForSHA(owner, name, pr.HeadSHA); err == nil && ok {
		pr.CI = &ci
	}
	if _, ok := a.store.GetMergeEntry(owner, name, pr.Number); ok {
		pr.MergeQueued = true
	}
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
		Draft        bool   `json:"draft"`
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
	pr, err := a.store.CreatePull(owner, name, userFrom(r), in.Title, in.Body, in.SourceBranch, in.TargetBranch, baseSHA, srcSHA, in.Draft)
	if err != nil {
		internalError(w, err)
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
	a.enrichPull(owner, name, &pr)
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
		internalError(w, err)
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
//	@Param       body   body mergePullReq true "合并方式（fast-forward/merge/squash/rebase）"
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
	var in struct {
		Method string `json:"method"` // ""(fast-forward) | merge | squash | rebase
	}
	if r.ContentLength > 0 {
		if rerr := readJSON(w, r, &in); rerr != nil {
			return
		}
	}
	// 目标分支开启合并队列时：入队串行合并，而非立即合并。
	if a.branchHasMergeQueue(owner, name, pr.TargetBranch) {
		switch in.Method {
		case "", "fast-forward", "merge", "squash", "rebase":
		default:
			writeCode(w, http.StatusBadRequest, "invalid_merge_method", "method must be 'fast-forward', 'merge', 'squash' or 'rebase'")
			return
		}
		if pr.State != "open" {
			writeCode(w, http.StatusConflict, "pull_not_mergeable", "only open pull requests can be merged")
			return
		}
		if err := a.store.EnqueueMerge(owner, name, pr.TargetBranch, pr.Number, in.Method, userFrom(r)); err != nil {
			internalError(w, err)
			return
		}
		a.processMergeQueue(owner, name, pr.TargetBranch)
		fresh, ferr := a.store.GetPull(owner, name, pr.Number)
		if ferr != nil {
			internalError(w, ferr)
			return
		}
		if fresh.State == "open" {
			a.enrichPull(owner, name, &fresh)
			writeJSON(w, http.StatusAccepted, fresh)
			return
		}
		writeJSON(w, http.StatusOK, fresh)
		return
	}
	merged, ge := a.executeMerge(owner, name, pr, in.Method, userFrom(r))
	if ge != nil {
		if ge.internal {
			writeErr(w, ge.status, ge.msg)
		} else {
			writeCode(w, ge.status, ge.code, ge.msg)
		}
		return
	}
	writeJSON(w, http.StatusOK, merged)
}

// setPullAutoMerge 开启/关闭 PR 自动合并。
//
//	@Summary     设置 PR 自动合并
//	@Description 需要仓库写权限；开启后当合并门禁满足时由服务端自动按指定方式合并。
//	@Tags        pulls
//	@Accept      json
//	@Produce     json
//	@Param       owner  path string true "仓库所有者"
//	@Param       name   path string true "仓库名"
//	@Param       number path int    true "PR 编号"
//	@Param       body   body object true "enabled 与可选 method"
//	@Success     200 {object} store.PullRequest
//	@Failure     400 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/pulls/{number}/auto-merge [post]
func (a *API) setPullAutoMerge(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, true)
	if !ok {
		return
	}
	pr, err := a.getPullOr404(w, owner, name, r.PathValue("number"))
	if err != nil {
		return
	}
	var in struct {
		Enabled bool   `json:"enabled"`
		Method  string `json:"method"`
	}
	if rerr := readJSON(w, r, &in); rerr != nil {
		return
	}
	switch in.Method {
	case "", "fast-forward", "merge", "squash", "rebase":
	default:
		writeCode(w, http.StatusBadRequest, "invalid_merge_method", "method must be 'fast-forward', 'merge', 'squash' or 'rebase'")
		return
	}
	if pr.State != "open" {
		writeCode(w, http.StatusBadRequest, "pull_not_open", "auto-merge can only be set on open pull requests")
		return
	}
	updated, err := a.store.SetPullAutoMerge(owner, name, pr.Number, in.Enabled, in.Method)
	if err != nil {
		internalError(w, err)
		return
	}
	if updated.AutoMerge {
		// 立即尝试一次；门禁未满足则保持开启，等待后续 review / CI 事件。
		a.tryAutoMerge(owner, name, updated.Number)
		if fresh, ferr := a.store.GetPull(owner, name, updated.Number); ferr == nil {
			updated = fresh
		}
	}
	writeJSON(w, http.StatusOK, updated)
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
		internalError(w, err)
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

// setPullDraft 切换 PR 草稿状态（仅 open）。
//
//	@Summary     切换 PR 草稿状态
//	@Tags        pulls
//	@Accept      json
//	@Produce     json
//	@Param       owner  path string true "仓库所有者"
//	@Param       name   path string true "仓库名"
//	@Param       number path int    true "PR 编号"
//	@Param       body   body object true "draft: true/false"
//	@Success     200 {object} store.PullRequest
//	@Failure     400 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/pulls/{number}/draft [post]
func (a *API) setPullDraft(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, true)
	if !ok {
		return
	}
	pr, err := a.getPullOr404(w, owner, name, r.PathValue("number"))
	if err != nil {
		return
	}
	var in struct {
		Draft bool `json:"draft"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	if pr.State != "open" {
		writeCode(w, http.StatusBadRequest, "invalid_state", "only open pull requests can change draft state")
		return
	}
	if pr.Draft == in.Draft {
		writeJSON(w, http.StatusOK, pr)
		return
	}
	updated, err := a.store.SetPullDraft(owner, name, pr.Number, in.Draft)
	if err != nil {
		internalError(w, err)
		return
	}
	action := "converted_to_draft"
	if !updated.Draft {
		action = "ready_for_review"
	}
	a.notify(owner, name, "pull", action, userFrom(r), updated.Number, updated.Title, "")
	writeJSON(w, http.StatusOK, updated)
}

// ---- issue labels & milestones ----
