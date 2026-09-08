package api

import (
	"errors"
	"gitdash/backend/internal/gitsvc"
	"gitdash/backend/internal/pipeline"
	"gitdash/backend/internal/store"
	"net/http"
	"strings"
)

func (a *API) attachStars(repos []store.Repo, me string) {
	if len(repos) == 0 {
		return
	}
	pairs := make([][2]string, 0, len(repos))
	for _, r := range repos {
		pairs = append(pairs, [2]string{r.Owner, r.Name})
	}
	counts := a.store.StarCounts(pairs)
	watchCounts := a.store.WatchCounts(pairs)
	starredSet := a.store.StarredSet(me)
	watchingSet := a.store.WatchingSet(me)
	for i := range repos {
		pair := [2]string{repos[i].Owner, repos[i].Name}
		repos[i].Stars = counts[pair]
		repos[i].Starred = starredSet[pair]
		repos[i].Watchers = watchCounts[pair]
		repos[i].Watching = watchingSet[pair]
	}
}

// listRepos 列出当前用户可访问的仓库。
//
//	@Summary     列出可访问仓库
//	@Description 返回当前用户可访问的全部仓库（含 star/watch 状态）。
//	@Tags        repos
//	@Produce     json
//	@Param       limit  query int false "每页数量（默认 200，最大 500）"
//	@Param       offset query int false "偏移量"
//	@Success     200 {array} store.Repo
//	@SuccessHeader X-Total-Count int "可访问仓库总数"
//	@Security    BearerAuth
//	@Router      /repos [get]
func (a *API) listRepos(w http.ResponseWriter, r *http.Request) {
	me := userFrom(r)
	limit, offset := pageParams(r)
	repos, err := a.store.AccessibleRepos(me, limit, offset)
	if err != nil {
		internalError(w, err)
		return
	}
	total, err := a.store.CountAccessibleRepos(me)
	if err != nil {
		internalError(w, err)
		return
	}
	setTotal(w, total)
	a.attachStars(repos, me)
	writeJSON(w, http.StatusOK, repos)
}

