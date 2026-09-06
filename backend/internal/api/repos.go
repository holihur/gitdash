package api

import (
	"errors"
	"fmt"
	"gitdash/backend/internal/gitsvc"
	"gitdash/backend/internal/gpgsig"
	"gitdash/backend/internal/jobs"
	"gitdash/backend/internal/pipeline"
	"gitdash/backend/internal/store"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
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
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	total, err := a.store.CountAccessibleRepos(me)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
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
	if !gitsvc.ValidName(in.Name) {
		writeCode(w, http.StatusBadRequest, "repo_name_invalid", "invalid name: use letters, digits, '.', '_' or '-' (must start alphanumeric)")
		return
	}
	if in.Template != "" && in.Template != "readme" {
		writeCode(w, http.StatusBadRequest, "invalid_template", "template must be empty or 'readme'")
		return
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
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	// 创建者自动 watch 自己的仓库（收件箱订阅）
	_ = a.store.WatchRepo(userFrom(r), owner, in.Name)
	repo.Watchers = 1
	repo.Watching = true
	if err := gitsvc.CreateBare(owner, in.Name); err != nil {
		_ = a.store.DeleteRepo(owner, in.Name)
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if in.Template == "readme" {
		if err := gitsvc.InitReadme(owner, in.Name); err != nil {
			_ = a.store.DeleteRepo(owner, in.Name)
			_ = gitsvc.Delete(owner, in.Name)
			writeErr(w, http.StatusInternalServerError, err.Error())
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
		writeErr(w, http.StatusInternalServerError, err.Error())
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
	if is, err := a.store.ImportStatus(owner, name); err == nil {
		repo.ImportStatus = is
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
			writeErr(w, http.StatusInternalServerError, err.Error())
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
		writeErr(w, http.StatusInternalServerError, err.Error())
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
		writeErr(w, http.StatusInternalServerError, err.Error())
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
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	// fork 保持源仓库可见性（私有源 -> 私有 fork；公开源 -> 公开 fork）
	repo, err := a.store.CreateRepo(targetOwner, targetName, srcRepo.Description, srcRepo.Private)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := gitsvc.ForkRepo(srcOwner, srcName, targetOwner, targetName); err != nil {
		_ = a.store.DeleteRepo(targetOwner, targetName)
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := a.store.SetForkSource(targetOwner, targetName, srcOwner, srcName); err != nil {
		_ = a.store.DeleteRepo(targetOwner, targetName)
		_ = gitsvc.Delete(targetOwner, targetName)
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	// fork 者自动 watch 自己的 fork（源仓库不一定可见/可 watch）
	_ = a.store.WatchRepo(me, targetOwner, targetName)
	repo.Watchers = 1
	repo.Watching = true
	writeJSON(w, http.StatusCreated, repo)
}

// ---- import ----

// validImportURL 校验导入地址并返回规范化后的 URL（http/https/ssh/scp-like）。

func validImportURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("empty url")
	}
	switch {
	case strings.HasPrefix(raw, "http://"), strings.HasPrefix(raw, "https://"):
		u, err := url.Parse(raw)
		if err != nil || u.Hostname() == "" {
			return "", fmt.Errorf("invalid http url")
		}
		if importHostBlocked(u) {
			return "", fmt.Errorf("blocked host")
		}
		return raw, nil
	case strings.HasPrefix(raw, "ssh://"):
		u, err := url.Parse(raw)
		if err != nil || u.Hostname() == "" {
			return "", fmt.Errorf("invalid ssh url")
		}
		if importHostBlocked(u) {
			return "", fmt.Errorf("blocked host")
		}
		return raw, nil
	case strings.HasPrefix(raw, "git://"):
		u, err := url.Parse(raw)
		if err != nil || u.Hostname() == "" {
			return "", fmt.Errorf("invalid git url")
		}
		// git:// 明文且无认证：仅允许回环/内网（本地测试 / 内网 Git 服务器）
		if importHostBlocked(u) || !importHostLoopbackOrPrivate(u) {
			return "", fmt.Errorf("git:// only allowed for loopback/private hosts")
		}
		return raw, nil
	default:
		// scp-like: git@host:path（无 :// 前缀）
		if strings.Contains(raw, "@") && strings.Contains(raw, ":") {
			return raw, nil
		}
		return "", fmt.Errorf("unsupported url scheme")
	}
}

// importHostBlocked 防 SSRF：阻止 link-local / 云元数据网段。

func importHostBlocked(u *url.URL) bool {
	host := u.Hostname()
	if host == "" {
		return true
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return true
	}
	for _, ip := range ips {
		addr, ok := netip.AddrFromSlice(ip)
		if !ok {
			continue
		}
		addr = addr.Unmap()
		if addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() {
			return true
		}
		s := addr.String()
		if addr.IsPrivate() && strings.HasPrefix(s, "169.254.") {
			return true
		}
		if s == "100.100.100.200" {
			return true
		}
	}
	return false
}

func importHostLoopbackOrPrivate(u *url.URL) bool {
	host := u.Hostname()
	ips, err := net.LookupIP(host)
	if err != nil {
		return false
	}
	for _, ip := range ips {
		addr, ok := netip.AddrFromSlice(ip)
		if ok {
			addr = addr.Unmap()
			if addr.IsLoopback() || addr.IsPrivate() {
				return true
			}
		}
	}
	return false
}

// repoNameFromURL 从仓库 URL 推断默认名称（去 .git 后缀取最后一段）。

func repoNameFromURL(raw string) string {
	if u, err := url.Parse(raw); err == nil && u.Path != "" && u.Path != "/" {
		p := strings.TrimSuffix(strings.TrimSuffix(u.Path, "/"), ".git")
		if i := strings.LastIndex(p, "/"); i >= 0 {
			p = p[i+1:]
		}
		return p
	}
	// scp-like fallback: git@host:owner/repo.git
	s := raw
	if i := strings.LastIndex(s, ":"); i >= 0 {
		s = s[i+1:]
	}
	s = strings.TrimSuffix(strings.TrimSuffix(s, "/"), ".git")
	if i := strings.LastIndex(s, "/"); i >= 0 {
		s = s[i+1:]
	}
	return s
}

// importRepo 从外部 URL 导入仓库。
//
//	@Summary     导入仓库
//	@Description 支持 http(s)/ssh/git 地址；URL 校验失败返回 400。导入异步执行，成功返回 202，通过 GET repo 的 import_status 轮询进度。
//	@Tags        repos
//	@Accept      json
//	@Produce     json
//	@Param       body body importRepoReq true "url、name（可选）、namespace（可选）、private（可选）、private_key（可选）"
//	@Success     202 {object} store.Repo
//	@Failure     400 {object} map[string]string
//	@Failure     403 {object} map[string]string
//	@Failure     409 {object} map[string]string
//	@Failure     500 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /imports [post]
func (a *API) importRepo(w http.ResponseWriter, r *http.Request) {
	var in importRepoReq
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	raw, err := validImportURL(in.URL)
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_url", "URL must be a valid http(s)/ssh/git repository URL")
		return
	}
	me := userFrom(r)
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
		targetName = repoNameFromURL(raw)
	}
	if !gitsvc.ValidName(targetName) {
		writeCode(w, http.StatusBadRequest, "repo_name_invalid", "invalid name: use letters, digits, '.', '_' or '-' (must start alphanumeric)")
		return
	}
	if _, err := a.store.GetRepo(targetOwner, targetName); err == nil || gitsvc.Exists(targetOwner, targetName) {
		writeCode(w, http.StatusConflict, "repo_exists", "repo already exists")
		return
	}
	private := true
	if in.Private != nil {
		private = *in.Private
	}
	repo, err := a.store.CreateRepo(targetOwner, targetName, "", private)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	// 导入者自动 watch 导入的仓库
	_ = a.store.WatchRepo(userFrom(r), targetOwner, targetName)
	repo.Watchers = 1
	repo.Watching = true
	if err := a.store.SetImportSource(targetOwner, targetName, raw); err != nil {
		_ = a.store.DeleteRepo(targetOwner, targetName)
		_ = gitsvc.Delete(targetOwner, targetName)
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	// 异步导入：任务队列排队，前端轮询 import_status
	if err := a.store.SetImportStatus(targetOwner, targetName, jobs.StatusQueued); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := jobs.EnqueueImport(targetOwner, targetName, raw, in.PrivateKey); err != nil {
		_ = a.store.SetImportStatus(targetOwner, targetName, jobs.StatusFailed)
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	repo.ImportStatus = jobs.StatusQueued
	writeJSON(w, http.StatusAccepted, repo)
}

// ---- push mirror ----

// getMirror 查看仓库推送镜像配置。
//
//	@Summary     查看镜像配置
//	@Tags        repos
//	@Produce     json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Success     200 {object} map[string]any "url 与 created_at"
//	@Failure     500 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/mirror [get]
func (a *API) getMirror(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, false)
	if !ok {
		return
	}
	m, err := a.store.GetMirror(owner, name)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"url":        m.URL,
		"status":     m.Status,
		"created_at": m.CreatedAt,
	})
}

// setMirror 设置推送镜像目标。
//
//	@Summary     设置镜像
//	@Tags        repos
//	@Accept      json
//	@Produce     json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Param       body  body setMirrorReq true "url 与 private_key（可选）"
//	@Success     200 {object} map[string]any "url、status 与 created_at"
//	@Failure     400 {object} map[string]string
//	@Failure     500 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/mirror [put]
func (a *API) setMirror(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireOwner(w, r)
	if !ok {
		return
	}
	var in setMirrorReq
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	raw, err := validImportURL(in.URL)
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_url", "URL must be a valid http(s)/ssh/git repository URL")
		return
	}
	if err := a.store.SetMirror(owner, name, raw, strings.TrimSpace(in.PrivateKey)); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = a.store.SetMirrorStatus(owner, name, "") // 换目标后重置状态
	m, err := a.store.GetMirror(owner, name)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"url":        m.URL,
		"status":     m.Status,
		"created_at": m.CreatedAt,
	})
}

