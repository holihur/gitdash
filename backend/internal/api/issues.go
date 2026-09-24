package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"gitdash/backend/internal/logx"
	"gitdash/backend/internal/store"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

// listIssues 列出仓库的 issue 列表。
//
//	@Summary     列出 Issue
//	@Tags        issues
//	@Produce     json
//	@Param       owner  path string true "仓库所有者（owner 路由时）"
//	@Param       name   path string true "仓库名"
//	@Param       limit  query int    false "每页数量"
//	@Param       offset query int    false "偏移量"
//	@Param       q      query string false "关键词（标题/正文/作者）"
//	@Param       state  query string false "状态过滤：open 或 closed（空 = 全部）"
//	@Param       milestone query string false "里程碑过滤：里程碑 id 或 none（未指派，空 = 全部）"
//	@Success     200 {array} store.Issue
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/issues [get]
//	@Router      /repos/{name}/issues [get]
func (a *API) listIssues(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, false)
	if !ok {
		return
	}
	limit, offset := pageParams(r)
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	state := strings.TrimSpace(r.URL.Query().Get("state"))
	if state != "" && state != "open" && state != "closed" {
		writeCode(w, http.StatusBadRequest, "invalid_state", "state must be 'open' or 'closed'")
		return
	}
	milestone := strings.TrimSpace(r.URL.Query().Get("milestone"))
	if milestone != "" && milestone != "none" {
		if id, err := strconv.ParseInt(milestone, 10, 64); err != nil || id <= 0 {
			writeCode(w, http.StatusBadRequest, "invalid_milestone", "milestone must be a milestone id or 'none'")
			return
		}
	}
	label := strings.TrimSpace(r.URL.Query().Get("label"))
	if label != "" && label != "none" {
		if id, err := strconv.ParseInt(label, 10, 64); err != nil || id <= 0 {
			writeCode(w, http.StatusBadRequest, "invalid_label", "label must be a label id or 'none'")
			return
		}
	}
	assignee := strings.TrimSpace(r.URL.Query().Get("assignee"))
	if assignee == "me" {
		assignee = userFrom(r)
		if assignee == "" {
			assignee = "none"
		}
	}
	sort := strings.TrimSpace(r.URL.Query().Get("sort"))
	switch sort {
	case "", "newest", "oldest", "updated", "popular":
	default:
		writeCode(w, http.StatusBadRequest, "invalid_sort", "sort must be newest, oldest, updated or popular")
		return
	}
	filter := store.IssueFilter{Label: label, Assignee: assignee, Sort: sort}
	issues, err := a.store.SearchIssuesInRepo(owner, name, q, state, milestone, limit, offset, filter)
	if err != nil {
		internalError(w, err)
		return
	}
	total, err := a.store.CountSearchIssuesInRepo(owner, name, q, state, milestone, filter)
	if err != nil {
		internalError(w, err)
		return
	}
	setTotal(w, total)
	writeJSON(w, http.StatusOK, a.enrichIssues(owner, name, issues))
}

// issueCounts 返回同过滤条件下 open / closed 的真实数量（不受分页影响）。
//
//	@Summary     Issue 状态计数
//	@Tags        issues
//	@Produce     json
//	@Param       owner path string true "仓库所有者（owner 路由时）"
//	@Param       name  path string true "仓库名"
//	@Param       q     query string false "关键词"
//	@Param       label query string false "标签 id 或 none"
//	@Param       milestone query string false "里程碑 id 或 none"
//	@Param       assignee query string false "负责人用户名、me 或 none"
//	@Success     200 {object} map[string]int
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/issues/counts [get]
//	@Router      /repos/{name}/issues/counts [get]
func (a *API) issueCounts(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, false)
	if !ok {
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	milestone := strings.TrimSpace(r.URL.Query().Get("milestone"))
	label := strings.TrimSpace(r.URL.Query().Get("label"))
	assignee := strings.TrimSpace(r.URL.Query().Get("assignee"))
	if assignee == "me" {
		assignee = userFrom(r)
	}
	open, closed, err := a.store.CountIssueStates(owner, name, q, milestone, label, assignee)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"open": open, "closed": closed})
}

