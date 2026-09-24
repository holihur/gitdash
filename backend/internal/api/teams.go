package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"gitdash/backend/internal/store"
)

// 组织团队：owner 建团队 / 加成员；仓库 admin 把团队授权到仓库（批量授予角色）。

func (a *API) requireOrgOwner(w http.ResponseWriter, r *http.Request, org string) bool {
	if a.store.OrgRole(org, userFrom(r)) != "owner" {
		writeCode(w, http.StatusNotFound, "org_not_found", "organization not found")
		return false
	}
	return true
}

// listOrgTeams 列出组织团队。
//
//	@Summary     列出组织团队
//	@Tags        orgs
//	@Produce     json
//	@Param       org path string true "组织名"
//	@Success     200 {array} store.OrgTeam
//	@Security    BearerAuth
//	@Router      /orgs/{org}/teams [get]
func (a *API) listOrgTeams(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	if a.store.OrgRole(org, userFrom(r)) == "" {
		writeCode(w, http.StatusNotFound, "org_not_found", "organization not found")
		return
	}
	teams, err := a.store.ListOrgTeams(org)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, teams)
}

// createOrgTeam 创建团队（仅 owner）。
//
//	@Summary     创建组织团队
//	@Tags        orgs
//	@Accept      json
//	@Produce     json
//	@Param       org  path string true "组织名"
//	@Param       body body map[string]string true "name"
//	@Success     201 {object} store.OrgTeam
//	@Security    BearerAuth
//	@Router      /orgs/{org}/teams [post]
func (a *API) createOrgTeam(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	if !a.requireOrgOwner(w, r, org) {
		return
	}
	var in struct {
		Name string `json:"name"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || tooLong(w, "name", in.Name, maxNameRunes) {
		if in.Name == "" {
			writeCode(w, http.StatusBadRequest, "name_required", "team name is required")
		}
		return
	}
	t, err := a.store.CreateOrgTeam(org, in.Name)
	if errors.Is(err, store.ErrExists) {
		writeCode(w, http.StatusConflict, "team_exists", "team name already exists")
		return
	}
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, t)
}

// deleteOrgTeam 删除团队（仅 owner）。
//
//	@Summary     删除组织团队
//	@Tags        orgs
//	@Param       org path string true "组织名"
//	@Param       id  path int    true "团队 ID"
//	@Success     204 {object} nil
//	@Security    BearerAuth
//	@Router      /orgs/{org}/teams/{id} [delete]
func (a *API) deleteOrgTeam(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	if !a.requireOrgOwner(w, r, org) {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeCode(w, http.StatusBadRequest, "invalid_id", "invalid id")
		return
	}
	if err := a.store.DeleteOrgTeam(org, id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeNotFound(w, "team")
			return
		}
		internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// listOrgTeamMembers 团队及其成员。
//
//	@Summary     团队详情
//	@Tags        orgs
//	@Produce     json
//	@Param       org path string true "组织名"
//	@Param       id  path int    true "团队 ID"
//	@Success     200 {object} map[string]interface{}
//	@Security    BearerAuth
//	@Router      /orgs/{org}/teams/{id}/members [get]
func (a *API) listOrgTeamMembers(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	if a.store.OrgRole(org, userFrom(r)) == "" {
		writeCode(w, http.StatusNotFound, "org_not_found", "organization not found")
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeCode(w, http.StatusBadRequest, "invalid_id", "invalid id")
		return
	}
	members, err := a.store.OrgTeamMembers(id)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"members": members})
}

// addOrgTeamMember 添加团队成员（仅 owner）。
//
//	@Summary     添加团队成员
//	@Tags        orgs
//	@Accept      json
//	@Param       org  path string true "组织名"
//	@Param       id   path int    true "团队 ID"
//	@Param       body body map[string]string true "username"
//	@Success     204 {object} nil
//	@Security    BearerAuth
//	@Router      /orgs/{org}/teams/{id}/members [post]
func (a *API) addOrgTeamMember(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	if !a.requireOrgOwner(w, r, org) {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeCode(w, http.StatusBadRequest, "invalid_id", "invalid id")
		return
	}
	var in struct {
		Username string `json:"username"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	in.Username = strings.ToLower(strings.TrimSpace(in.Username))
	if in.Username == "" {
		writeCode(w, http.StatusBadRequest, "username_required", "username is required")
		return
	}
	if err := a.store.AddOrgTeamMember(org, id, in.Username); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeCode(w, http.StatusNotFound, "not_found", "team or user not found")
			return
		}
		internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// removeOrgTeamMember 移除团队成员（仅 owner）。
//
//	@Summary     移除团队成员
//	@Tags        orgs
//	@Param       org      path string true "组织名"
//	@Param       id       path int    true "团队 ID"
//	@Param       username path string true "用户名"
//	@Success     204 {object} nil
//	@Security    BearerAuth
//	@Router      /orgs/{org}/teams/{id}/members/{username} [delete]
func (a *API) removeOrgTeamMember(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	if !a.requireOrgOwner(w, r, org) {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeCode(w, http.StatusBadRequest, "invalid_id", "invalid id")
		return
	}
	username := strings.ToLower(strings.TrimSpace(r.PathValue("username")))
	if err := a.store.RemoveOrgTeamMember(org, id, username); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeNotFound(w, "member")
			return
		}
		internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- repo team grants（admin）----

// listRepoTeamGrants 仓库上的团队授权。
//
//	@Summary     仓库团队授权列表
//	@Tags        repos
//	@Produce     json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Success     200 {array} store.RepoTeamGrant
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/team-grants [get]
func (a *API) listRepoTeamGrants(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireRole(w, r, store.RoleAdmin)
	if !ok {
		return
	}
	grants, err := a.store.RepoTeamGrants(owner, name)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, grants)
}