// deleteMirror 删除推送镜像配置。
//
//	@Summary     删除镜像
//	@Tags        repos
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Success     204 {string} string ""
//	@Failure     500 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/mirror [delete]
func (a *API) deleteMirror(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireOwner(w, r)
	if !ok {
		return
	}
	if err := a.store.DeleteMirror(owner, name); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// syncMirror 立即触发一次镜像推送（异步队列）。
//
//	@Summary     同步镜像
//	@Tags        repos
//	@Produce     json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Success     202 {object} map[string]any "status=queued"
//	@Failure     400 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/mirror/sync [post]
func (a *API) syncMirror(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireOwner(w, r)
	if !ok {
		return
	}
	m, err := a.store.GetMirror(owner, name)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if m.URL == "" {
		writeCode(w, http.StatusBadRequest, "mirror_not_configured", "no mirror target configured")
		return
	}
	// 异步同步：排队后立即返回，通过 GET mirror 的 status 轮询进度
	if err := a.store.SetMirrorStatus(owner, name, jobs.StatusQueued); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := jobs.EnqueueMirror(owner, name, m.URL, m.PrivateKey); err != nil {
		_ = a.store.SetMirrorStatus(owner, name, jobs.StatusFailed)
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"status": jobs.StatusQueued})
}

