package api

import (
	"net/http"
	"sort"

	"gitdash/backend/internal/gitsvc"
	"gitdash/backend/internal/store"
)

// codeownerFile 变更文件及其 code owners。
type codeownerFile struct {
	Path   string   `json:"path"`
	Owners []string `json:"owners"`
}

// codeownersStatus 计算 PR 变更文件的 CODEOWNERS 需求与当前批准情况。
// required/approved/missing 为去重后的用户名；无 CODEOWNERS 时全部为空。
func (a *API) codeownersStatus(owner, name string, pr store.PullRequest) (files []codeownerFile, required, approved, missing []string, err error) {
	files = []codeownerFile{}
	required, approved, missing = []string{}, []string{}, []string{}
	if pr.State != "open" {
		return files, required, approved, missing, nil
	}
	co := gitsvc.LoadCodeowners(owner, name, "refs/heads/"+pr.TargetBranch)
	if co == nil {
		return files, required, approved, missing, nil
	}
	head := pr.HeadSHA
	if h, herr := gitsvc.RevSHA(owner, name, "refs/heads/"+pr.SourceBranch); herr == nil {
		head = h
	}
	diff, err := gitsvc.DiffStats(owner, name, pr.BaseSHA, head)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	requiredSet := map[string]bool{}
	for _, f := range diff {
		owns := co.Owners(f.Path)
		if len(owns) == 0 {
			continue
		}
		files = append(files, codeownerFile{Path: f.Path, Owners: owns})
		for _, o := range owns {
			if !requiredSet[o] {
				requiredSet[o] = true
				required = append(required, o)
			}
		}
	}
	if len(required) == 0 {
		return files, required, approved, missing, nil
	}
	reviews, _, rerr := a.store.ListReviews(owner, name, pr.Number)
	if rerr != nil {
		return nil, nil, nil, nil, rerr
	}
	latest := map[string]store.PullReview{}
	for _, rv := range reviews {
		if prev, ok := latest[rv.Reviewer]; !ok || rv.ID > prev.ID {
			latest[rv.Reviewer] = rv
		}
	}
	for _, o := range required {
		rv, ok := latest[o]
		if ok && rv.State == "approve" && rv.Reviewer != pr.Author && rv.CommitSHA == head {
			approved = append(approved, o)
		} else {
			missing = append(missing, o)
		}
	}
	sort.Strings(required)
	sort.Strings(approved)
	sort.Strings(missing)
	return files, required, approved, missing, nil
}

// pullCodeowners 返回 PR 变更文件的 code owners 及批准状态。
//
//	@Summary     获取 PR 的 CODEOWNERS 状态
//	@Tags        pulls
//	@Produce     json
//	@Param       owner  path string true "仓库所有者"
//	@Param       name   path string true "仓库名"
//	@Param       number path int    true "PR 编号"
//	@Success     200 {object} object "files、owners、approved、missing、satisfied"
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/pulls/{number}/codeowners [get]
func (a *API) pullCodeowners(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, false)
	if !ok {
		return
	}
	pr, err := a.getPullOr404(w, owner, name, r.PathValue("number"))
	if err != nil {
		return
	}
	files, required, approved, missing, err := a.codeownersStatus(owner, name, pr)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"files":     files,
		"owners":    required,
		"approved":  approved,
		"missing":   missing,
		"satisfied": len(missing) == 0,
	})
}