// createIssue 创建 issue。
//
//	@Summary     创建 Issue
//	@Tags        issues
//	@Accept      json
//	@Produce     json
//	@Param       owner path string true "仓库所有者（owner 路由时）"
//	@Param       name  path string true "仓库名"
//	@Param       body  body createIssueReq true "标题与正文"
//	@Success     201 {object} store.Issue
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/issues [post]
//	@Router      /repos/{name}/issues [post]
func (a *API) createIssue(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireRole(w, r, "triage")
	if !ok {
		return
	}
	var in createIssueReq
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	issue, ok := a.newIssue(w, owner, name, userFrom(r), in.Title, in.Body)
	if !ok {
		return
	}
	writeJSON(w, http.StatusCreated, issue)
}

// newIssue 校验并创建 issue（校验/权限失败时已写入响应），推送通知与 webhook 后返回
// 组装好的 issue。createIssue 与入站 webhook 共用此逻辑。
//
// 可选的 labelNames 会在创建后自动附到 issue 上（标签不存在时按需创建），
// 目前用于把反馈 issue 打上 "feedback" 标签。
func (a *API) newIssue(w http.ResponseWriter, owner, name, author, title, body string, labelNames ...string) (map[string]any, bool) {
	title = strings.TrimSpace(title)
	if title == "" {
		writeCode(w, http.StatusBadRequest, "title_required", "title is required")
		return nil, false
	}
	if len([]rune(title)) > 200 {
		writeCode(w, http.StatusBadRequest, "title_too_long", "title too long (max 200 chars)")
		return nil, false
	}
	if len([]rune(body)) > 10000 {
		writeCode(w, http.StatusBadRequest, "body_too_long", "body too long (max 10000 chars)")
		return nil, false
	}
	repo, err := a.store.GetRepo(owner, name)
	if err != nil {
		internalError(w, err)
		return nil, false
	}
	if !repo.HasIssues {
		writeCode(w, http.StatusForbidden, "issues_disabled", "issues are disabled for this repository")
		return nil, false
	}
	issue, err := a.store.CreateIssue(owner, name, author, title, body)
	if err != nil {
		internalError(w, err)
		return nil, false
	}
	if len(labelNames) > 0 {
		if err := a.attachIssueLabels(owner, name, issue.Number, labelNames...); err != nil {
			// 打标签失败不影响 issue 本身；记录日志便于排查。
			logx.Warnf("attach issue labels %s/%s#%d: %v", owner, name, issue.Number, err)
		}
	}
	a.notify(owner, name, "issue", "opened", author, issue.Number, issue.Title, "")
	// 作者自动订阅，并记入时间线。
	_ = a.store.SubscribeIssue(owner, name, "issue", issue.Number, author)
	_ = a.store.AddIssueEvent(owner, name, "issue", issue.Number, author, "opened", "")
	return a.enrichIssues(owner, name, []store.Issue{issue})[0], true
}

// attachIssueLabels 把标签附加到指定 issue（仅用于新建的 issue）：标签不存在时自动创建。
// 供内部调用（如反馈 issue 自动打标）使用。
func (a *API) attachIssueLabels(owner, repo string, number int64, names ...string) error {
	labels, err := a.store.ListLabels(owner, repo)
	if err != nil {
		return err
	}
	byName := make(map[string]int64, len(labels))
	for _, l := range labels {
		byName[strings.ToLower(l.Name)] = l.ID
	}
	ids := make([]int64, 0, len(names))
	for _, n := range names {
		name := strings.TrimSpace(n)
		if name == "" {
			continue
		}
		id, ok := byName[strings.ToLower(name)]
		if !ok {
			created, err := a.store.CreateLabel(owner, repo, name, issueLabelColor(name))
			switch {
			case err == nil:
				id = created.ID
			case errors.Is(err, store.ErrExists):
				// 并发下已被创建：重查一次。
				labels, err = a.store.ListLabels(owner, repo)
				if err != nil {
					return err
				}
				for _, l := range labels {
					if strings.EqualFold(l.Name, name) {
						id, ok = l.ID, true
						break
					}
				}
				if !ok {
					return fmt.Errorf("label %q not found after create", name)
				}
			default:
				return err
			}
		}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil
	}
	return a.store.SetIssueLabels(owner, repo, number, ids)
}

// issueLabelColor 返回内部自动创建标签时使用的颜色。
func issueLabelColor(name string) string {
	if strings.EqualFold(name, "feedback") {
		return "fbca04"
	}
	return "ededed"
}

