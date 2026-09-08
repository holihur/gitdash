package api

import (
	"fmt"
	"gitdash/backend/internal/gitsvc"
	"gitdash/backend/internal/jobs"
	"gitdash/backend/internal/ssrf"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
)

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
		// git:// 明文且无认证：仅允许回环/内网（本地测试 / 内网 Git 服务器）；
		// 公网与 link-local 一律拒绝。
		if !importHostLoopbackOrPrivate(u) {
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

// importHostBlocked 防 SSRF：默认禁止回环/私有/链路本地/云元数据网段，
// 仅允许公网目标；GITDASH_SSRF_ALLOW_PRIVATE=1 可放开私有网段。

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
		if ssrf.IsDangerous(addr) {
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
		internalError(w, err)
		return
	}
	// 导入者自动 watch 导入的仓库
	_ = a.store.WatchRepo(userFrom(r), targetOwner, targetName)
	repo.Watchers = 1
	repo.Watching = true
	if err := a.store.SetImportSource(targetOwner, targetName, raw); err != nil {
		_ = a.store.DeleteRepo(targetOwner, targetName)
		_ = gitsvc.Delete(targetOwner, targetName)
		internalError(w, err)
		return
	}
	// 异步导入：任务队列排队，前端轮询 import_status
	if err := a.store.SetImportStatus(targetOwner, targetName, jobs.StatusQueued, ""); err != nil {
		internalError(w, err)
		return
	}
	if err := jobs.EnqueueImport(targetOwner, targetName, raw, in.PrivateKey); err != nil {
		_ = a.store.SetImportStatus(targetOwner, targetName, jobs.StatusFailed, "enqueue: "+err.Error())
		internalError(w, err)
		return
	}
	repo.ImportStatus = jobs.StatusQueued
	writeJSON(w, http.StatusAccepted, repo)
}
