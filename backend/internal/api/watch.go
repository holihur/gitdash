package api

import (
	"log"
	"net/http"
	"time"

	"gitdash/backend/internal/webhooks"
)

func (a *API) writeWatchState(w http.ResponseWriter, owner, name, me string) {
	counts := a.store.WatchCounts([][2]string{{owner, name}})
	writeJSON(w, http.StatusOK, map[string]any{
		"watching": a.store.IsWatching(me, owner, name),
		"watchers": counts[[2]string{owner, name}],
	})
}

// watchRepo 关注仓库（收件箱通知订阅）。
// 返回当前 watch 状态与该仓库的 watch 数。
//
//	@Summary     关注仓库
//	@Tags        watch
//	@Produce     json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Success     200 {object} object "watching 与 watchers"
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/watch [put]
func (a *API) watchRepo(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, false)
	if !ok {
		return
	}
	me := userFrom(r)
	if !a.store.IsWatching(me, owner, name) {
		if err := a.store.WatchRepo(me, owner, name); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	a.writeWatchState(w, owner, name, me)
}

func (a *API) unwatchRepo(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, false)
	if !ok {
		return
	}
	me := userFrom(r)
	if err := a.store.UnwatchRepo(me, owner, name); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.writeWatchState(w, owner, name, me)
}

// listWatched 我 watch 过的仓库（含公开仓库与我有权访问的仓库）。
//
//	@Summary     列出我关注的仓库
//	@Tags        watch
//	@Produce     json
//	@Success     200 {array} store.Repo
//	@Security    BearerAuth
//	@Router      /watched [get]
func (a *API) listWatched(w http.ResponseWriter, r *http.Request) {
	me := userFrom(r)
	repos, err := a.store.WatchedRepos(me)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.attachStars(repos, me)
	writeJSON(w, http.StatusOK, repos)
}

// notify 向某仓库的关注者推送一条收件箱通知，并异步发布 API 侧 webhook 事件
// （issues/pulls/comment，由 main 注入的 publisher 写入 dataDir/webhooks-spool-api）。
// 通知写入失败不影响主操作（best-effort，仅记日志）。
func (a *API) notify(owner, repo, kind, action, actor string, number int64, title, comment string) {
	users := a.store.NotifyRecipients(owner, repo, actor)
	if err := a.store.AddNotifications(users, kind, action, owner, repo, number, title, actor); err != nil {
		log.Printf("notify %s/%s: %v", owner, repo, err)
	}
	if a.Publish == nil {
		return
	}
	ev := webhooks.Event{
		Event:     "issues",
		Owner:     owner,
		Repo:      repo,
		Kind:      kind,
		Action:    action,
		Number:    number,
		Title:     title,
		Actor:     actor,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if kind == "pull" {
		ev.Event = "pulls"
	}
	if comment != "" {
		ev.Event = "comment"
		ev.Comment = comment
	}
	// 异步写 spool，不阻塞请求
	go a.Publish(ev)
}
