package api

import (
	"errors"
	"gitdash/backend/internal/gitsvc"
	"gitdash/backend/internal/store"
	"net/http"
	"strings"
)

// ---- star & fork ----

func (a *API) writeStarState(w http.ResponseWriter, owner, name, me string) {
	counts := a.store.StarCounts([][2]string{{owner, name}})
	writeJSON(w, http.StatusOK, map[string]any{
		"starred": a.store.IsStarred(me, owner, name),
		"stars":   counts[[2]string{owner, name}],
	})
}

// starRepo 收藏仓库。
//
//	@Summary     收藏仓库
//	@Description 幂等；返回当前收藏状态与 star 数。
//	@Tags        repos
//	@Produce     json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Success     200 {object} map[string]any "starred 与 stars"
//	@Failure     500 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/star [put]
func (a *API) starRepo(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, false)
	if !ok {
		return
	}
	me := userFrom(r)
	if !a.store.IsStarred(me, owner, name) {
		if err := a.store.StarRepo(me, owner, name); err != nil && !errors.Is(err, store.ErrExists) {
			internalError(w, err)
			return
		}
	}
	a.writeStarState(w, owner, name, me)
}

// unstarRepo 取消收藏仓库。
//
//	@Summary     取消收藏
//	@Description 返回当前收藏状态与 star 数。
//	@Tags        repos
//	@Produce     json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Success     200 {object} map[string]any "starred 与 stars"
//	@Failure     500 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/star [delete]
func (a *API) unstarRepo(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, false)
	if !ok {
		return
	}
	me := userFrom(r)
	if err := a.store.UnstarRepo(me, owner, name); err != nil {
		internalError(w, err)
		return
	}
	a.writeStarState(w, owner, name, me)
}

// listStarred 列出当前用户收藏的仓库。
//
//	@Summary     列出收藏仓库
//	@Tags        repos
//	@Produce     json
//	@Success     200 {array} store.Repo
//	@Failure     500 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /starred [get]
func (a *API) listStarred(w http.ResponseWriter, r *http.Request) {
	me := userFrom(r)
	repos, err := a.store.StarredRepos(me)
	if err != nil {
		internalError(w, err)
		return
	}
	a.attachStars(repos, me)
	writeJSON(w, http.StatusOK, repos)
}

// forkRepo fork 仓库。
//
//	@Summary     fork 仓库
//	@Description fork 保持源仓库可见性，创建成功返回 201。
//	@Tags        repos
//	@Accept      json
//	@Produce     json
//	@Param       owner path string true "源仓库所有者"
//	@Param       name  path string true "源仓库名"
//	@Param       body  body forkRepoReq true "目标名称与组织命名空间（可选）"
//	@Success     201 {object} store.Repo
//	@Failure     400 {object} map[string]string
//	@Failure     403 {object} map[string]string
//	@Failure     409 {object} map[string]string
//	@Failure     500 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/fork [post]
func (a *API) forkRepo(w http.ResponseWriter, r *http.Request) {
	srcOwner, srcName, ok := a.requireAccess(w, r, false)
	if !ok {
		return
	}
	var in forkRepoReq
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	me := userFrom(r)
	// 禁止 fork 自己的项目（无论目标是个人还是组织命名空间）。
	if srcOwner == me {
		writeCode(w, http.StatusBadRequest, "cannot_fork_own_repo", "you cannot fork your own repository")
		return
	}
	targetOwner := me
	if ns := strings.TrimSpace(in.Namespace); ns != "" && ns != me {
		if !a.store.IsOrg(ns) || a.store.OrgRole(ns, me) == "" {
			writeCode(w, http.StatusForbidden, "org_forbidden", "you are not a member of this organization")
			return
		}
		targetOwner = ns
	}
	targetName := strings.TrimSpace(in.Name)
	if targetName == "" {
		targetName = srcName
	}
	if !gitsvc.ValidName(targetName) {
		writeCode(w, http.StatusBadRequest, "repo_name_invalid", "invalid name: use letters, digits, '.', '_' or '-' (must start alphanumeric)")
		return
	}
	if _, err := a.store.GetRepo(targetOwner, targetName); err == nil || gitsvc.Exists(targetOwner, targetName) {
		writeCode(w, http.StatusConflict, "repo_exists", "repo already exists")
		return
	}
	srcRepo, err := a.store.GetRepo(srcOwner, srcName)
	if err != nil {
		internalError(w, err)
		return
	}
	// fork 保持源仓库可见性（私有源 -> 私有 fork；公开源 -> 公开 fork）
	repo, err := a.store.CreateRepo(targetOwner, targetName, srcRepo.Description, srcRepo.Private)
	if err != nil {
		internalError(w, err)
		return
	}
	if err := gitsvc.ForkRepo(srcOwner, srcName, targetOwner, targetName); err != nil {
		_ = a.store.DeleteRepo(targetOwner, targetName)
		internalError(w, err)
		return
	}
	if err := a.store.SetForkSource(targetOwner, targetName, srcOwner, srcName); err != nil {
		_ = a.store.DeleteRepo(targetOwner, targetName)
		_ = gitsvc.Delete(targetOwner, targetName)
		internalError(w, err)
		return
	}
	// fork 者自动 watch 自己的 fork（源仓库不一定可见/可 watch）
	_ = a.store.WatchRepo(me, targetOwner, targetName)
	repo.Watchers = 1
	repo.Watching = true
	writeJSON(w, http.StatusCreated, repo)
}
