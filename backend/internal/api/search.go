package api

import (
	"errors"
	"gitdash/backend/internal/codesearch"
	"gitdash/backend/internal/gitsvc"
	"net/http"
	"strconv"
	"strings"
)

// search 在仓库指定 ref 上做代码搜索。
//
//	@Summary     代码搜索
//	@Description 固定字符串全文搜索（跳过二进制文件），返回 {path, line, text} 列表。
//	@Description 以空格分隔的多个关键词按 AND 语义（命中行需同时包含全部关键词）。
//	@Tags        repos
//	@Produce     json
//	@Param       owner path string false "仓库所有者（简写路由时省略）"
//	@Param       name  path string true  "仓库名"
//	@Param       q     query string true  "搜索字符串"
//	@Param       ref   query string false "分支/标签/commit（默认默认分支）"
//	@Param       limit query int    false "返回条数上限（默认 50，最大 200）"
//	@Success     200 {array} gitsvc.SearchHit
//	@Failure     400 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /repos/{name}/search [get]
//	@Router      /users/{owner}/repos/{name}/search [get]
func (a *API) search(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, false)
	if !ok {
		return
	}
	q := r.URL.Query().Get("q")
	if q == "" {
		writeCode(w, http.StatusBadRequest, "query_required", "query parameter q is required")
		return
	}
	max, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	// 只把「显式 ref 查询参数」传给索引做范围校验；未显式指定视为默认分支（Ref 为空）。
	ref := strings.TrimSpace(r.URL.Query().Get("ref"))
	if gitsvc.IsEmptyRepo(owner, name) {
		writeJSON(w, http.StatusOK, []codesearch.Hit{})
		return
	}
	// 取仓库 ID：索引侧据此确认索引归属当前仓库（同名重建后旧索引自动失效）。
	var repoID int64
	if rp, gerr := a.store.GetRepo(owner, name); gerr == nil {
		repoID = rp.ID
	}
	hits, src, err := a.searchOne(r.Context(), owner, name, q, codesearch.Options{
		Ref:    ref,
		Max:    max,
		Terms:  strings.Fields(q),
		RepoID: repoID,
	})
	switch {
	case err == nil:
		w.Header().Set("X-Code-Search", string(src))
		writeJSON(w, http.StatusOK, hits)
	case errors.Is(err, codesearch.ErrRefNotIndexed):
		writeCode(w, http.StatusBadRequest, "ref_not_indexed", "only the default branch is indexed")
	case errors.Is(err, codesearch.ErrIndexing):
		// 最终一致：索引构建中，返回空列表并提示调用方可重试。
		w.Header().Set("X-Code-Search", "indexing")
		writeJSON(w, http.StatusOK, []codesearch.Hit{})
	default:
		writeErr(w, http.StatusBadRequest, err.Error())
	}
}

// globalSearch 全局搜索（跨仓库）。
//
//	@Summary     全局搜索
//	@Description 按 q 模糊搜索公开仓库、issue（公开仓库 + 自己仓库）与用户/组织，返回 {repos, issues, users}。
//	@Tags        search
//	@Produce     json
//	@Param       q     query string true  "搜索字符串"
//	@Param       limit query int    false "每类返回条数上限（默认 20，最大 50）"
//	@Success     200 {object} object
//	@Failure     400 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /search [get]
func (a *API) globalSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeCode(w, http.StatusBadRequest, "query_required", "query parameter q is required")
		return
	}
	if tooLong(w, "q", q, maxTitleRunes) {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	repos, err := a.store.SearchRepos(q, limit)
	if err != nil {
		internalError(w, err)
		return
	}
	issues, err := a.store.SearchIssues(q, userFrom(r), limit)
	if err != nil {
		internalError(w, err)
		return
	}
	users, err := a.store.SearchUsers(q, limit)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"repos": repos, "issues": issues, "users": users})
}
