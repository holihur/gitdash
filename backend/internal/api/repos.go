package api

import (
	"errors"
	"fmt"
	"gitdash/backend/internal/gitsvc"
	"gitdash/backend/internal/gpgsig"
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

func (a *API) listRepos(w http.ResponseWriter, r *http.Request) {
	me := userFrom(r)
	repos, err := a.store.AccessibleRepos(me)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.attachStars(repos, me)
	writeJSON(w, http.StatusOK, repos)
}

func (a *API) createRepo(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Template    string `json:"template"`  // "" = 空仓库；"readme" = 默认模版（README.md）
		Private     *bool  `json:"private"`   // 默认 true（私有）
		Namespace   string `json:"namespace"` // 可选：组织名（成员可把仓库建到组织下）
	}
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

func (a *API) forkRepo(w http.ResponseWriter, r *http.Request) {
	srcOwner, srcName, ok := a.requireAccess(w, r, false)
	if !ok {
		return
	}
	var in struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	}
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

func (a *API) importRepo(w http.ResponseWriter, r *http.Request) {
	var in struct {
		URL        string `json:"url"`
		Name       string `json:"name"`
		Namespace  string `json:"namespace"`
		Private    *bool  `json:"private"`
		PrivateKey string `json:"private_key"`
	}
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
	if err := gitsvc.ImportRepo(raw, targetOwner, targetName, in.PrivateKey); err != nil {
		_ = a.store.DeleteRepo(targetOwner, targetName)
		writeCode(w, http.StatusBadRequest, "import_failed", err.Error())
		return
	}
	if err := a.store.SetImportSource(targetOwner, targetName, raw); err != nil {
		_ = a.store.DeleteRepo(targetOwner, targetName)
		_ = gitsvc.Delete(targetOwner, targetName)
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, repo)
}

// ---- push mirror ----

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
		"created_at": m.CreatedAt,
	})
}

func (a *API) setMirror(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireOwner(w, r)
	if !ok {
		return
	}
	var in struct {
		URL        string `json:"url"`
		PrivateKey string `json:"private_key"`
	}
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
	m, err := a.store.GetMirror(owner, name)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"url": m.URL, "created_at": m.CreatedAt})
}

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
	if err := gitsvc.PushMirror(owner, name, m.URL, m.PrivateKey); err != nil {
		writeCode(w, http.StatusBadGateway, "mirror_sync_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
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
			writeErr(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	_ = gitsvc.Delete(owner, name)
	_ = pipeline.DeleteLogs(owner, name)
	w.WriteHeader(http.StatusNoContent)
}

// ---- git browsing ----

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

func (a *API) createRef(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, true)
	if !ok {
		return
	}
	var in struct {
		Type string `json:"type"`
		Name string `json:"name"`
		From string `json:"from"`
	}
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

func (a *API) deleteRef(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, true)
	if !ok {
		return
	}
	kind := r.PathValue("kind")
	refName := r.PathValue("refname")
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

func (a *API) writeCommit(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, true)
	if !ok {
		return
	}
	var in struct {
		Branch  string              `json:"branch"`
		Message string              `json:"message"`
		Changes []gitsvc.FileChange `json:"changes"`
	}
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

func (a *API) setRepoVisibility(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireOwner(w, r)
	if !ok {
		return
	}
	var in struct {
		Private *bool `json:"private"`
	}
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

func (a *API) exploreRepos(w http.ResponseWriter, r *http.Request) {
	repos, err := a.store.ExploreRepos()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	// 返回所有公开仓库（含自己的，便于确认可见性设置是否生效）
	me := userFrom(r)
	out := []store.Repo{}
	for _, rp := range repos {
		out = append(out, rp)
	}
	a.attachStars(out, me)
	writeJSON(w, http.StatusOK, out)
}

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

func (a *API) addCollab(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireOwner(w, r)
	if !ok {
		return
	}
	var in struct {
		Username   string `json:"username"`
		Permission string `json:"permission"`
	}
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
