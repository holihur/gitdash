package api

import (
	"errors"
	"gitdash/backend/internal/store"
	"net/http"
)

// ---- user page & follow ----

// getUserProfile 用户主页：基本信息、关注统计与可见仓库。
//
//	@Summary     用户主页
//	@Description 返回用户公开资料、followers/following 数量、当前用户是否已关注，以及其可见仓库（他人仅公开仓库）。
//	@Tags        users
//	@Produce     json
//	@Param       username path string true "用户名"
//	@Success     200 {object} map[string]any
//	@Failure     404 {object} map[string]string
//	@Failure     500 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /users/{username} [get]
func (a *API) getUserProfile(w http.ResponseWriter, r *http.Request) {
	username := r.PathValue("username")
	u, err := a.store.GetByUsername(username)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeNotFound(w, "user")
			return
		}
		internalError(w, err)
		return
	}
	me := userFrom(r)
	followers, following, err := a.store.FollowCounts(username)
	if err != nil {
		internalError(w, err)
		return
	}
	repos, err := a.store.ListRepos(username)
	if err != nil {
		internalError(w, err)
		return
	}
	if username != me { // 他人主页只展示公开仓库
		public := make([]store.Repo, 0, len(repos))
		for _, repo := range repos {
			if !repo.Private {
				public = append(public, repo)
			}
		}
		repos = public
	}
	a.attachStars(repos, me)
	a.attachTopics(repos)
	writeJSON(w, http.StatusOK, map[string]any{
		"username":     u.Username,
		"bio":          u.Bio,
		"created_at":   u.CreatedAt,
		"followers":    followers,
		"following":    following,
		"is_self":      username == me,
		"is_following": username != me && a.store.IsFollowing(me, username),
		"repos":        repos,
		"avatar_url":   a.avatarURL(username),
	})
}

// writeFollowState 返回关注状态与统计。
func (a *API) writeFollowState(w http.ResponseWriter, username, me string) {
	followers, following, err := a.store.FollowCounts(username)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"followers":    followers,
		"following":    following,
		"is_following": a.store.IsFollowing(me, username),
	})
}

// followUser 关注用户。
//
//	@Summary     关注用户
//	@Description 幂等；不能关注自己。返回关注状态与统计。
//	@Tags        users
//	@Produce     json
//	@Param       username path string true "用户名"
//	@Success     200 {object} map[string]any
//	@Failure     400 {object} map[string]string
//	@Failure     404 {object} map[string]string
//	@Failure     500 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /users/{username}/follow [post]
func (a *API) followUser(w http.ResponseWriter, r *http.Request) {
	username := r.PathValue("username")
	me := userFrom(r)
	if username == me {
		writeCode(w, http.StatusBadRequest, "cannot_follow_self", "cannot follow yourself")
		return
	}
	if _, err := a.store.GetByUsername(username); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeNotFound(w, "user")
			return
		}
		internalError(w, err)
		return
	}
	if err := a.store.FollowUser(me, username); err != nil && !errors.Is(err, store.ErrExists) {
		internalError(w, err)
		return
	}
	a.writeFollowState(w, username, me)
}

// unfollowUser 取消关注。
//
//	@Summary     取消关注
//	@Description 幂等。返回关注状态与统计。
//	@Tags        users
//	@Produce     json
//	@Param       username path string true "用户名"
//	@Success     200 {object} map[string]any
//	@Failure     404 {object} map[string]string
//	@Failure     500 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /users/{username}/follow [delete]
func (a *API) unfollowUser(w http.ResponseWriter, r *http.Request) {
	username := r.PathValue("username")
	me := userFrom(r)
	if err := a.store.UnfollowUser(me, username); err != nil {
		internalError(w, err)
		return
	}
	a.writeFollowState(w, username, me)
}

// listFollowers 关注某用户的用户列表。
//
//	@Summary     关注者列表
//	@Tags        users
//	@Produce     json
//	@Param       username path string true "用户名"
//	@Param       limit    query int   false "每页数量（默认 200，最大 500）"
//	@Param       offset   query int   false "偏移量"
//	@Success     200 {array} store.UserSummary
//	@SuccessHeader X-Total-Count int "关注者总数"
//	@Failure     404 {object} map[string]string
//	@Failure     500 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /users/{username}/followers [get]
func (a *API) listFollowers(w http.ResponseWriter, r *http.Request) {
	username := r.PathValue("username")
	if _, err := a.store.GetByUsername(username); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeNotFound(w, "user")
			return
		}
		internalError(w, err)
		return
	}
	limit, offset := pageParams(r)
	users, err := a.store.ListFollowers(username, limit, offset)
	if err != nil {
		internalError(w, err)
		return
	}
	total, err := a.store.CountFollowers(username)
	if err != nil {
		internalError(w, err)
		return
	}
	setTotal(w, total)
	writeJSON(w, http.StatusOK, users)
}

// listFollowing 某用户关注的用户列表。
//
//	@Summary     关注列表
//	@Tags        users
//	@Produce     json
//	@Param       username path string true "用户名"
//	@Param       limit    query int   false "每页数量（默认 200，最大 500）"
//	@Param       offset   query int   false "偏移量"
//	@Success     200 {array} store.UserSummary
//	@SuccessHeader X-Total-Count int "关注总数"
//	@Failure     404 {object} map[string]string
//	@Failure     500 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /users/{username}/following [get]
func (a *API) listFollowing(w http.ResponseWriter, r *http.Request) {
	username := r.PathValue("username")
	if _, err := a.store.GetByUsername(username); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeNotFound(w, "user")
			return
		}
		internalError(w, err)
		return
	}
	limit, offset := pageParams(r)
	users, err := a.store.ListFollowing(username, limit, offset)
	if err != nil {
		internalError(w, err)
		return
	}
	total, err := a.store.CountFollowing(username)
	if err != nil {
		internalError(w, err)
		return
	}
	setTotal(w, total)
	writeJSON(w, http.StatusOK, users)
}