// getIssue 获取单个 issue（含标签 / 里程碑），供 issue 详情页使用。
//
//	@Summary     获取 Issue
//	@Tags        issues
//	@Produce     json
//	@Param       owner  path string true "仓库所有者（owner 路由时）"
//	@Param       name   path string true "仓库名"
//	@Param       number path int    true "Issue 编号"
//	@Success     200 {object} store.Issue
//	@Failure     404 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/issues/{number} [get]
//	@Router      /repos/{name}/issues/{number} [get]
func (a *API) getIssue(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, false)
	if !ok {
		return
	}
	number, err := strconv.ParseInt(r.PathValue("number"), 10, 64)
	if err != nil || number < 1 {
		writeCode(w, http.StatusBadRequest, "invalid_issue_number", "invalid issue number")
		return
	}
	issue, err := a.store.GetIssue(owner, name, number)
	if errors.Is(err, store.ErrNotFound) {
		writeCode(w, http.StatusNotFound, "issue_not_found", "issue not found")
		return
	}
	if err != nil {
		internalError(w, err)
		return
	}
	out := a.enrichIssues(owner, name, []store.Issue{issue})[0]
	me := userFrom(r)
	out["subscribed"] = me != "" && a.store.IsSubscribed(owner, name, "issue", number, me)
	out["linked_pulls"] = a.linkedPulls(owner, name, issue)
	writeJSON(w, http.StatusOK, out)
}

var refNumberRe = regexp.MustCompile(`#(\d+)`)

// linkedPulls 从 issue 正文与评论中提取 #number 引用，返回同仓库内对应的 PR。
func (a *API) linkedPulls(owner, name string, issue store.Issue) []store.PullRequest {
	nums := map[int64]bool{}
	collect := func(s string) {
		for _, m := range refNumberRe.FindAllStringSubmatch(s, -1) {
			if n, err := strconv.ParseInt(m[1], 10, 64); err == nil && n > 0 {
				nums[n] = true
			}
		}
	}
	collect(issue.Body)
	if cs, err := a.store.ListComments(owner, name, "issue", issue.Number, 0, 0); err == nil {
		for _, c := range cs {
			collect(c.Body)
		}
	}
	if len(nums) == 0 {
		return []store.PullRequest{}
	}
	list := make([]int64, 0, len(nums))
	for n := range nums {
		list = append(list, n)
	}
	prs, _ := a.store.PullsByNumbers(owner, name, list)
	return prs
}

