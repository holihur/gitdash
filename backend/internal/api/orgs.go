package api

import (
	"errors"
	"gitdash/backend/internal/store"
	"net/http"
	"strings"
)

// createOrg 创建组织。
//
//	@Summary     创建组织
//	@Tags        orgs
//	@Accept      json
//	@Produce     json
//	@Param       body body createOrgReq true "组织名与显示名"
//	@Success     201 {object} map[string]string
//	@Failure     400 {object} map[string]string
//	@Failure     409 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /orgs [post]
func (a *API) createOrg(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name    string `json:"name"`
		Display string `json:"display"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	in.Name = strings.ToLower(strings.TrimSpace(in.Name))
	if !orgNameRe.MatchString(in.Name) {
		writeCode(w, http.StatusBadRequest, "username_invalid", "org name must be 2-32 chars: lowercase letters, digits, '_' or '-', starting alphanumeric")
		return
	}
	o, err := a.store.CreateOrg(in.Name, strings.TrimSpace(in.Display), userFrom(r))
	if errors.Is(err, store.ErrExists) {
		writeCode(w, http.StatusConflict, "org_exists", "organization name already taken")
		return
	}
	if err != nil {
		internalError(w, err)
		return
	}
	// 组织创建时初始化同名公开仓库（<org>/<org>），创建者自动订阅。
	a.provisionSameNameRepo(o.Name, userFrom(r))
	writeJSON(w, http.StatusCreated, o)
}

// listOrgs 列出当前用户的组织。
//
//	@Summary     列出我的组织
//	@Tags        orgs
//	@Produce     json
//	@Success     200 {array}  object
//	@Security    BearerAuth
//	@Router      /orgs [get]
func (a *API) listOrgs(w http.ResponseWriter, r *http.Request) {
	me := userFrom(r)
	limit, offset := pageParams(r)
	orgs, err := a.store.ListMyOrgs(me, limit, offset)
	if err != nil {
		internalError(w, err)
		return
	}
	total, err := a.store.CountMyOrgs(me)
	if err != nil {
		internalError(w, err)
		return
	}
	setTotal(w, total)
	out := []map[string]any{}
	for _, o := range orgs {
		out = append(out, map[string]any{
			"name": o.Name, "display": o.Display, "created_at": o.CreatedAt,
			"role": a.store.OrgRole(o.Name, me),
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// getOrg 获取组织及当前用户角色。
//
//	@Summary     获取组织
//	@Tags        orgs
//	@Produce     json
//	@Param       org path string true "组织名"
//	@Success     200 {object} map[string]string
//	@Failure     404 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /orgs/{org} [get]
func (a *API) getOrg(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	role := a.store.OrgRole(org, userFrom(r))
	if role == "" {
		writeCode(w, http.StatusNotFound, "org_not_found", "organization not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"name": org, "role": role})
}

// deleteOrg 删除组织（仅 owner；组织内仍有仓库时返回 409）。
//
//	@Summary     删除组织
//	@Tags        orgs
//	@Produce     json
//	@Param       org path string true "组织名"
//	@Success     204 {object} nil
//	@Failure     404 {object} map[string]string
//	@Failure     409 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /orgs/{org} [delete]
func (a *API) deleteOrg(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	if a.store.OrgRole(org, userFrom(r)) != "owner" {
		writeCode(w, http.StatusNotFound, "org_not_found", "organization not found")
		return
	}
	if err := a.store.DeleteOrg(org); err != nil {
		if strings.Contains(err.Error(), "not empty") {
			writeCode(w, http.StatusConflict, "org_not_empty", "delete or move all repositories before deleting the organization")
			return
		}
		internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// listOrgMembers 列出组织成员。
//
//	@Summary     列出组织成员
//	@Tags        orgs
//	@Produce     json
//	@Param       org path string true "组织名"
//	@Success     200 {array}  object
//	@Failure     404 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /orgs/{org}/members [get]
func (a *API) listOrgMembers(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	if a.store.OrgRole(org, userFrom(r)) == "" {
		writeCode(w, http.StatusNotFound, "org_not_found", "organization not found")
		return
	}
	limit, offset := pageParams(r)
	members, err := a.store.ListOrgMembers(org, limit, offset)
	if err != nil {
		internalError(w, err)
		return
	}
	total, err := a.store.CountOrgMembers(org)
	if err != nil {
		internalError(w, err)
		return
	}
	setTotal(w, total)
	writeJSON(w, http.StatusOK, members)
}

// addOrgMember 添加组织成员（仅 owner）。
//
//	@Summary     添加组织成员
//	@Tags        orgs
//	@Accept      json
//	@Produce     json
//	@Param       org  path string true "组织名"
//	@Param       body body addOrgMemberReq true "用户名与角色（member/owner）"
//	@Success     200 {object} map[string]string
//	@Failure     400 {object} map[string]string
//	@Failure     404 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /orgs/{org}/members [post]
func (a *API) addOrgMember(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	if a.store.OrgRole(org, userFrom(r)) != "owner" {
		writeCode(w, http.StatusNotFound, "org_not_found", "organization not found")
		return
	}
	var in struct {
		Username string `json:"username"`
		Role     string `json:"role"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	in.Username = strings.ToLower(strings.TrimSpace(in.Username))
	if in.Role == "" {
		in.Role = "member"
	}
	if in.Role != "member" && in.Role != "owner" {
		writeCode(w, http.StatusBadRequest, "invalid_permission", "role must be member or owner")
		return
	}
	if _, err := a.store.GetByUsername(in.Username); err != nil {
		writeCode(w, http.StatusNotFound, "user_not_found", "user not found")
		return
	}
	if err := a.store.AddOrgMember(org, in.Username, in.Role); err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"org": org, "username": in.Username, "role": in.Role})
}

