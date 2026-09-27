package api

import (
	"errors"
	"net/http"
	"sort"
	"strings"

	"gitdash/backend/internal/store"
)

// ---- 个人仓库置顶（pinned repositories）----

// pinRepoReq 置顶 / 取消置顶仓库请求体。
//
//nolint:unused // 仅供 swagger @Param 注解引用
type pinRepoReq struct {
	Owner string `json:"owner"` // 仓库归属（缺省为当前用户）
	Repo  string `json:"repo"`  // 仓库名
}

// pinnedRepos 把 pin 引用解析为完整仓库 DTO（保持 pin 顺序），并补齐展示字段。
// viewer 用来过滤私有仓库（以他人身份查看时）。
func (a *API) pinnedRepos(viewer string, pins []store.RepoPin) []store.Repo {
	repos := []store.Repo{}
	for _, p := range pins {
		repo, err := a.store.GetRepo(p.Owner, p.Repo)
		if err != nil {
			continue // 仓库已删除：跳过陈旧 pin
		}
		if viewer != repo.Owner && repo.Private {
			continue
		}
		repo.Pinned = true
		repo.PinPosition = p.Position
		repos = append(repos, repo)
	}
	a.attachStars(repos, viewer)
	a.attachTopics(repos)
	a.attachLanguages(repos)
	return repos
}

// listMyPins 列出当前用户置顶的仓库。
//
//	@Summary     列出置顶仓库
//	@Description 返回当前用户置顶的仓库（最多 6 个）与数量上限。
//	@Tags        users
//	@Produce     json
//	@Security    BearerAuth
//	@Success     200 {object} map[string]any
//	@Failure     401 {object} map[string]string
//	@Router      /me/pins [get]
func (a *API) listMyPins(w http.ResponseWriter, r *http.Request) {
	me := userFrom(r)
	pins, err := a.store.ListRepoPins(me)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"pins":  a.pinnedRepos(me, pins),
		"limit": store.MaxPinnedRepos,
	})
}

// pinMyRepo 置顶一个自己的仓库。
//
//	@Summary     置顶仓库
//	@Description 置顶当前用户拥有的仓库；幂等，最多 6 个（超限返回 409）。
//	@Tags        users
//	@Accept      json
//	@Produce     json
//	@Security    BearerAuth
//	@Param       body body pinRepoReq true "owner 与 repo"
//	@Success     200 {object} map[string]any
//	@Failure     400 {object} map[string]string
//	@Failure     403 {object} map[string]string
//	@Failure     404 {object} map[string]string
//	@Failure     409 {object} map[string]string
//	@Router      /me/pins [post]
func (a *API) pinMyRepo(w http.ResponseWriter, r *http.Request) {
	me := userFrom(r)
	var in pinRepoReq
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	owner := strings.TrimSpace(in.Owner)
	if owner == "" {
		owner = me
	}
	repo := strings.TrimSpace(in.Repo)
	if repo == "" {
		writeCode(w, http.StatusBadRequest, "invalid_repo_name", "repository name is required")
		return
	}
	if owner != me { // 仅允许置顶自己的仓库
		writeCode(w, http.StatusForbidden, "not_repo_owner", "you can only pin your own repositories")
		return
	}
	if _, err := a.store.GetRepo(owner, repo); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeNotFound(w, "repository")
			return
		}
		internalError(w, err)
		return
	}
	pins, err := a.store.PinRepo(me, owner, repo)
	if errors.Is(err, store.ErrPinLimit) {
		writeCode(w, http.StatusConflict, "pin_limit_reached", "you can pin at most 6 repositories")
		return
	}
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"pins":  a.pinnedRepos(me, pins),
		"limit": store.MaxPinnedRepos,
	})
}

// unpinMyRepo 取消置顶一个自己的仓库。
//
//	@Summary     取消置顶仓库
//	@Description 取消置顶指定的自己的仓库；幂等返回最新列表。
//	@Tags        users
//	@Produce     json
//	@Security    BearerAuth
//	@Param       owner path string true "仓库归属"
//	@Param       repo  path string true "仓库名"
//	@Success     200 {object} map[string]any
//	@Failure     401 {object} map[string]string
//	@Router      /me/pins/{owner}/{repo} [delete]
func (a *API) unpinMyRepo(w http.ResponseWriter, r *http.Request) {
	me := userFrom(r)
	owner := strings.TrimSpace(r.PathValue("owner"))
	repo := strings.TrimSpace(r.PathValue("repo"))
	pins, err := a.store.UnpinRepo(me, owner, repo)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"pins":  a.pinnedRepos(me, pins),
		"limit": store.MaxPinnedRepos,
	})
}

// annotatePins 为仓库列表标注 pinned / pin_position 并把置顶仓库排到最前。
func annotatePins(repos []store.Repo, pins []store.RepoPin) {
	if len(repos) == 0 || len(pins) == 0 {
		return
	}
	pos := make(map[string]int, len(pins))
	for _, p := range pins {
		pos[p.Owner+"/"+p.Repo] = p.Position
	}
	for i := range repos {
		if p, ok := pos[repos[i].Owner+"/"+repos[i].Name]; ok {
			repos[i].Pinned = true
			repos[i].PinPosition = p
		}
	}
	// 稳定排序：置顶在前（按 position），其余保持原有顺序。
	sort.SliceStable(repos, func(i, j int) bool {
		if repos[i].Pinned != repos[j].Pinned {
			return repos[i].Pinned
		}
		if repos[i].Pinned && repos[j].Pinned {
			return repos[i].PinPosition < repos[j].PinPosition
		}
		return false
	})
}
