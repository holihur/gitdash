package api

import (
	"gitdash/backend/internal/gitsvc"
	"net/http"
)

// gcRepo 对仓库执行 git gc（仓库所有者专属）。
//
//	@Summary     仓库垃圾回收
//	@Description 运行 `git gc` 打包松散对象并回收磁盘空间，仅仓库所有者可执行。返回回收前后的字节数。
//	@Tags        repos
//	@Produce     json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Success     200 {object} gitsvc.GCResult
//	@Failure     404 {object} map[string]string
//	@Failure     500 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /repos/{name}/gc [post]
//	@Router      /users/{owner}/repos/{name}/gc [post]
func (a *API) gcRepo(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireOwner(w, r)
	if !ok {
		return
	}
	res, err := gitsvc.GC(owner, name)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	// 对象布局变化后，让分支 / 标签缓存失效，后续浏览拿到最新 refs。
	gitsvc.InvalidateRefs(owner, name)
	writeJSON(w, http.StatusOK, res)
}