// removeOrgMember 移除组织成员（仅 owner；不能移除自己，须保留至少一个 owner）。
//
//	@Summary     移除组织成员
//	@Tags        orgs
//	@Produce     json
//	@Param       org      path string true "组织名"
//	@Param       username path string true "成员用户名"
//	@Success     204 {object} nil
//	@Failure     400 {object} map[string]string
//	@Failure     404 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /orgs/{org}/members/{username} [delete]
func (a *API) removeOrgMember(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	me := userFrom(r)
	if a.store.OrgRole(org, me) != "owner" {
		writeCode(w, http.StatusNotFound, "org_not_found", "organization not found")
		return
	}
	target := strings.ToLower(strings.TrimSpace(r.PathValue("username")))
	if target == me {
		writeCode(w, http.StatusBadRequest, "cannot_remove_self", "transfer ownership or delete the organization instead")
		return
	}
	if a.store.OrgRole(org, target) == "owner" {
		// 仅剩一个 owner 时不允许移除 owner
		members, _ := a.store.OrgMembers(org)
		owners := 0
		for _, m := range members {
			if m.Role == "owner" {
				owners++
			}
		}
		if owners <= 1 {
			writeCode(w, http.StatusBadRequest, "last_owner", "organization must keep at least one owner")
			return
		}
	}
	if errors.Is(a.store.RemoveOrgMember(org, target), store.ErrNotFound) {
		writeCode(w, http.StatusNotFound, "member_not_found", "member not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// getOrgProfile 组织主页（公开）：资料、关注统计、成员与可见仓库。
//
//	@Summary     组织主页
//	@Description 返回组织公开资料、粉丝数、当前用户是否已关注、成员列表，以及可见仓库（非成员仅公开仓库）。
//	@Tags        orgs
//	@Produce     json
//	@Param       org path string true "组织名"
//	@Success     200 {object} map[string]any
//	@Failure     404 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /orgs/{org}/profile [get]
func (a *API) getOrgProfile(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	me := userFrom(r)
	o, err := a.store.GetOrg(org)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeCode(w, http.StatusNotFound, "org_not_found", "organization not found")
			return
		}
		internalError(w, err)
		return
	}
	if o.Banned {
		writeCode(w, http.StatusNotFound, "org_not_found", "organization not found")
		return
	}
	role := a.store.OrgRole(org, me)
	members, err := a.store.OrgMembers(org)
	if err != nil {
		internalError(w, err)
		return
	}
	repos, err := a.store.ListRepos(org)
	if err != nil {
		internalError(w, err)
		return
	}
	if role == "" { // 非成员仅可见公开仓库
		public := make([]store.Repo, 0, len(repos))
		for _, repo := range repos {
			if !repo.Private {
				public = append(public, repo)
			}
		}
		repos = public
	}
	a.attachStars(repos, me)
	a.attachTopics(repos)
	followers, err := a.store.OrgFollowerCount(org)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"name":         o.Name,
		"display":      o.Display,
		"created_at":   o.CreatedAt,
		"role":         role,
		"members":      members,
		"repos":        repos,
		"followers":    followers,
		"is_following": a.store.IsFollowingOrg(me, org),
	})
}