// grantRepoTeam 把团队授权到仓库（需 admin）。
//
//	@Summary     授予团队仓库权限
//	@Tags        repos
//	@Accept      json
//	@Param       owner  path string true "仓库所有者"
//	@Param       name   path string true "仓库名"
//	@Param       teamId path int    true "团队 ID"
//	@Param       body   body map[string]string true "permission"
//	@Success     204 {object} nil
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/team-grants/{teamId} [put]
func (a *API) grantRepoTeam(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireRole(w, r, store.RoleAdmin)
	if !ok {
		return
	}
	teamID, err := strconv.ParseInt(r.PathValue("teamId"), 10, 64)
	if err != nil || teamID < 1 {
		writeCode(w, http.StatusBadRequest, "invalid_id", "invalid id")
		return
	}
	var in struct {
		Permission string `json:"permission"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	if !store.ValidCollabRole(strings.TrimSpace(in.Permission)) {
		writeCode(w, http.StatusBadRequest, "invalid_permission", "permission must be one of read/triage/write/maintain/admin")
		return
	}
	if err := a.store.GrantRepoTeam(owner, name, teamID, strings.TrimSpace(in.Permission)); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeCode(w, http.StatusNotFound, "not_found", "team or repository not found")
			return
		}
		internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// revokeRepoTeam 撤销团队仓库授权（需 admin）。
//
//	@Summary     撤销团队仓库权限
//	@Tags        repos
//	@Param       owner  path string true "仓库所有者"
//	@Param       name   path string true "仓库名"
//	@Param       teamId path int    true "团队 ID"
//	@Success     204 {object} nil
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/team-grants/{teamId} [delete]
func (a *API) revokeRepoTeam(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireRole(w, r, store.RoleAdmin)
	if !ok {
		return
	}
	teamID, err := strconv.ParseInt(r.PathValue("teamId"), 10, 64)
	if err != nil || teamID < 1 {
		writeCode(w, http.StatusBadRequest, "invalid_id", "invalid id")
		return
	}
	if err := a.store.RevokeRepoTeam(owner, name, teamID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeNotFound(w, "grant")
			return
		}
		internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// getRepoAccess 权限审计：谁通过什么途径获得什么角色（需 admin）。
//
//	@Summary     仓库权限审计
//	@Tags        repos
//	@Produce     json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Success     200 {array} store.AccessEntry
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/access [get]
func (a *API) getRepoAccess(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireRole(w, r, store.RoleAdmin)
	if !ok {
		return
	}
	entries, err := a.store.AccessEntries(owner, name)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, entries)
}