// createRepo 创建仓库。
//
//	@Summary     创建仓库
//	@Description 创建成功返回 201。namespace 可选，指定组织名可将仓库建到组织下。
//	@Tags        repos
//	@Accept      json
//	@Produce     json
//	@Param       body body createRepoReq true "仓库名、描述、模板（readme）、是否私有、组织命名空间"
//	@Success     201 {object} store.Repo
//	@Failure     400 {object} map[string]string
//	@Failure     403 {object} map[string]string
//	@Failure     409 {object} map[string]string
//	@Failure     500 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /repos [post]
func (a *API) createRepo(w http.ResponseWriter, r *http.Request) {
	var in createRepoReq
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	owner := userFrom(r)
	if ns := strings.TrimSpace(in.Namespace); ns != "" && ns != owner {
		if !a.store.IsOrg(ns) || a.store.OrgRole(ns, owner) == "" {
			writeCode(w, http.StatusForbidden, "org_forbidden", "you are not a member of this organization")
			return
		}
		owner = ns
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Template = strings.ToLower(strings.TrimSpace(in.Template))
	in.TemplateOwner = strings.TrimSpace(in.TemplateOwner)
	in.TemplateName = strings.TrimSpace(in.TemplateName)
	if !gitsvc.ValidName(in.Name) {
		writeCode(w, http.StatusBadRequest, "repo_name_invalid", "invalid name: use letters, digits, '.', '_' or '-' (must start alphanumeric)")
		return
	}
	if in.Template != "" && in.Template != "readme" {
		writeCode(w, http.StatusBadRequest, "invalid_template", "template must be empty or 'readme'")
		return
	}
	// 从模版仓库创建：只能二选一，且源仓库必须是模版仓库。
	if (in.TemplateOwner == "") != (in.TemplateName == "") {
		writeCode(w, http.StatusBadRequest, "invalid_template_repo", "template_owner and template_name must be provided together")
		return
	}
	if in.TemplateOwner != "" && in.Template != "" {
		writeCode(w, http.StatusBadRequest, "invalid_template", "template repo and default template cannot be combined")
		return
	}
	var srcRepo store.Repo
	var srcErr error
	if in.TemplateOwner != "" {
		if !gitsvc.ValidName(in.TemplateOwner) || !gitsvc.ValidName(in.TemplateName) {
			writeCode(w, http.StatusBadRequest, "invalid_template_repo", "invalid template repo")
			return
		}
		if in.TemplateOwner == owner && in.TemplateName == in.Name {
			writeCode(w, http.StatusBadRequest, "invalid_template_repo", "cannot use the new repo as its own template")
			return
		}
		srcRepo, srcErr = a.store.GetRepo(in.TemplateOwner, in.TemplateName)
		if srcErr != nil {
			writeCode(w, http.StatusNotFound, "template_repo_not_found", "template repo not found")
			return
		}
		if !srcRepo.IsTemplate {
			writeCode(w, http.StatusBadRequest, "not_template_repo", "source repo is not a template repository")
			return
		}
		if !a.store.CanRead(in.TemplateOwner, in.TemplateName, userFrom(r)) {
			writeCode(w, http.StatusForbidden, "forbidden", "no access to template repo")
			return
		}
		if !gitsvc.Exists(in.TemplateOwner, in.TemplateName) {
			writeCode(w, http.StatusNotFound, "template_repo_not_found", "template repo not on disk")
			return
		}
	}
	if _, err := a.store.GetRepo(owner, in.Name); err == nil || gitsvc.Exists(owner, in.Name) {
		writeCode(w, http.StatusConflict, "repo_exists", "repo already exists")
		return
	}
	private := true
	if in.Private != nil {
		private = *in.Private
	}
	repo, err := a.store.CreateRepo(owner, in.Name, strings.TrimSpace(in.Description), private)
	if err != nil {
		internalError(w, err)
		return
	}
	// 创建者自动 watch 自己的仓库（收件箱订阅）
	_ = a.store.WatchRepo(userFrom(r), owner, in.Name)
	repo.Watchers = 1
	repo.Watching = true
	if srcRepo.ID != 0 {
		// 从模版仓库克隆内容（保留分支/标签并安装 hooks）
		if err := gitsvc.ForkRepo(in.TemplateOwner, in.TemplateName, owner, in.Name); err != nil {
			_ = a.store.DeleteRepo(owner, in.Name)
			_ = gitsvc.Delete(owner, in.Name)
			internalError(w, err)
			return
		}
	} else if err := gitsvc.CreateBare(owner, in.Name); err != nil {
		_ = a.store.DeleteRepo(owner, in.Name)
		internalError(w, err)
		return
	}
	if in.Template == "readme" {
		if err := gitsvc.InitReadme(owner, in.Name); err != nil {
			_ = a.store.DeleteRepo(owner, in.Name)
			_ = gitsvc.Delete(owner, in.Name)
			internalError(w, err)
			return
		}
	}
	writeJSON(w, http.StatusCreated, repo)
}

// getRepo 获取仓库详情。
//
//	@Summary     获取仓库
//	@Description 返回仓库信息（含 fork/导入来源、star/watch 状态与当前用户角色）。
//	@Tags        repos
//	@Produce     json
//	@Param       owner path string false "仓库所有者（简写路由时省略）"
//	@Param       name  path string true  "仓库名"
//	@Success     200 {object} store.Repo
//	@Failure     404 {object} map[string]string
//	@Failure     500 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /repos/{name} [get]
//	@Router      /users/{owner}/repos/{name} [get]
func (a *API) getRepo(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, false)
	if !ok {
		return
	}
	repo, err := a.store.GetRepo(owner, name)
	if err != nil {
		internalError(w, err)
		return
	}
	me := userFrom(r)
	list := []store.Repo{repo}
	a.attachStars(list, me)
	repo = list[0]
	if fo, fr, err := a.store.ForkSource(owner, name); err == nil {
		repo.ForkOwner, repo.ForkRepo = fo, fr
	}
	if iu, err := a.store.ImportSource(owner, name); err == nil {
		repo.ImportURL = iu
	}
	if is, ie, err := a.store.ImportStatus(owner, name); err == nil {
		repo.ImportStatus = is
		repo.ImportError = ie
	}
	// 当前用户视角的角色（owner/write/read），前端据此控制设置类 UI
	switch {
	case a.store.IsRepoOwner(owner, me):
		repo.Role = "owner"
	case a.store.CanWrite(owner, name, me):
		repo.Role = "write"
	default:
		repo.Role = "read"
	}
	writeJSON(w, http.StatusOK, repo)
}

func (a *API) deleteRepo(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, false)
	if !ok {
		return
	}
	if !a.store.IsRepoOwner(owner, userFrom(r)) { // 仅仓库所有者可删除
		writeNotFound(w, "repo")
		return
	}
	if err := a.store.DeleteRepo(owner, name); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeNotFound(w, "repo")
		} else {
			internalError(w, err)
		}
		return
	}
	_ = gitsvc.Delete(owner, name)
	_ = pipeline.DeleteLogs(owner, name)
	w.WriteHeader(http.StatusNoContent)
}
