package api

import (
	"errors"
	"net/http"

	"gitdash/backend/internal/store"
)

// ---- 仓库标签/话题（repo topics）----

// attachTopics 为一批仓库批量填充 topics（避免 N+1）。
func (a *API) attachTopics(repos []store.Repo) {
	if len(repos) == 0 {
		return
	}
	pairs := make([][2]string, 0, len(repos))
	for _, r := range repos {
		pairs = append(pairs, [2]string{r.Owner, r.Name})
	}
	m := a.store.TopicsForRepos(pairs)
	for i := range repos {
		repos[i].Topics = m[[2]string{repos[i].Owner, repos[i].Name}]
	}
}

type setRepoTopsReq struct {
	Topics []string `json:"topics"` // 小写字母/数字/连字符，最多 20 个
}

// setRepoTops 设置仓库标签（仅 owner）。
//
//	@Summary     设置仓库标签
//	@Description 全量替换仓库的 topics；仅仓库所有者可操作。
//	@Tags        repos
//	@Accept      json
//	@Produce     json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Param       body  body setRepoTopsReq true "topics 列表"
//	@Success     200 {object} map[string]any
//	@Failure     400 {object} map[string]string
//	@Failure     404 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/topics [put]
func (a *API) setRepoTops(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireRole(w, r, "write")
	if !ok {
		return
	}
	var in setRepoTopsReq
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	topics, valid := store.NormalizeTopics(in.Topics)
	if !valid {
		writeCode(w, http.StatusBadRequest, "invalid_topic",
			"topics must be 1-50 chars of lowercase letters, digits or '-', up to 20 topics")
		return
	}
	if err := a.store.SetRepoTopics(owner, name, topics); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeNotFound(w, "repo")
			return
		}
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"topics": topics})
}

// listTopics 列出公开仓库使用过的标签（按热度降序）。
//
//	@Summary     列出仓库标签
//	@Description 返回公开仓库的 topic 及使用数，供 Explore 筛选。
//	@Tags        repos
//	@Produce     json
//	@Success     200 {array} store.TopicCount
//	@Security    BearerAuth
//	@Router      /topics [get]
func (a *API) listTopics(w http.ResponseWriter, r *http.Request) {
	limit, offset := pageParams(r)
	topics, err := a.store.AllTopics(limit, offset)
	if err != nil {
		internalError(w, err)
		return
	}
	total, err := a.store.CountAllTopics()
	if err != nil {
		internalError(w, err)
		return
	}
	setTotal(w, total)
	writeJSON(w, http.StatusOK, topics)
}
