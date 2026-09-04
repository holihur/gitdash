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

func (a *API) listPulls(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, false)
	if !ok {
		return
	}
	pulls, err := a.store.ListPulls(owner, name, r.URL.Query().Get("state"))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, pulls)
}

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
	writeJSON(w, http.StatusCreated, pr)
}

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
	writeJSON(w, http.StatusOK, merged)
}

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
	writeJSON(w, http.StatusOK, updated)
}

// ---- issue labels & milestones ----
