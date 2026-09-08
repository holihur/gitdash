package api

import (
	"gitdash/backend/internal/gitsvc"
	"gitdash/backend/internal/gpgsig"
	"net/http"
	"strconv"
)

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
	SHA         string `json:"sha"`
	Author      string `json:"author"`
	Date        string `json:"date"`
	Message     string `json:"message"`
	GPGVerified string `json:"gpg_verified,omitempty"`
	// GPG 签名状态：verified | unknown_key | invalid（无签名字段缺省，与旧行为兼容）
	GPGStatus string `json:"gpg_status,omitempty"`
}
