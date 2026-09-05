package api

import (
	"encoding/json"
	"errors"
	"gitdash/backend/internal/store"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

func (a *API) listIssues(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, false)
	if !ok {
		return
	}
	issues, err := a.store.ListIssues(owner, name)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, a.enrichIssues(owner, name, issues))
}

func (a *API) createIssue(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, true)
	if !ok {
		return
	}
	var in struct {
		Title string `json:"title"`
		Body  string `json:"body"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	title := strings.TrimSpace(in.Title)
	if title == "" {
		writeCode(w, http.StatusBadRequest, "title_required", "title is required")
		return
	}
	if len([]rune(title)) > 200 {
		writeCode(w, http.StatusBadRequest, "title_too_long", "title too long (max 200 chars)")
		return
	}
	if len([]rune(in.Body)) > 10000 {
		writeCode(w, http.StatusBadRequest, "body_too_long", "body too long (max 10000 chars)")
		return
	}
	me := userFrom(r)
	issue, err := a.store.CreateIssue(owner, name, me, title, in.Body)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.notify(owner, name, "issue", "opened", me, issue.Number, issue.Title)
	writeJSON(w, http.StatusCreated, a.enrichIssues(owner, name, []store.Issue{issue})[0])
}

func (a *API) setIssueState(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, true)
	if !ok {
		return
	}
	number, err := strconv.ParseInt(r.PathValue("number"), 10, 64)
	if err != nil || number < 1 {
		writeCode(w, http.StatusBadRequest, "invalid_issue_number", "invalid issue number")
		return
	}
	var in struct {
		State string `json:"state"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	if in.State != "open" && in.State != "closed" {
		writeCode(w, http.StatusBadRequest, "invalid_state", "state must be 'open' or 'closed'")
		return
	}
	// 先取当前状态，用于判断状态是否真正变化（重复 close/reopen 不发通知）
	prev, err := a.store.GetPullIssue(owner, name, number)
	if errors.Is(err, store.ErrNotFound) {
		writeCode(w, http.StatusNotFound, "issue_not_found", "issue not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	issue, err := a.store.SetIssueState(owner, name, number, in.State)
	if errors.Is(err, store.ErrNotFound) {
		writeCode(w, http.StatusNotFound, "issue_not_found", "issue not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if prev.State != issue.State {
		action := "closed"
		if issue.State == "open" {
			action = "reopened"
		}
		a.notify(owner, name, "issue", action, userFrom(r), issue.Number, issue.Title)
	}
	writeJSON(w, http.StatusOK, a.enrichIssues(owner, name, []store.Issue{issue})[0])
}

func (a *API) enrichIssues(owner, repo string, issues []store.Issue) []map[string]any {
	numbers := make([]int64, 0, len(issues))
	for _, it := range issues {
		numbers = append(numbers, it.Number)
	}
	labels, _ := a.store.IssueLabels(owner, repo, numbers)
	milestones, _ := a.store.IssueMilestones(owner, repo, numbers)
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
		out = append(out, m)
	}
	return out
}

func (a *API) parseRepoNumber(w http.ResponseWriter, r *http.Request) (string, string, int64, bool) {
	owner, name, ok := a.requireAccess(w, r, false)
	if !ok {
		return "", "", 0, false
	}
	n, err := strconv.ParseInt(r.PathValue("number"), 10, 64)
	if err != nil || n < 1 {
		writeCode(w, http.StatusBadRequest, "invalid_issue_number", "invalid issue number")
		return "", "", 0, false
	}
	return owner, name, n, true
}

func (a *API) setIssueLabels(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, true)
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
	err = a.store.SetIssueLabels(owner, name, n, in.LabelIDs)
	if errors.Is(err, store.ErrNotFound) {
		writeCode(w, http.StatusNotFound, "issue_not_found", "issue not found")
		return
	}
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_label", err.Error())
		return
	}
	issue, err := a.store.GetPullIssue(owner, name, n)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, a.enrichIssues(owner, name, []store.Issue{issue})[0])
}

func (a *API) setIssueMilestone(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, true)
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
	err = a.store.SetIssueMilestone(owner, name, n, in.MilestoneID)
	if errors.Is(err, store.ErrNotFound) {
		writeCode(w, http.StatusNotFound, "issue_not_found", "issue not found")
		return
	}
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_milestone", err.Error())
		return
	}
	issue, err := a.store.GetPullIssue(owner, name, n)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, a.enrichIssues(owner, name, []store.Issue{issue})[0])
}

func (a *API) listLabels(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, false)
	if !ok {
		return
	}
	ls, err := a.store.ListLabels(owner, name)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, ls)
}

var labelColorRe = regexp.MustCompile(`^#?[0-9a-fA-F]{6}$`)

func (a *API) createLabel(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, true)
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
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, l)
}

func (a *API) updateLabel(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, true)
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
		writeErr(w, http.StatusInternalServerError, err.Error())
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
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, upd)
}

func (a *API) deleteLabel(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, true)
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

func (a *API) listMilestones(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, false)
	if !ok {
		return
	}
	ms, err := a.store.ListMilestones(owner, name)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, ms)
}

func (a *API) createMilestone(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, true)
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
	if in.Title == "" {
		writeCode(w, http.StatusBadRequest, "milestone_title_required", "milestone title is required")
		return
	}
	m, err := a.store.CreateMilestone(owner, name, in.Title, strings.TrimSpace(in.Description))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, m)
}

func (a *API) updateMilestone(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, true)
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
	m, err := a.store.UpdateMilestone(owner, name, id, strings.TrimSpace(in.Title), strings.TrimSpace(in.Description), in.State)
	if errors.Is(err, store.ErrNotFound) {
		writeCode(w, http.StatusNotFound, "milestone_not_found", "milestone not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func (a *API) deleteMilestone(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, true)
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