// deleteRepo 删除仓库。
//
//	@Summary     删除仓库
//	@Description 仅仓库所有者可删除。
//	@Tags        repos
//	@Param       owner path string false "仓库所有者（简写路由时省略）"
//	@Param       name  path string true  "仓库名"
//	@Success     204 {string} string ""
//	@Failure     404 {object} map[string]string
//	@Failure     500 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /repos/{name} [delete]
//	@Router      /users/{owner}/repos/{name} [delete]
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
			writeErr(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	_ = gitsvc.Delete(owner, name)
	_ = pipeline.DeleteLogs(owner, name)
	w.WriteHeader(http.StatusNoContent)
}

// ---- git browsing ----

// branches 列出仓库分支。
//
//	@Summary     列出分支
//	@Tags        repos
//	@Produce     json
//	@Param       owner path string false "仓库所有者（简写路由时省略）"
//	@Param       name  path string true  "仓库名"
//	@Success     200 {array} gitsvc.Branch
//	@Failure     500 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /repos/{name}/branches [get]
//	@Router      /users/{owner}/repos/{name}/branches [get]
func (a *API) branches(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, false)
	if !ok {
		return
	}
	bs, err := gitsvc.Branches(owner, name)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, bs)
}

// tree 列出目录内容。
//
//	@Summary     浏览目录树
//	@Tags        repos
//	@Produce     json
//	@Param       owner path string false "仓库所有者（简写路由时省略）"
//	@Param       name  path string true  "仓库名"
//	@Param       ref   query string false "分支/标签/commit（默认默认分支）"
//	@Param       path  query string false "目录路径（默认根目录）"
//	@Success     200 {object} map[string]any "path、entries 与 truncated"
//	@Failure     400 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /repos/{name}/tree [get]
//	@Router      /users/{owner}/repos/{name}/tree [get]
func (a *API) tree(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, false)
	if !ok {
		return
	}
	ref := r.URL.Query().Get("ref")
	dir, err := gitsvc.CleanPath(r.URL.Query().Get("path"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	entries, err := gitsvc.Tree(owner, name, ref, dir)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	// 超大目录分页上限，防止每次请求产生过多子进程
	const maxEntries = 1000
	truncated := len(entries) > maxEntries
	if truncated {
		entries = entries[:maxEntries]
	}
	writeJSON(w, http.StatusOK, map[string]any{"path": dir, "entries": entries, "truncated": truncated})
}

// blob 读取文件内容。
//
//	@Summary     读取文件
//	@Tags        repos
//	@Produce     json
//	@Param       owner path string false "仓库所有者（简写路由时省略）"
//	@Param       name  path string true  "仓库名"
//	@Param       ref   query string false "分支/标签/commit"
//	@Param       path  query string true  "文件路径"
//	@Success     200 {object} gitsvc.Blob
//	@Failure     400 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /repos/{name}/blob [get]
//	@Router      /users/{owner}/repos/{name}/blob [get]
func (a *API) blob(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, false)
	if !ok {
		return
	}
	b, err := gitsvc.ReadBlob(owner, name, r.URL.Query().Get("ref"), r.URL.Query().Get("path"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, b)
}

// blame 查看文件逐行归属。
//
//	@Summary     文件 blame
//	@Tags        repos
//	@Produce     json
//	@Param       owner path string false "仓库所有者（简写路由时省略）"
//	@Param       name  path string true  "仓库名"
//	@Param       ref   query string false "分支/标签/commit"
//	@Param       path  query string true  "文件路径"
//	@Success     200 {object} gitsvc.Blame
//	@Failure     400 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /repos/{name}/blame [get]
//	@Router      /users/{owner}/repos/{name}/blame [get]
func (a *API) blame(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, false)
	if !ok {
		return
	}
	b, err := gitsvc.BlameFile(owner, name, r.URL.Query().Get("ref"), r.URL.Query().Get("path"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, b)
}

// commits 列出提交历史。
//
//	@Summary     提交历史
//	@Description 返回提交列表，含 GPG 验证结果（对已注册公钥）。
//	@Tags        repos
//	@Produce     json
//	@Param       owner path string false "仓库所有者（简写路由时省略）"
//	@Param       name  path string true  "仓库名"
//	@Param       ref   query string false "分支/标签/commit"
//	@Param       limit query int    false "返回条数上限"
//	@Success     200 {array} api.commitResp
//	@Failure     400 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /repos/{name}/commits [get]
//	@Router      /users/{owner}/repos/{name}/commits [get]
func (a *API) commits(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, false)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	cs, err := gitsvc.Commits(owner, name, r.URL.Query().Get("ref"), limit)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	// GPG 签名校验（对已注册公钥；失败不影响列表展示）。公钥走 TTL 缓存，
	// commit 原文用单次 cat-file --batch 读取，避免逐条 spawn 进程。
	keys := a.gpgVerifyKeys()
	shas := make([]string, 0, len(cs))
	for _, c := range cs {
		shas = append(shas, c.SHA)
	}
	raws := gitsvc.RawCommits(owner, name, shas)
	out := make([]commitResp, 0, len(cs))
	for _, c := range cs {
		r := commitResp{SHA: c.SHA, Author: c.Author, Date: c.Date, Message: c.Message}
		if raw, ok := raws[c.SHA]; ok {
			if user, _, ok := gpgsig.VerifyCommit(raw, keys); ok && user != "" {
				r.GPGVerified = user
			}
		}
		out = append(out, r)
	}
	writeJSON(w, http.StatusOK, out)
}