// updateIssue 编辑 issue（标题 / 正文 / 状态，字段均可选）。
//
//	@Summary     编辑 Issue
//	@Description 局部更新标题、正文、状态与置顶；至少提供一个字段。
//	@Tags        issues
//	@Accept      json
//	@Produce     json
//	@Param       owner  path string true "仓库所有者（owner 路由时）"
//	@Param       name   path string true "仓库名"
//	@Param       number path int    true "Issue 编号"
//	@Param       body   body updateIssueReq true "标题 / 正文 / 状态（均可选）"
//	@Success     200 {object} store.Issue
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/issues/{number} [patch]
//	@Router      /repos/{name}/issues/{number} [patch]
func (a *API) updateIssue(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireRole(w, r, "triage")
	if !ok {
		return
	}
	number, err := strconv.ParseInt(r.PathValue("number"), 10, 64)
	if err != nil || number < 1 {
		writeCode(w, http.StatusBadRequest, "invalid_issue_number", "invalid issue number")
		return
	}
	var in struct {
		Title       *string `json:"title"`
		Body        *string `json:"body"`
		State       *string `json:"state"`
		Pinned      *bool   `json:"pinned"`
		Comment     string  `json:"comment"`      // 可选：关闭/重开时附带评论
		StateReason string  `json:"state_reason"` // 可选：completed | not_planned
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}

	prev, err := a.store.GetIssue(owner, name, number)
	if errors.Is(err, store.ErrNotFound) {
		writeCode(w, http.StatusNotFound, "issue_not_found", "issue not found")
		return
	}
	if err != nil {
		internalError(w, err)
		return
	}

	var titlePtr, bodyPtr *string
	if in.Title != nil {
		t := strings.TrimSpace(*in.Title)
		if t == "" {
			writeCode(w, http.StatusBadRequest, "title_required", "title is required")
			return
		}
		if len([]rune(t)) > 200 {
			writeCode(w, http.StatusBadRequest, "title_too_long", "title too long (max 200 chars)")
			return
		}
		titlePtr = &t
	}
	if in.Body != nil {
		if len([]rune(*in.Body)) > 10000 {
			writeCode(w, http.StatusBadRequest, "body_too_long", "body too long (max 10000 chars)")
			return
		}
		bodyPtr = in.Body
	}
	state := ""
	if in.State != nil {
		state = *in.State // 不 trim："closed " 等带空白值应被拒绝
		if state != "open" && state != "closed" {
			writeCode(w, http.StatusBadRequest, "invalid_state", "state must be 'open' or 'closed'")
			return
		}
	}
	if titlePtr == nil && bodyPtr == nil && state == "" && in.Pinned == nil && strings.TrimSpace(in.Comment) == "" {
		writeCode(w, http.StatusBadRequest, "no_changes", "provide title, body, state, pinned or comment")
		return
	}
	if in.Comment != "" && len([]rune(in.Comment)) > 10000 {
		writeCode(w, http.StatusBadRequest, "comment_too_long", "comment too long (max 10000 chars)")
		return
	}
	reason := strings.TrimSpace(in.StateReason)
	if reason != "" && reason != "completed" && reason != "not_planned" {
		writeCode(w, http.StatusBadRequest, "invalid_state_reason", "state_reason must be completed or not_planned")
		return
	}

	issue, err := a.store.UpdateIssue(owner, name, number, titlePtr, bodyPtr)
	if err != nil {
		internalError(w, err)
		return
	}
	if state != "" && state != issue.State {
		issue, err = a.store.SetIssueStateWithReason(owner, name, number, state, reason)
		if err != nil {
			internalError(w, err)
			return
		}
	}
	if in.Pinned != nil && *in.Pinned != issue.Pinned {
		issue, err = a.store.SetIssuePinned(owner, name, number, *in.Pinned)
		if err != nil {
			internalError(w, err)
			return
		}
	}

	me := userFrom(r)
	// 通知：状态变化优先（closed/reopened），否则内容编辑发 edited（重复值不发）；同步记入时间线。
	switch {
	case prev.State != issue.State:
		action := "closed"
		if issue.State == "open" {
			action = "reopened"
		}
		_ = a.store.AddIssueEvent(owner, name, "issue", number, me, action, "")
		a.notify(owner, name, "issue", action, me, issue.Number, issue.Title, "")
	case (titlePtr != nil && *titlePtr != prev.Title) || (bodyPtr != nil && *bodyPtr != prev.Body):
		_ = a.store.AddIssueEvent(owner, name, "issue", number, me, "edited", "")
		a.notify(owner, name, "issue", "edited", me, issue.Number, issue.Title, "")
	}
	if in.Pinned != nil && *in.Pinned != prev.Pinned {
		action := "pinned"
		if !issue.Pinned {
			action = "unpinned"
		}
		_ = a.store.AddIssueEvent(owner, name, "issue", number, me, action, "")
	}
	// 关闭/重开时可附带一条评论（「Close with comment」）。
	if c := strings.TrimSpace(in.Comment); c != "" {
		if _, e := a.store.CreateComment(owner, name, "issue", number, me, c, nil); e != nil {
			internalError(w, e)
			return
		}
		_ = a.store.AddIssueEvent(owner, name, "issue", number, me, "commented", "")
		summary := []rune(c)
		if len(summary) > 200 {
			summary = summary[:200]
		}
		a.notifyMessage(owner, name, "issue", "commented", me, number, issue.Title, string(summary), store.ExtractMentions(c), "")
	}
	writeJSON(w, http.StatusOK, a.enrichIssues(owner, name, []store.Issue{issue})[0])
}

