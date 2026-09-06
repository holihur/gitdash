package api

import (
	"net/http"
	"strconv"
	"strings"
)

// 评论：issue 与 PR 共用（kind 区分宿主）。

// issueOrPullComments 由闭包决定宿主类型，issue 与 PR 的路由共用同一组 handler。
func (a *API) issueOrPullComments(kind string) (list, add http.HandlerFunc) {
	return func(w http.ResponseWriter, r *http.Request) { a.listComments(w, r, kind) },
		func(w http.ResponseWriter, r *http.Request) { a.addComment(w, r, kind) }
}

// listComments 列出 issue/PR 的评论。
//
//	@Summary     列出评论
//	@Tags        comments
//	@Produce     json
//	@Param       owner  path string true "仓库所有者（owner 路由时）"
//	@Param       name   path string true "仓库名"
//	@Param       number path int    true "Issue/PR 编号"
//	@Param       limit  query int    false "每页数量"
//	@Param       offset query int    false "偏移量"
//	@Success     200 {array} store.Comment
//	@Failure     404 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/issues/{number}/comments [get]
//	@Router      /repos/{name}/issues/{number}/comments [get]
//	@Router      /users/{owner}/repos/{name}/pulls/{number}/comments [get]
//	@Router      /repos/{name}/pulls/{number}/comments [get]
func (a *API) listComments(w http.ResponseWriter, r *http.Request, kind string) {
	owner, name, ok := a.requireAccess(w, r, false)
	if !ok {
		return
	}
	number, err := strconv.ParseInt(r.PathValue("number"), 10, 64)
	if err != nil || number < 1 {
		writeCode(w, http.StatusBadRequest, "invalid_number", "invalid number")
		return
	}
	limit, offset := pageParams(r)
	comments, err := a.store.ListComments(owner, name, kind, number, limit, offset)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, comments)
}

// addComment 在 issue/PR 下发表评论。
//
//	@Summary     发表评论
//	@Description 需要仓库写权限；成功返回 201 并向关注者推送通知。
//	@Tags        comments
//	@Accept      json
//	@Produce     json
//	@Param       owner  path string true "仓库所有者（owner 路由时）"
//	@Param       name   path string true "仓库名"
//	@Param       number path int    true "Issue/PR 编号"
//	@Param       body   body addCommentReq true "评论正文"
//	@Success     201 {object} store.Comment
//	@Failure     400 {object} map[string]string
//	@Failure     404 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/issues/{number}/comments [post]
//	@Router      /repos/{name}/issues/{number}/comments [post]
//	@Router      /users/{owner}/repos/{name}/pulls/{number}/comments [post]
//	@Router      /repos/{name}/pulls/{number}/comments [post]
func (a *API) addComment(w http.ResponseWriter, r *http.Request, kind string) {
	owner, name, ok := a.requireAccess(w, r, true)
	if !ok {
		return
	}
	number, err := strconv.ParseInt(r.PathValue("number"), 10, 64)
	if err != nil || number < 1 {
		writeCode(w, http.StatusBadRequest, "invalid_number", "invalid number")
		return
	}
	// 宿主必须存在
	if kind == "issue" {
		if _, err := a.store.GetIssue(owner, name, number); err != nil {
			writeNotFound(w, "issue")
			return
		}
	} else {
		if _, err := a.store.GetPull(owner, name, number); err != nil {
			writeNotFound(w, "pull")
			return
		}
	}
	var in addCommentReq
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	body := strings.TrimSpace(in.Body)
	if body == "" {
		writeCode(w, http.StatusBadRequest, "body_required", "body is required")
		return
	}
	if len([]rune(body)) > 10000 {
		writeCode(w, http.StatusBadRequest, "body_too_long", "body too long (max 10000 chars)")
		return
	}
	me := userFrom(r)
	comment, err := a.store.CreateComment(owner, name, kind, number, me, body)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	// 通知关注者（issue/PR 作者与 watcher），不通知评论者本人
	if it, err := a.store.GetIssue(owner, name, number); err == nil {
		a.notify(owner, name, kind, "commented", me, number, it.Title)
	}
	writeJSON(w, http.StatusCreated, comment)
}

// deleteComment 删除评论（作者本人或仓库写权限）。
//
//	@Summary     删除评论
//	@Tags        comments
//	@Param       owner path string true "仓库所有者（owner 路由时）"
//	@Param       name  path string true "仓库名"
//	@Param       id    path int    true "评论 ID"
//	@Success     204 {object} nil
//	@Failure     403 {object} map[string]string
//	@Failure     404 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/comments/{id} [delete]
//	@Router      /repos/{name}/comments/{id} [delete]
func (a *API) deleteComment(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, false)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeCode(w, http.StatusBadRequest, "invalid_id", "invalid id")
		return
	}
	comment, err := a.store.GetComment(owner, name, id)
	if err != nil {
		writeNotFound(w, "comment")
		return
	}
	me := userFrom(r)
	if comment.Author != me && !a.store.CanWrite(owner, name, me) {
		writeCode(w, http.StatusForbidden, "comment_forbidden", "only the author or repo writers can delete a comment")
		return
	}
	if err := a.store.DeleteComment(owner, name, id); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
