package api

import (
	"errors"
	"mime"
	"net/http"
	"path"
	"strings"

	"gitdash/backend/internal/gitsvc"
	"gitdash/backend/internal/store"
)

// 仓库静态网站托管（Pages，类 GitHub Pages）。
//
// 默认关闭；由仓库 owner 在设置页开启并指定分支（默认分支）与目录（仓库根）。
// 站点通过 /pages/{owner}/{repo}/... 提供服务。为避免 UGC 的 HTML/JS 与主应用
// 同源而窃取会话/发起带凭据请求，统一用 CSP `sandbox`（无 allow-same-origin）
// 隔离为不透明源：脚本仍可运行，但拿不到主站 cookie/localStorage，跨源 fetch
// 也不再携带凭据。
const pagesSandboxCSP = "sandbox allow-scripts allow-forms allow-popups allow-modals allow-downloads allow-top-navigation-by-user-activation"

type repoPagesResp struct {
	Enabled bool   `json:"enabled"`
	Branch  string `json:"branch"`
	Dir     string `json:"dir"`
	URL     string `json:"url"`
}

func pagesConfig(rp store.Repo, base string) repoPagesResp {
	return repoPagesResp{
		Enabled: rp.PagesEnabled,
		Branch:  rp.PagesBranch,
		Dir:     rp.PagesDir,
		URL:     base + "/pages/" + rp.Owner + "/" + rp.Name + "/",
	}
}

// getRepoPages 读取仓库 Pages 配置。
//
//	@Summary     读取仓库 Pages 配置
//	@Tags        repos
//	@Produce     json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Success     200 {object} object "enabled/branch/dir/url"
//	@Failure     404 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/pages [get]
func (a *API) getRepoPages(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, false)
	if !ok {
		return
	}
	repo, err := a.store.GetRepo(owner, name)
	if err != nil {
		writeNotFound(w, "repo")
		return
	}
	writeJSON(w, http.StatusOK, pagesConfig(repo, reqBase(r)))
}

