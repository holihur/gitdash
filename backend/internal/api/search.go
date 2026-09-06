package api

import (
	"gitdash/backend/internal/gitsvc"
	"net/http"
	"strconv"
)

// search 在仓库指定 ref 上做代码搜索。
//
//	@Summary     代码搜索
//	@Description 固定字符串全文搜索（跳过二进制文件），返回 {path, line, text} 列表。
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
	ref := r.URL.Query().Get("ref")
	hits, err := gitsvc.Search(owner, name, q, ref, max)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, hits)
}
