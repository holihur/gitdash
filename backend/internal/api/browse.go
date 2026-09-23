package api

import (
	"gitdash/backend/internal/gitsvc"
	"gitdash/backend/internal/gpgsig"
	"net/http"
	"path"
	"strconv"
	"strings"
)

// browseRef 解析浏览用的 ref（分支/标签/commit）；为空时回退到仓库默认分支。
func (a *API) browseRef(r *http.Request, owner, name string) string {
	if ref := strings.TrimSpace(r.URL.Query().Get("ref")); ref != "" {
		return ref
	}
	if repo, err := a.store.GetRepo(owner, name); err == nil && repo.DefaultBranch != "" {
		return repo.DefaultBranch
	}
	return "main"
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
		internalError(w, err)
		return
	}
	notes, _ := a.store.RefNotes(owner, name)
	out := make([]gitsvc.Branch, 0, len(bs))
	for _, b := range bs {
		b.Note = notes["branch/"+b.Name]
		out = append(out, b)
	}
	total := len(out)
	limit, offset := pageParams(r)
	setTotal(w, total)
	writeJSON(w, http.StatusOK, pageSlice(out, limit, offset))
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
	ref := a.browseRef(r, owner, name)
	dir, err := gitsvc.CleanPath(r.URL.Query().Get("path"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	// 空仓库（尚无提交）没有可列举的默认分支：返回空目录而非原始 git 报错。
	if gitsvc.IsEmptyRepo(owner, name) {
		writeJSON(w, http.StatusOK, map[string]any{
			"path": dir, "entries": []gitsvc.Entry{}, "truncated": false, "empty": true,
		})
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
	// 当前目录的整体最后提交（“综合”信息），供前端 Latest commit 横幅展示
	latest, _ := gitsvc.LastCommit(owner, name, ref, dir)
	writeJSON(w, http.StatusOK, map[string]any{
		"path": dir, "entries": entries, "truncated": truncated, "latest_commit": latest,
	})
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
	ref := a.browseRef(r, owner, name)
	file := r.URL.Query().Get("path")
	if gitsvc.IsEmptyRepo(owner, name) {
		writeNotFound(w, "file")
		return
	}
	b, err := gitsvc.ReadBlob(owner, name, ref, file)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	b.LatestCommit, _ = gitsvc.LastCommit(owner, name, ref, file)
	writeJSON(w, http.StatusOK, b)
}

// rawFile 返回文件原始字节，供图片 / PDF 等二进制文件在浏览器内直接预览。
//
//	@Summary     原始文件内容
//	@Description 以正确的 Content-Type 返回文件原始字节（图片 / PDF 等可直接内联查看）。
//	@Tags        repos
//	@Produce     application/octet-stream
//	@Param       owner path string false "仓库所有者（简写路由时省略）"
//	@Param       name  path string true  "仓库名"
//	@Param       ref   query string false "分支/标签/commit"
//	@Param       path  query string true  "文件路径"
//	@Success     200 {file} binary
//	@Failure     404 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /repos/{name}/raw [get]
//	@Router      /users/{owner}/repos/{name}/raw [get]
func (a *API) rawFile(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, false)
	if !ok {
		return
	}
	ref := a.browseRef(r, owner, name)
	file := r.URL.Query().Get("path")
	if gitsvc.IsEmptyRepo(owner, name) {
		writeNotFound(w, "file")
		return
	}
	data, err := gitsvc.ReadRawFile(owner, name, ref, file)
	if err != nil {
		writeNotFound(w, "file")
		return
	}
	// 允许同源 iframe/object 内联预览（PDF），覆盖全局的 DENY / frame-ancestors 'none'。
	w.Header().Set("X-Frame-Options", "SAMEORIGIN")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'self'")
	w.Header().Set("Content-Type", pagesContentType(file))
	// 仅图片 / PDF 内联，其余类型强制下载，避免 raw HTML/SVG 直接导航。
	if isInlinePreviewable(file) {
		w.Header().Set("Content-Disposition", "inline")
	} else {
		w.Header().Set("Content-Disposition", "attachment")
	}
	w.Header().Set("Cache-Control", "private, max-age=300")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	_, _ = w.Write(data)
}

// isInlinePreviewable 报告文件类型是否可直接在浏览器内联预览。
func isInlinePreviewable(name string) bool {
	switch strings.ToLower(path.Ext(name)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".avif", ".bmp", ".ico", ".svg", ".pdf":
		return true
	}
	return false
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
	ref := a.browseRef(r, owner, name)
	if gitsvc.IsEmptyRepo(owner, name) {
		writeNotFound(w, "file")
		return
	}
	b, err := gitsvc.BlameFile(owner, name, ref, r.URL.Query().Get("path"))
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
//	@Param       ref    query string false "分支/标签/commit"
//	@Param       limit  query int    false "返回条数上限（默认 30，最大 100）"
//	@Param       offset query int    false "跳过条数（用于“加载更多”）"
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
	ref := a.browseRef(r, owner, name)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if gitsvc.IsEmptyRepo(owner, name) {
		writeJSON(w, http.StatusOK, []commitResp{})
		return
	}
	cs, err := gitsvc.Commits(owner, name, ref, limit, offset, r.URL.Query().Get("q"))
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
		r := commitResp{SHA: c.SHA, Author: c.Author, Date: c.Date, Message: c.Message, Parents: c.Parents, Refs: c.Refs}
		if raw, ok := raws[c.SHA]; ok {
			if user, _, status := gpgsig.VerifyCommit(raw, keys); status != gpgsig.StatusUnsigned {
				r.GPGStatus = status
				if status == gpgsig.StatusVerified {
					r.GPGVerified = user
				}
			}
		}
		out = append(out, r)
	}
	writeJSON(w, http.StatusOK, out)
}

type commitResp struct {
	SHA         string   `json:"sha"`
	Author      string   `json:"author"`
	Date        string   `json:"date"`
	Message     string   `json:"message"`
	Parents     []string `json:"parents,omitempty"`
	Refs        []string `json:"refs,omitempty"`
	GPGVerified string   `json:"gpg_verified,omitempty"`
	// GPG 签名状态：verified | unknown_key | invalid（无签名字段缺省，与旧行为兼容）
	GPGStatus string `json:"gpg_status,omitempty"`
}