// setRepoPages 更新仓库 Pages 配置（仅 owner）。
//
//	@Summary     更新仓库 Pages 配置
//	@Tags        repos
//	@Accept      json
//	@Produce     json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Param       body  body object true "enabled/branch/dir"
//	@Success     200 {object} object "enabled/branch/dir/url"
//	@Failure     400 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/pages [put]
func (a *API) setRepoPages(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireRole(w, r, "maintain")
	if !ok {
		return
	}
	var in struct {
		Enabled bool   `json:"enabled"`
		Branch  string `json:"branch"`
		Dir     string `json:"dir"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	in.Branch = strings.TrimSpace(in.Branch)
	in.Dir = strings.TrimSpace(in.Dir)
	if in.Enabled {
		if tooLong(w, "branch", in.Branch, 255) {
			return
		}
		if in.Branch != "" {
			if _, err := gitsvc.RevSHA(owner, name, "refs/heads/"+in.Branch); err != nil {
				writeCode(w, http.StatusBadRequest, "branch_not_found", "branch not found: "+in.Branch)
				return
			}
		}
		if in.Dir != "" {
			d, err := gitsvc.CleanPath(in.Dir)
			if err != nil || d == "" {
				writeCode(w, http.StatusBadRequest, "invalid_dir", "invalid source directory")
				return
			}
			in.Dir = d
		}
	}
	if err := a.store.SetRepoPages(owner, name, in.Enabled, in.Branch, in.Dir); err != nil {
		internalError(w, err)
		return
	}
	repo, err := a.store.GetRepo(owner, name)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, pagesConfig(repo, reqBase(r)))
}

// servePages 提供托管站点内容：GET /pages/{owner}/{repo}/<path>。
// 访问控制：私有仓库需读权限；未开启 Pages / 无权限 / 不存在一律 404（不泄露存在性）。
func (a *API) servePages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/pages/")
	owner, tail, ok := strings.Cut(rest, "/")
	if !ok || owner == "" {
		http.NotFound(w, r)
		return
	}
	repo, sub, _ := strings.Cut(tail, "/")
	if repo == "" || !gitsvc.ValidName(owner) || !gitsvc.ValidName(repo) {
		http.NotFound(w, r)
		return
	}
	rp, err := a.store.GetRepo(owner, repo)
	if err != nil || !rp.PagesEnabled {
		http.NotFound(w, r)
		return
	}
	if !a.store.CanRead(owner, repo, userFrom(r)) {
		http.NotFound(w, r)
		return
	}
	ref := rp.PagesBranch
	if ref == "" {
		ref = rp.DefaultBranch
	}
	if ref == "" {
		ref = "main"
	}
	dir := strings.Trim(rp.PagesDir, "/")
	if dir != "" {
		if d, derr := gitsvc.CleanPath(dir); derr == nil {
			dir = d
		} else {
			dir = ""
		}
	}

	sub, cerr := gitsvc.CleanPath(sub)
	if cerr != nil {
		http.NotFound(w, r)
		return
	}
	full := path.Join(dir, sub)

	candidates := []string{full}
	// 目录或扩展名缺失时回退该目录下的 index.html
	if sub == "" || !strings.Contains(path.Base(sub), ".") {
		candidates = append(candidates, path.Join(full, "index.html"))
	}
	for _, cand := range candidates {
		data, rerr := gitsvc.ReadRawFile(owner, repo, ref, cand)
		if rerr == nil {
			a.writePage(w, r, cand, data, http.StatusOK)
			return
		}
		if !errors.Is(rerr, gitsvc.ErrFileNotFound) {
			writeErr(w, http.StatusInternalServerError, "failed to read page")
			return
		}
	}
	// 自定义 404 页面
	if data, rerr := gitsvc.ReadRawFile(owner, repo, ref, path.Join(dir, "404.html")); rerr == nil {
		a.writePage(w, r, "404.html", data, http.StatusNotFound)
		return
	}
	http.NotFound(w, r)
}

func (a *API) writePage(w http.ResponseWriter, r *http.Request, name string, data []byte, status int) {
	w.Header().Set("Content-Type", pagesContentType(name))
	w.Header().Set("Cache-Control", "public, max-age=300")
	// 覆盖 secureHeaders 的默认 CSP：把 UGC 页面隔离为不透明源。
	w.Header().Set("Content-Security-Policy", pagesSandboxCSP)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write(data)
}

// pagesContentType 按扩展名给出 Content-Type；未知回退 mime 库/octet-stream。
func pagesContentType(name string) string {
	ext := strings.ToLower(path.Ext(name))
	if ct, ok := pagesContentTypes[ext]; ok {
		return ct
	}
	if ct := mime.TypeByExtension(ext); ct != "" {
		return ct
	}
	return "application/octet-stream"
}

var pagesContentTypes = map[string]string{
	".html":        "text/html; charset=utf-8",
	".htm":         "text/html; charset=utf-8",
	".css":         "text/css; charset=utf-8",
	".js":          "text/javascript; charset=utf-8",
	".mjs":         "text/javascript; charset=utf-8",
	".json":        "application/json; charset=utf-8",
	".map":         "application/json; charset=utf-8",
	".webmanifest": "application/manifest+json",
	".xml":         "application/xml; charset=utf-8",
	".txt":         "text/plain; charset=utf-8",
	".md":          "text/plain; charset=utf-8",
	".svg":         "image/svg+xml",
	".png":         "image/png",
	".jpg":         "image/jpeg",
	".jpeg":        "image/jpeg",
	".gif":         "image/gif",
	".webp":        "image/webp",
	".avif":        "image/avif",
	".ico":         "image/x-icon",
	".woff":        "font/woff",
	".woff2":       "font/woff2",
	".ttf":         "font/ttf",
	".otf":         "font/otf",
	".pdf":         "application/pdf",
	".wasm":        "application/wasm",
}
