package api

import (
	"errors"
	"gitdash/backend/internal/store"
	"net/http"
	"strings"
)

// ---- visibility / explore / collabs ----

// listOrgRepos 列出组织仓库。
//
//	@Summary     组织仓库列表
//	@Description 返回组织仓库列表与当前用户在组织中的角色。
//	@Tags        orgs
//	@Produce     json
//	@Param       org path string true "组织名"
//	@Success     200 {object} map[string]any "role 与 repos"
//	@Failure     404 {object} map[string]string
//	@Failure     500 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /orgs/{org}/repos [get]
func (a *API) listOrgRepos(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	me := userFrom(r)
	role := a.store.OrgRole(org, me)
	if role == "" {
		writeCode(w, http.StatusNotFound, "org_not_found", "organization not found")
		return
	}
	rows, err := a.store.QueryOrgRepos(org)
	if err != nil {
		internalError(w, err)
		return
	}
	label := "read"
	switch role {
	case "owner":
		label = "owner"
	case "member":
		label = "write"
	}
	out := append([]store.Repo{}, rows...)
	a.attachStars(out, me)
	writeJSON(w, http.StatusOK, map[string]any{"role": label, "repos": out})
}

// setRepoVisibility 设置仓库可见性。
//
//	@Summary     设置仓库可见性
//	@Description 仅仓库所有者可设置。
//	@Tags        repos
//	@Accept      json
//	@Produce     json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Param       body  body setRepoVisibilityReq true "private"
//	@Success     200 {object} store.Repo
//	@Failure     400 {object} map[string]string
//	@Failure     500 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/visibility [post]
func (a *API) setRepoVisibility(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireOwner(w, r)
	if !ok {
		return
	}
	var in setRepoVisibilityReq
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	if in.Private == nil {
		writeErr(w, http.StatusBadRequest, "missing field: private")
		return
	}
	if err := a.store.SetRepoPrivate(owner, name, *in.Private); err != nil {
		internalError(w, err)
		return
	}
	repo, err := a.store.GetRepo(owner, name)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, repo)
}

// setRepoTemplate 设置模版仓库标记。
//
//	@Summary     设置模版仓库
//	@Description 仅仓库所有者可设置。模版仓库可作为创建新仓库的源。
//	@Tags        repos
//	@Accept      json
//	@Produce     json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Param       body  body setRepoTemplateReq true "is_template"
//	@Success     200 {object} store.Repo
//	@Failure     400 {object} map[string]string
//	@Failure     500 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/template [post]
func (a *API) setRepoTemplate(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireOwner(w, r)
	if !ok {
		return
	}
	var in setRepoTemplateReq
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	if in.IsTemplate == nil {
		writeErr(w, http.StatusBadRequest, "missing field: is_template")
		return
	}
	if err := a.store.SetRepoTemplate(owner, name, *in.IsTemplate); err != nil {
		internalError(w, err)
		return
	}
	repo, err := a.store.GetRepo(owner, name)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, repo)
}

// listTemplateRepos 列出当前用户可访问的模版仓库。
//
//	@Summary     列出可访问的模版仓库
//	@Tags        repos
//	@Produce     json
//	@Success     200 {array} store.Repo
//	@Security    BearerAuth
//	@Router      /templates [get]
func (a *API) listTemplateRepos(w http.ResponseWriter, r *http.Request) {
	me := userFrom(r)
	repos, err := a.store.ListAccessibleTemplateRepos(me)
	if err != nil {
		internalError(w, err)
		return
	}
	a.attachStars(repos, me)
	writeJSON(w, http.StatusOK, repos)
}

// exploreRepos 列出所有公开仓库。
//
//	@Summary     探索公开仓库
//	@Tags        repos
//	@Produce     json
//	@Param       limit  query int false "每页数量（默认 200，最大 500）"
//	@Param       offset query int false "偏移量"
//	@Success     200 {array} store.Repo
//	@SuccessHeader X-Total-Count int "公开仓库总数"
//	@Failure     500 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /explore/repos [get]
func (a *API) exploreRepos(w http.ResponseWriter, r *http.Request) {
	limit, offset := pageParams(r)
	repos, err := a.store.ExploreRepos(limit, offset)
	if err != nil {
		internalError(w, err)
		return
	}
	total, err := a.store.CountExploreRepos()
	if err != nil {
		internalError(w, err)
		return
	}
	setTotal(w, total)
	// 返回所有公开仓库（含自己的，便于确认可见性设置是否生效）
	me := userFrom(r)
	out := append([]store.Repo{}, repos...)
	a.attachStars(out, me)
	writeJSON(w, http.StatusOK, out)
}

// listCollabs 列出仓库协作者。
//
//	@Summary     列出协作者
//	@Description 仅仓库所有者可查看。
//	@Tags        repos
//	@Produce     json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Success     200 {array} store.Collab
//	@Failure     500 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/collabs [get]
func (a *API) listCollabs(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireOwner(w, r)
	if !ok {
		return
	}
	collabs, err := a.store.ListCollabs(owner, name)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, collabs)
}

// addCollab 添加或更新协作者。
//
//	@Summary     添加协作者
//	@Description permission 为 read 或 write；仅仓库所有者可操作。
//	@Tags        repos
//	@Accept      json
//	@Produce     json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Param       body  body addCollabReq true "username 与 permission"
//	@Success     200 {object} store.Collab
//	@Failure     400 {object} map[string]string
//	@Failure     404 {object} map[string]string
//	@Failure     500 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/collabs [post]
func (a *API) addCollab(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireOwner(w, r)
	if !ok {
		return
	}
	var in addCollabReq
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	in.Username = strings.ToLower(strings.TrimSpace(in.Username))
	if !usernameRe.MatchString(in.Username) {
		writeCode(w, http.StatusBadRequest, "username_invalid", "invalid collaborator username")
		return
	}
	if in.Username == userFrom(r) {
		writeCode(w, http.StatusBadRequest, "owner_as_collab", "owner is already the owner")
		return
	}
	if in.Permission != "read" && in.Permission != "write" {
		writeCode(w, http.StatusBadRequest, "invalid_permission", "permission must be 'read' or 'write'")
		return
	}
	if _, err := a.store.GetByUsername(in.Username); err != nil {
		writeCode(w, http.StatusNotFound, "user_not_found", "user not found")
		return
	}
	if err := a.store.UpsertCollab(owner, name, in.Username, in.Permission); err != nil {
		internalError(w, err)
		return
	}
	collabs, err := a.store.ListCollabs(owner, name)
	if err != nil {
		internalError(w, err)
		return
	}
	for _, c := range collabs {
		if c.Username == in.Username {
			writeJSON(w, http.StatusOK, c)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"username": in.Username, "permission": in.Permission})
}

// removeCollab 移除协作者。
//
//	@Summary     移除协作者
//	@Tags        repos
//	@Param       owner    path string true "仓库所有者"
//	@Param       name     path string true "仓库名"
//	@Param       username path string true "协作者用户名"
//	@Success     204 {string} string ""
//	@Failure     404 {object} map[string]string
//	@Failure     500 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/collabs/{username} [delete]
func (a *API) removeCollab(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireOwner(w, r)
	if !ok {
		return
	}
	username := strings.ToLower(strings.TrimSpace(r.PathValue("username")))
	err := a.store.RemoveCollab(owner, name, username)
	if errors.Is(err, store.ErrNotFound) {
		writeCode(w, http.StatusNotFound, "collab_not_found", "collaborator not found")
		return
	}
	if err != nil {
		internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