// deleteIssue 删除 issue（级联清理标签 / 评论 / 通知 / 看板卡片）。
//
//	@Summary     删除 Issue
//	@Tags        issues
//	@Param       owner  path string true "仓库所有者（owner 路由时）"
//	@Param       name   path string true "仓库名"
//	@Param       number path int    true "Issue 编号"
//	@Success     204 {object} nil
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/issues/{number} [delete]
//	@Router      /repos/{name}/issues/{number} [delete]
func (a *API) deleteIssue(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireRole(w, r, "triage")
	if !ok {
		return
	}
	number, err := strconv.ParseInt(r.PathValue("number"), 10, 64)
	if err != nil || number < 1 {
		writeCode(w, http.StatusBadRequest, "invalid_issue_number", "invalid issue number")
		return
	}
	if errors.Is(a.store.DeleteIssue(owner, name, number), store.ErrNotFound) {
		writeCode(w, http.StatusNotFound, "issue_not_found", "issue not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) enrichIssues(owner, repo string, issues []store.Issue) []map[string]any {
	numbers := make([]int64, 0, len(issues))
	for _, it := range issues {
		numbers = append(numbers, it.Number)
	}
	labels, _ := a.store.IssueLabels(owner, repo, numbers)
	milestones, _ := a.store.IssueMilestones(owner, repo, numbers)
	assignees, _ := a.store.IssueAssignees(owner, repo, numbers)
	commentCounts := a.store.IssueCommentCounts(owner, repo, "issue", numbers)
	out := make([]map[string]any, 0, len(issues))
	for _, it := range issues {
		raw, _ := json.Marshal(it)
		var m map[string]any
		_ = json.Unmarshal(raw, &m)
		ls := []store.Label{}
		if v, ok := labels[it.Number]; ok {
			ls = v
		}
		m["labels"] = ls
		if ms, ok := milestones[it.Number]; ok {
			m["milestone"] = ms
		} else {
			m["milestone"] = nil
		}
		as := assignees[it.Number]
		if as == nil {
			as = []string{}
		}
		m["assignees"] = as
		m["comment_count"] = commentCounts[it.Number]
		out = append(out, m)
	}
	return out
}

// setIssueLabels 设置 issue 的标签。
//
//	@Summary     设置 Issue 标签
//	@Tags        issues
//	@Accept      json
//	@Produce     json
//	@Param       owner  path string   true "仓库所有者"
//	@Param       name   path string   true "仓库名"
//	@Param       number path int      true "Issue 编号"
//	@Param       body   body setIssueLabelsReq   true "标签 ID 列表"
//	@Success     200 {object} store.Issue
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/issues/{number}/labels [post]
func (a *API) setIssueLabels(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireRole(w, r, "triage")
	if !ok {
		return
	}
	n, err := strconv.ParseInt(r.PathValue("number"), 10, 64)
	if err != nil || n < 1 {
		writeCode(w, http.StatusBadRequest, "invalid_issue_number", "invalid issue number")
		return
	}
	var in struct {
		LabelIDs []int64 `json:"label_ids"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	before, _ := a.store.IssueLabels(owner, name, []int64{n})
	err = a.store.SetIssueLabels(owner, name, n, in.LabelIDs)
	if errors.Is(err, store.ErrNotFound) {
		writeCode(w, http.StatusNotFound, "issue_not_found", "issue not found")
		return
	}
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_label", err.Error())
		return
	}
	after, _ := a.store.IssueLabels(owner, name, []int64{n})
	a.recordLabelDelta(owner, name, n, userFrom(r), before[n], after[n])
	issue, err := a.store.GetPullIssue(owner, name, n)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, a.enrichIssues(owner, name, []store.Issue{issue})[0])
}

// setIssueMilestone 设置 issue 的里程碑。
//
//	@Summary     设置 Issue 里程碑
//	@Tags        issues
//	@Accept      json
//	@Produce     json
//	@Param       owner  path string true "仓库所有者"
//	@Param       name   path string true "仓库名"
//	@Param       number path int    true "Issue 编号"
//	@Param       body  body setIssueMilestoneReq true "里程碑 ID（0 = 清除）"
//	@Success     200 {object} store.Issue
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/issues/{number}/milestone [post]
func (a *API) setIssueMilestone(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireRole(w, r, "triage")
	if !ok {
		return
	}
	n, err := strconv.ParseInt(r.PathValue("number"), 10, 64)
	if err != nil || n < 1 {
		writeCode(w, http.StatusBadRequest, "invalid_issue_number", "invalid issue number")
		return
	}
	var in struct {
		MilestoneID int64 `json:"milestone_id"` // 0 = 清除
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	before, _ := a.store.IssueMilestones(owner, name, []int64{n})
	err = a.store.SetIssueMilestone(owner, name, n, in.MilestoneID)
	if errors.Is(err, store.ErrNotFound) {
		writeCode(w, http.StatusNotFound, "issue_not_found", "issue not found")
		return
	}
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_milestone", err.Error())
		return
	}
	after, _ := a.store.IssueMilestones(owner, name, []int64{n})
	oldTitle, newTitle := "", ""
	if m, ok := before[n]; ok {
		oldTitle = m.Title
	}
	if m, ok := after[n]; ok {
		newTitle = m.Title
	}
	if oldTitle != newTitle {
		action, detail := "milestoned", newTitle
		if newTitle == "" {
			action, detail = "demilestoned", oldTitle
		}
		_ = a.store.AddIssueEvent(owner, name, "issue", n, userFrom(r), action, detail)
	}
	issue, err := a.store.GetPullIssue(owner, name, n)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, a.enrichIssues(owner, name, []store.Issue{issue})[0])
}

// recordLabelDelta 对比标签前后差异，写入 labeled / unlabeled 时间线事件。
func (a *API) recordLabelDelta(owner, name string, number int64, actor string, before, after []store.Label) {
	prev := map[int64]string{}
	for _, l := range before {
		prev[l.ID] = l.Name
	}
	next := map[int64]string{}
	for _, l := range after {
		next[l.ID] = l.Name
	}
	for id, lname := range next {
		if _, ok := prev[id]; !ok {
			_ = a.store.AddIssueEvent(owner, name, "issue", number, actor, "labeled", lname)
		}
	}
	for id, lname := range prev {
		if _, ok := next[id]; !ok {
			_ = a.store.AddIssueEvent(owner, name, "issue", number, actor, "unlabeled", lname)
		}
	}
}

// setIssueAssignees 全量设置 issue 负责人（需写权限）。
//
//	@Summary     设置 Issue 负责人
//	@Tags        issues
//	@Accept      json
//	@Produce     json
//	@Param       owner  path string true "仓库所有者"
//	@Param       name   path string true "仓库名"
//	@Param       number path int    true "Issue 编号"
//	@Param       body   body map[string]interface{} true "负责人用户名列表"
//	@Success     200 {object} store.Issue
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/issues/{number}/assignees [put]
func (a *API) setIssueAssignees(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireRole(w, r, "triage")
	if !ok {
		return
	}
	n, err := strconv.ParseInt(r.PathValue("number"), 10, 64)
	if err != nil || n < 1 {
		writeCode(w, http.StatusBadRequest, "invalid_issue_number", "invalid issue number")
		return
	}
	var in struct {
		Assignees []string `json:"assignees"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	if len(in.Assignees) > 10 {
		writeCode(w, http.StatusBadRequest, "too_many_assignees", "at most 10 assignees")
		return
	}
	valid := a.store.ExistingUsernames(in.Assignees)
	before, _ := a.store.IssueAssignees(owner, name, []int64{n})
	if err := a.store.SetIssueAssignees(owner, name, n, valid); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeCode(w, http.StatusNotFound, "issue_not_found", "issue not found")
			return
		}
		internalError(w, err)
		return
	}
	after, _ := a.store.IssueAssignees(owner, name, []int64{n})
	prev := map[string]bool{}
	for _, u := range before[n] {
		prev[u] = true
	}
	next := map[string]bool{}
	for _, u := range after[n] {
		next[u] = true
	}
	me := userFrom(r)
	for _, u := range valid {
		if !prev[u] {
			_ = a.store.AddIssueEvent(owner, name, "issue", n, me, "assigned", u)
		}
	}
	for _, u := range before[n] {
		if !next[u] {
			_ = a.store.AddIssueEvent(owner, name, "issue", n, me, "unassigned", u)
		}
	}
	issue, err := a.store.GetPullIssue(owner, name, n)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, a.enrichIssues(owner, name, []store.Issue{issue})[0])
}

