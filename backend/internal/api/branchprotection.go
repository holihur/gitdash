package api

import (
	"net/http"

	"gitdash/backend/internal/gitsvc"
	"gitdash/backend/internal/store"
)

// ---- 分支保护 ----

// listBranchProtections 列出仓库的分支保护规则。
//
//	@Summary     列出分支保护
//	@Tags        branch-protection
//	@Produce     json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Success     200 {array} store.BranchProtection
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/branch-protections [get]
func (a *API) listBranchProtections(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, false)
	if !ok {
		return
	}
	prots, err := a.store.ListBranchProtections(owner, name)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, prots)
}

// setBranchProtection 创建或更新分支保护。
//
//	@Summary     设置分支保护
//	@Description 需要 owner 权限；min_approvals>0 时合并该分支的 PR 需要对应数量的 approve。
//	@Tags        branch-protection
//	@Accept      json
//	@Param       owner  path string true "仓库所有者"
//	@Param       name   path string true "仓库名"
//	@Param       branch path string true "分支名"
//	@Param       body   body setBranchProtectionReq true "保护规则"
//	@Success     200 {object} store.BranchProtection
//	@Failure     400 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/branch-protections/{branch} [put]
func (a *API) setBranchProtection(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireOwner(w, r)
	if !ok {
		return
	}
	branch := r.PathValue("branch")
	if !gitsvc.ValidRef(branch) {
		writeCode(w, http.StatusBadRequest, "invalid_ref_name", "invalid branch name")
		return
	}
	var in setBranchProtectionReq
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	bp := store.BranchProtection{
		Owner: owner, Repo: name, Branch: branch,
		MinApprovals: in.MinApprovals, BlockDeletion: in.BlockDeletion, BlockForcePush: in.BlockForcePush,
	}
	if err := a.store.SetBranchProtection(&bp); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, bp)
}

// deleteBranchProtection 移除分支保护。
//
//	@Summary     移除分支保护
//	@Tags        branch-protection
//	@Param       owner  path string true "仓库所有者"
//	@Param       name   path string true "仓库名"
//	@Param       branch path string true "分支名"
//	@Success     204 {object} nil
//	@Failure     404 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/branch-protections/{branch} [delete]
func (a *API) deleteBranchProtection(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireOwner(w, r)
	if !ok {
		return
	}
	if err := a.store.DeleteBranchProtection(owner, name, r.PathValue("branch")); err != nil {
		writeNotFound(w, "branch protection")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// branchProtectionGuard 分支名 → 该分支的保护规则（无则 ok=false）。
func (a *API) branchProtectionGuard(owner, name, branch string) (store.BranchProtection, bool) {
	prot, err := a.store.GetBranchProtection(owner, name, branch)
	return prot, err == nil && (prot.MinApprovals > 0 || prot.BlockDeletion || prot.BlockForcePush)
}