// writeOrgFollowState 返回组织关注状态与粉丝数。
func (a *API) writeOrgFollowState(w http.ResponseWriter, org, me string) {
	followers, err := a.store.OrgFollowerCount(org)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"followers":    followers,
		"is_following": a.store.IsFollowingOrg(me, org),
	})
}

// followOrg 关注组织。
//
//	@Summary     关注组织
//	@Description 幂等。返回关注状态与粉丝数。
//	@Tags        orgs
//	@Produce     json
//	@Param       org path string true "组织名"
//	@Success     200 {object} map[string]any
//	@Failure     404 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /orgs/{org}/follow [post]
func (a *API) followOrg(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	me := userFrom(r)
	o, err := a.store.GetOrg(org)
	if err != nil || o.Banned {
		writeCode(w, http.StatusNotFound, "org_not_found", "organization not found")
		return
	}
	if err := a.store.FollowOrg(me, org); err != nil && !errors.Is(err, store.ErrExists) {
		internalError(w, err)
		return
	}
	a.writeOrgFollowState(w, org, me)
}

// unfollowOrg 取消关注组织。
//
//	@Summary     取消关注组织
//	@Description 幂等。返回关注状态与粉丝数。
//	@Tags        orgs
//	@Produce     json
//	@Param       org path string true "组织名"
//	@Success     200 {object} map[string]any
//	@Failure     404 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /orgs/{org}/follow [delete]
func (a *API) unfollowOrg(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	me := userFrom(r)
	if _, err := a.store.GetOrg(org); err != nil {
		writeCode(w, http.StatusNotFound, "org_not_found", "organization not found")
		return
	}
	if err := a.store.UnfollowOrg(me, org); err != nil {
		internalError(w, err)
		return
	}
	a.writeOrgFollowState(w, org, me)
}

// listOrgFollowers 关注某组织的用户列表。
//
//	@Summary     组织粉丝列表
//	@Tags        orgs
//	@Produce     json
//	@Param       org path string true "组织名"
//	@Success     200 {array} store.UserSummary
//	@Failure     404 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /orgs/{org}/followers [get]
func (a *API) listOrgFollowers(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	if _, err := a.store.GetOrg(org); err != nil {
		writeCode(w, http.StatusNotFound, "org_not_found", "organization not found")
		return
	}
	limit, offset := pageParams(r)
	users, err := a.store.ListOrgFollowers(org, limit, offset)
	if err != nil {
		internalError(w, err)
		return
	}
	total, err := a.store.OrgFollowerCount(org)
	if err != nil {
		internalError(w, err)
		return
	}
	setTotal(w, int(total))
	writeJSON(w, http.StatusOK, users)
}