// subscribeIssue 订阅 issue（作者与评论者已自动订阅）。
//
//	@Summary     订阅 Issue
//	@Tags        issues
//	@Produce     json
//	@Param       owner  path string true "仓库所有者"
//	@Param       name   path string true "仓库名"
//	@Param       number path int    true "Issue 编号"
//	@Success     200 {object} map[string]bool
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/issues/{number}/subscribe [post]
func (a *API) subscribeIssue(w http.ResponseWriter, r *http.Request) {
	a.setIssueSubscription(w, r, true)
}

// unsubscribeIssue 取消订阅 issue。
//
//	@Summary     取消订阅 Issue
//	@Tags        issues
//	@Produce     json
//	@Param       owner  path string true "仓库所有者"
//	@Param       name   path string true "仓库名"
//	@Param       number path int    true "Issue 编号"
//	@Success     200 {object} map[string]bool
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/issues/{number}/subscribe [delete]
func (a *API) unsubscribeIssue(w http.ResponseWriter, r *http.Request) {
	a.setIssueSubscription(w, r, false)
}

func (a *API) setIssueSubscription(w http.ResponseWriter, r *http.Request, on bool) {
	owner, name, ok := a.requireAccess(w, r, false)
	if !ok {
		return
	}
	n, err := strconv.ParseInt(r.PathValue("number"), 10, 64)
	if err != nil || n < 1 {
		writeCode(w, http.StatusBadRequest, "invalid_issue_number", "invalid issue number")
		return
	}
	if _, e := a.store.GetIssue(owner, name, n); e != nil {
		writeNotFound(w, "issue")
		return
	}
	me := userFrom(r)
	if on {
		_ = a.store.SubscribeIssue(owner, name, "issue", n, me)
	} else {
		_ = a.store.UnsubscribeIssue(owner, name, "issue", n, me)
	}
	writeJSON(w, http.StatusOK, map[string]bool{"subscribed": on})
}