type commitResp struct {
	SHA         string `json:"sha"`
	Author      string `json:"author"`
	Date        string `json:"date"`
	Message     string `json:"message"`
	GPGVerified string `json:"gpg_verified,omitempty"`
}

// ---- issues ----

// listTags 列出仓库标签。
//
//	@Summary     列出标签
//	@Tags        repos
//	@Produce     json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Success     200 {array} gitsvc.Tag
//	@Failure     500 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/tags [get]
func (a *API) listTags(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, false)
	if !ok {
		return
	}
	tags, err := gitsvc.Tags(owner, name)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, tags)
}

// createRef 创建分支或标签。
//
//	@Summary     创建分支/标签
//	@Description type 必须为 branch 或 tag；from 缺省为 HEAD。
//	@Tags        repos
//	@Accept      json
//	@Produce     json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Param       body  body createRefReq true "type、name、from（可选）"
//	@Success     201 {object} map[string]any "type、name 与 sha"
//	@Failure     400 {object} map[string]string
//	@Failure     409 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/refs [post]
func (a *API) createRef(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, true)
	if !ok {
		return
	}
	var in createRefReq
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	if in.Type != "branch" && in.Type != "tag" {
		writeCode(w, http.StatusBadRequest, "invalid_ref_type", "type must be 'branch' or 'tag'")
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	in.From = strings.TrimSpace(in.From)
	if in.Name == "" {
		writeCode(w, http.StatusBadRequest, "invalid_ref_name", "ref name is required")
		return
	}
	if in.From == "" {
		in.From = "HEAD"
	}
	sha, err := gitsvc.CreateRef(owner, name, in.Type, in.Name, in.From)
	if errors.Is(err, gitsvc.ErrRefExists) {
		writeCode(w, http.StatusConflict, "ref_exists", in.Type+" already exists: "+in.Name)
		return
	}
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_ref_name", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"type": in.Type, "name": in.Name, "sha": sha})
}

