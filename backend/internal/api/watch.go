package api

import (
	"log"
	"net/http"
)

// watchRepo / unwatchRepo：关注 / 取消关注仓库（收件箱通知订阅）。
// 返回当前 watch 状态与该仓库的 watch 数。

func (a *API) writeWatchState(w http.ResponseWriter, owner, name, me string) {
	counts := a.store.WatchCounts([][2]string{{owner, name}})
	writeJSON(w, http.StatusOK, map[string]any{
		"watching": a.store.IsWatching(me, owner, name),
		"watchers": counts[[2]string{owner, name}],
	})
}

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

// notify 向某仓库的关注者推送一条收件箱通知：
//   - 显式 watch 该仓库的用户
//   - 个人仓库的所有者 / 组织仓库的全部成员（始终订阅自己仓库的动态）
//   - 不通知 actor 本人
//
// 通知写入失败不影响主操作（best-effort，仅记日志）。
func (a *API) notify(owner, repo, kind, action, actor string, number int64, title string) {
	seen := map[string]bool{}
	if watchers, err := a.store.WatchingUsers(owner, repo); err == nil {
		for _, u := range watchers {
			seen[u] = true
		}
	}
	if a.store.IsOrg(owner) {
		if members, err := a.store.OrgMembers(owner); err == nil {
			for _, m := range members {
				seen[m.Username] = true
			}
		}
	} else if owner != "" {
		seen[owner] = true
	}
	delete(seen, actor)
	users := make([]string, 0, len(seen))
	for u := range seen {
		if u != "" {
			users = append(users, u)
		}
	}
	if err := a.store.AddNotifications(users, kind, action, owner, repo, number, title, actor); err != nil {
		log.Printf("notify %s/%s: %v", owner, repo, err)
	}
}