// listIssueEvents 列出 issue 的活动时间线。
//
//	@Summary     Issue 时间线
//	@Tags        issues
//	@Produce     json
//	@Param       owner  path string true "仓库所有者"
//	@Param       name   path string true "仓库名"
//	@Param       number path int    true "Issue 编号"
//	@Success     200 {array} store.IssueEvent
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/issues/{number}/events [get]
func (a *API) listIssueEvents(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, false)
	if !ok {
		return
	}
	n, err := strconv.ParseInt(r.PathValue("number"), 10, 64)
	if err != nil || n < 1 {
		writeCode(w, http.StatusBadRequest, "invalid_issue_number", "invalid issue number")
		return
	}
	events, err := a.store.ListIssueEvents(owner, name, "issue", n)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, events)
}

// listLabels 列出仓库的标签。
//
//	@Summary     列出标签
//	@Tags        issues
//	@Produce     json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Success     200 {array} store.Label
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/labels [get]
func (a *API) listLabels(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, false)
	if !ok {
		return
	}
	limit, offset := pageParams(r)
	ls, err := a.store.ListLabelsPaged(owner, name, limit, offset)
	if err != nil {
		internalError(w, err)
		return
	}
	total, err := a.store.CountLabels(owner, name)
	if err != nil {
		internalError(w, err)
		return
	}
	setTotal(w, total)
	writeJSON(w, http.StatusOK, ls)
}

var labelColorRe = regexp.MustCompile(`^#?[0-9a-fA-F]{6}$`)

// createLabel 创建标签。
//
//	@Summary     创建标签
//	@Tags        issues
//	@Accept      json
//	@Produce     json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Param       body  body createLabelReq true "名称与颜色"
//	@Success     201 {object} store.Label
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/labels [post]
func (a *API) createLabel(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireRole(w, r, "triage")
	if !ok {
		return
	}
	var in struct {
		Name  string `json:"name"`
		Color string `json:"color"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Color = strings.TrimSpace(strings.TrimPrefix(in.Color, "#"))
	if in.Name == "" {
		writeCode(w, http.StatusBadRequest, "label_name_required", "label name is required")
		return
	}
	if len([]rune(in.Name)) > 50 {
		writeCode(w, http.StatusBadRequest, "label_name_required", "label name too long (max 50)")
		return
	}
	if in.Color == "" {
		in.Color = "0366d6"
	}
	if !labelColorRe.MatchString(in.Color) {
		writeCode(w, http.StatusBadRequest, "invalid_color", "color must be a hex value like 'ff0000'")
		return
	}
	l, err := a.store.CreateLabel(owner, name, in.Name, in.Color)
	if errors.Is(err, store.ErrExists) {
		writeCode(w, http.StatusConflict, "label_exists", "label already exists")
		return
	}
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, l)
}

// updateLabel 更新标签。
//
//	@Summary     更新标签
//	@Tags        issues
//	@Accept      json
//	@Produce     json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Param       id    path int    true "标签 ID"
//	@Param       body  body updateLabelReq true "名称与颜色"
//	@Success     200 {object} store.Label
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/labels/{id} [patch]
func (a *API) updateLabel(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireRole(w, r, "triage")
	if !ok {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeCode(w, http.StatusBadRequest, "invalid_id", "invalid id")
		return
	}
	var in struct {
		Name  string `json:"name"`
		Color string `json:"color"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Color = strings.TrimSpace(strings.TrimPrefix(in.Color, "#"))
	if tooLong(w, "name", in.Name, 50) {
		return
	}
	if in.Name == "" {
		in.Name = "" // keep
	}
	if in.Color != "" && !labelColorRe.MatchString(in.Color) {
		writeCode(w, http.StatusBadRequest, "invalid_color", "color must be a hex value like 'ff0000'")
		return
	}
	if in.Name == "" && in.Color == "" {
		writeCode(w, http.StatusBadRequest, "label_name_required", "provide name or color")
		return
	}
	cur, err := a.store.ListLabels(owner, name)
	if err != nil {
		internalError(w, err)
		return
	}
	var curLabel *store.Label
	for i := range cur {
		if cur[i].ID == id {
			curLabel = &cur[i]
			break
		}
	}
	if curLabel == nil {
		writeCode(w, http.StatusNotFound, "label_not_found", "label not found")
		return
	}
	nn, cc := curLabel.Name, curLabel.Color
	if in.Name != "" {
		nn = in.Name
	}
	if in.Color != "" {
		cc = in.Color
	}
	upd, err := a.store.UpdateLabel(owner, name, id, nn, cc)
	if errors.Is(err, store.ErrNotFound) {
		writeCode(w, http.StatusNotFound, "label_not_found", "label not found")
		return
	}
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, upd)
}