// deleteRef 删除分支或标签。
//
//	@Summary     删除分支/标签
//	@Description 不能删除默认（HEAD）分支。
//	@Tags        repos
//	@Param       owner   path string true "仓库所有者"
//	@Param       name    path string true "仓库名"
//	@Param       kind    path string true "类型：branches 或 tags"
//	@Param       refname path string true "分支/标签名"
//	@Success     204 {string} string ""
//	@Failure     400 {object} map[string]string
//	@Failure     404 {object} map[string]string
//	@Failure     409 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/refs/{kind}/{refname} [delete]
func (a *API) deleteRef(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, true)
	if !ok {
		return
	}
	kind := r.PathValue("kind")
	refName := r.PathValue("refname")
	// 分支保护：被保护的分支禁止通过 web API 删除
	if kind == "branches" {
		if _, prot := a.branchProtectionGuard(owner, name, refName); prot {
			writeCode(w, http.StatusConflict, "branch_protected",
				"branch is protected: deletion is not allowed")
			return
		}
	}
	err := gitsvc.DeleteRef(owner, name, kind, refName)
	if errors.Is(err, gitsvc.ErrHeadBranch) {
		writeCode(w, http.StatusConflict, "branch_is_head", "cannot delete the default (HEAD) branch")
		return
	}
	if errors.Is(err, gitsvc.ErrRefNotFound) {
		writeCode(w, http.StatusNotFound, "ref_not_found", kind+" not found")
		return
	}
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_ref_name", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// writeCommit 写入一次提交。
//
//	@Summary     创建提交
//	@Description 支持批量文件变更（create/update/delete/delete_tree），总内容不超过 2MB。
//	@Tags        repos
//	@Accept      json
//	@Produce     json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Param       body  body writeCommitReq true "branch（默认 main）、message 与 changes"
//	@Success     201 {object} map[string]any "sha、branch 与 message"
//	@Failure     400 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/commits [post]
func (a *API) writeCommit(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, true)
	if !ok {
		return
	}
	var in writeCommitReq
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	in.Branch = strings.TrimSpace(in.Branch)
	in.Message = strings.TrimSpace(in.Message)
	if in.Branch == "" {
		in.Branch = "main"
	}
	if in.Message == "" {
		writeCode(w, http.StatusBadRequest, "message_required", "commit message is required")
		return
	}
	if len(in.Changes) == 0 {
		writeCode(w, http.StatusBadRequest, "no_changes", "no file changes provided")
		return
	}
	if len(in.Changes) > 100 {
		writeCode(w, http.StatusBadRequest, "too_many_changes", "too many changes (max 100)")
		return
	}
	total := 0
	for i := range in.Changes {
		c := &in.Changes[i]
		c.Path = strings.TrimSpace(c.Path)
		c.Path = strings.TrimPrefix(c.Path, "/")
		if _, err := gitsvc.CleanPath(c.Path); err != nil {
			writeCode(w, http.StatusBadRequest, "invalid_path", err.Error())
			return
		}
		if c.Action == "" {
			c.Action = "update"
		}
		switch c.Action {
		case "create", "update", "delete", "delete_tree":
		default:
			writeCode(w, http.StatusBadRequest, "invalid_action", "action must be create/update/delete/delete_tree")
			return
		}
		total += len(c.Content)
	}
	if total > 2<<20 { // 2MB
		writeCode(w, http.StatusBadRequest, "content_too_large", "content too large (max 2MB per commit)")
		return
	}
	sha, err := gitsvc.WriteCommit(owner, name, in.Branch, in.Message, userFrom(r), in.Changes)
	if err != nil {
		writeCode(w, http.StatusBadRequest, "commit_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"sha": sha, "branch": in.Branch, "message": in.Message})
}

// commitDiff 查看提交差异。
//
//	@Summary     提交 diff
//	@Tags        repos
//	@Produce     json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Param       sha   path string true "commit SHA"
//	@Success     200 {object} map[string]any "files 与 patch"
//	@Failure     400 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/commits/{sha}/diff [get]
func (a *API) commitDiff(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, false)
	if !ok {
		return
	}
	sha := r.PathValue("sha")
	if !shaRe.MatchString(sha) {
		writeCode(w, http.StatusBadRequest, "invalid_sha", "invalid commit sha")
		return
	}
	files, patch, err := gitsvc.CommitDiff(owner, name, sha)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"files": files, "patch": patch})
}

// ---- pull requests ----

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
		writeErr(w, http.StatusInternalServerError, err.Error())
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
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	repo, err := a.store.GetRepo(owner, name)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, repo)
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
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	total, err := a.store.CountExploreRepos()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	setTotal(w, total)
	// 返回所有公开仓库（含自己的，便于确认可见性设置是否生效）
	me := userFrom(r)
	out := []store.Repo{}
	for _, rp := range repos {
		out = append(out, rp)
	}
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
		writeErr(w, http.StatusInternalServerError, err.Error())
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
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	collabs, err := a.store.ListCollabs(owner, name)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
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
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- webhooks ----
