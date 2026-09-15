package api

// 管理面板的封禁能力：用户 / 仓库 / 组织。

import (
	"errors"
	"net/http"
	"strings"

	"gitdash/backend/internal/logx"
	"gitdash/backend/internal/store"
)

type banReq struct {
	Banned bool `json:"banned"`
}

// notifyBanned 给接收者写一条系统收件箱通知（kind=system）。
// action 区分封禁对象（banned_user / banned_repo / banned_org），
// owner/repo 携带目标标识供前端本地化渲染。仅封禁时调用；unban 不发消息。
func (a *API) notifyBanned(usernames []string, action, owner, repo, actor string) {
	for _, u := range usernames {
		if u == "" {
			continue
		}
		if err := a.store.AddNotification(u, "system", action, owner, repo, 0, "", actor); err != nil {
			logx.Infof("notify banned %q: %v", u, err)
		}
	}
}

// adminListRepos 管理端仓库列表（含封禁状态）。
//
//	@Summary     管理端仓库列表
//	@Tags        admin
//	@Produce     json
//	@Param       q     query string false "owner/name 模糊过滤"
//	@Success     200 {array} store.Repo
//	@Failure     401 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /admin/repos [get]
func (a *API) adminListRepos(w http.ResponseWriter, r *http.Request) {
	limit, offset := pageParams(r)
	repos, total, err := a.store.AdminListRepos(strings.TrimSpace(r.URL.Query().Get("q")), limit, offset)
	if err != nil {
		internalError(w, err)
		return
	}
	setTotal(w, total)
	writeJSON(w, http.StatusOK, repos)
}

// adminListOrgs 管理端组织列表（含封禁状态）。
//
//	@Summary     管理端组织列表
//	@Tags        admin
//	@Produce     json
//	@Param       q     query string false "组织名模糊过滤"
//	@Success     200 {array} store.Org
//	@Failure     401 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /admin/orgs [get]
func (a *API) adminListOrgs(w http.ResponseWriter, r *http.Request) {
	limit, offset := pageParams(r)
	orgs, total, err := a.store.AdminListOrgs(strings.TrimSpace(r.URL.Query().Get("q")), limit, offset)
	if err != nil {
		internalError(w, err)
		return
	}
	setTotal(w, total)
	writeJSON(w, http.StatusOK, orgs)
}

// adminBanUser 封禁 / 解封用户。封禁会向该用户发送站内通知；解封不发。
//
//	@Summary     管理端封禁/解封用户
//	@Tags        admin
//	@Accept      json
//	@Param       username path string true "用户名"
//	@Param       body body banReq true "banned"
//	@Success     204 {object} nil
//	@Failure     403 {object} map[string]string
//	@Failure     404 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /admin/users/{username}/ban [post]
func (a *API) adminBanUser(w http.ResponseWriter, r *http.Request) {
	username := strings.ToLower(strings.TrimSpace(r.PathValue("username")))
	var in banReq
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	if username == store.TemplateUser && !in.Banned {
		writeCode(w, http.StatusForbidden, "template_user_protected", "the system template user cannot be unbanned")
		return
	}
	if err := a.store.SetUserBanned(username, in.Banned); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeNotFound(w, "user")
			return
		}
		internalError(w, err)
		return
	}
	if in.Banned {
		a.notifyBanned([]string{username}, "banned_user", username, "", userFrom(r))
	}
	logx.Infof("admin %q set user %q banned=%v", userFrom(r), username, in.Banned)
	w.WriteHeader(http.StatusNoContent)
}

// adminBanRepo 封禁 / 解封仓库。封禁后仓库整体禁止访问（含 SSH），并通知 owner。
//
//	@Summary     管理端封禁/解封仓库
//	@Tags        admin
//	@Accept      json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Param       body body banReq true "banned"
//	@Success     204 {object} nil
//	@Failure     404 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /admin/repos/{owner}/{name}/ban [post]
func (a *API) adminBanRepo(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	name := r.PathValue("name")
	var in banReq
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	if err := a.store.SetRepoBanned(owner, name, in.Banned); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeNotFound(w, "repo")
			return
		}
		internalError(w, err)
		return
	}
	if in.Banned {
		a.notifyBanned([]string{owner}, "banned_repo", owner, name, userFrom(r))
	}
	logx.Infof("admin %q set repo %s/%s banned=%v", userFrom(r), owner, name, in.Banned)
	w.WriteHeader(http.StatusNoContent)
}

// adminBanOrg 封禁 / 解封组织。封禁后组织隐藏且其下仓库整体禁止访问，通知组织 owner。
//
//	@Summary     管理端封禁/解封组织
//	@Tags        admin
//	@Accept      json
//	@Param       name path string true "组织名"
//	@Param       body body banReq true "banned"
//	@Success     204 {object} nil
//	@Failure     404 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /admin/orgs/{name}/ban [post]
func (a *API) adminBanOrg(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("name")
	var in banReq
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	if err := a.store.SetOrgBanned(org, in.Banned); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeNotFound(w, "org")
			return
		}
		internalError(w, err)
		return
	}
	if in.Banned {
		a.notifyBanned(a.store.OrgOwners(org), "banned_org", org, "", userFrom(r))
	}
	logx.Infof("admin %q set org %s banned=%v", userFrom(r), org, in.Banned)
	w.WriteHeader(http.StatusNoContent)
}