// deleteLabel 删除标签。
//
//	@Summary     删除标签
//	@Tags        issues
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Param       id    path int    true "标签 ID"
//	@Success     204 {object} nil
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/labels/{id} [delete]
func (a *API) deleteLabel(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireRole(w, r, "triage")
	if !ok {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeCode(w, http.StatusBadRequest, "invalid_id", "invalid id")
		return
	}
	if errors.Is(a.store.DeleteLabel(owner, name, id), store.ErrNotFound) {
		writeCode(w, http.StatusNotFound, "label_not_found", "label not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// listMilestones 列出仓库的里程碑。
//
//	@Summary     列出里程碑
//	@Tags        issues
//	@Produce     json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Success     200 {array} store.Milestone
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/milestones [get]
func (a *API) listMilestones(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, false)
	if !ok {
		return
	}
	limit, offset := pageParams(r)
	ms, err := a.store.ListMilestonesPaged(owner, name, limit, offset)
	if err != nil {
		internalError(w, err)
		return
	}
	total, err := a.store.CountMilestones(owner, name)
	if err != nil {
		internalError(w, err)
		return
	}
	setTotal(w, total)
	writeJSON(w, http.StatusOK, ms)
}

// createMilestone 创建里程碑。
//
//	@Summary     创建里程碑
//	@Tags        issues
//	@Accept      json
//	@Produce     json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Param       body  body createMilestoneReq true "标题与描述"
//	@Success     201 {object} store.Milestone
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/milestones [post]
func (a *API) createMilestone(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireRole(w, r, "triage")
	if !ok {
		return
	}
	var in struct {
		Title       string `json:"title"`
		Description string `json:"description"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	in.Title = strings.TrimSpace(in.Title)
	if tooLong(w, "title", in.Title, maxTitleRunes) || tooLong(w, "description", in.Description, maxBodyRunes) {
		return
	}
	if in.Title == "" {
		writeCode(w, http.StatusBadRequest, "milestone_title_required", "milestone title is required")
		return
	}
	m, err := a.store.CreateMilestone(owner, name, in.Title, strings.TrimSpace(in.Description))
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, m)
}

// updateMilestone 更新里程碑。
//
//	@Summary     更新里程碑
//	@Tags        issues
//	@Accept      json
//	@Produce     json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Param       id    path int    true "里程碑 ID"
//	@Param       body  body updateMilestoneReq true "标题、描述与状态"
//	@Success     200 {object} store.Milestone
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/milestones/{id} [patch]
func (a *API) updateMilestone(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireRole(w, r, "triage")
	if !ok {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeCode(w, http.StatusBadRequest, "invalid_id", "invalid id")
		return
	}
	var in struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		State       string `json:"state"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	in.State = strings.TrimSpace(in.State)
	if in.State != "" && in.State != "open" && in.State != "closed" {
		writeCode(w, http.StatusBadRequest, "invalid_state", "state must be open or closed")
		return
	}
	if tooLong(w, "title", in.Title, maxTitleRunes) || tooLong(w, "description", in.Description, maxBodyRunes) {
		return
	}
	m, err := a.store.UpdateMilestone(owner, name, id, strings.TrimSpace(in.Title), strings.TrimSpace(in.Description), in.State)
	if errors.Is(err, store.ErrNotFound) {
		writeCode(w, http.StatusNotFound, "milestone_not_found", "milestone not found")
		return
	}
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, m)
}

// deleteMilestone 删除里程碑。
//
//	@Summary     删除里程碑
//	@Tags        issues
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Param       id    path int    true "里程碑 ID"
//	@Success     204 {object} nil
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/milestones/{id} [delete]
func (a *API) deleteMilestone(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireRole(w, r, "triage")
	if !ok {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeCode(w, http.StatusBadRequest, "invalid_id", "invalid id")
		return
	}
	if errors.Is(a.store.DeleteMilestone(owner, name, id), store.ErrNotFound) {
		writeCode(w, http.StatusNotFound, "milestone_not_found", "milestone not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- collaborators ----

// requireOwner 仅仓库所有者可访问（管理协作者等）。
