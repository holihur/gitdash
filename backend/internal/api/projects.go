package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"gitdash/backend/internal/store"
)

// ---- projects ----

// listProjects 列出仓库的看板项目。
//
//	@Summary     列出看板项目
//	@Tags        projects
//	@Produce     json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Success     200 {array} store.Project
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/projects [get]
func (a *API) listProjects(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, false)
	if !ok {
		return
	}
	ps, err := a.store.ListProjects(owner, name)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ps)
}

// createProject 创建看板项目（自动初始化默认列与默认泳道）。
//
//	@Summary     创建看板项目
//	@Tags        projects
//	@Accept      json
//	@Produce     json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Param       body  body createProjectReq true "名称与描述"
//	@Success     201 {object} store.Project
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/projects [post]
func (a *API) createProject(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, true)
	if !ok {
		return
	}
	var in struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		writeCode(w, http.StatusBadRequest, "project_name_required", "project name is required")
		return
	}
	p, err := a.store.CreateProject(owner, name, in.Name, strings.TrimSpace(in.Description))
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

// updateProject 更新看板项目。
//
//	@Summary     更新看板项目
//	@Tags        projects
//	@Accept      json
//	@Produce     json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Param       id    path int    true "项目 ID"
//	@Param       body  body updateProjectReq true "名称与描述"
//	@Success     200 {object} store.Project
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/projects/{id} [patch]
func (a *API) updateProject(w http.ResponseWriter, r *http.Request) {
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
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	p, err := a.store.UpdateProject(owner, name, id, strings.TrimSpace(in.Name), strings.TrimSpace(in.Description))
	if errors.Is(err, store.ErrNotFound) {
		writeCode(w, http.StatusNotFound, "project_not_found", "project not found")
		return
	}
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// deleteProject 删除看板项目（级联删除列、泳道、卡片）。
//
//	@Summary     删除看板项目
//	@Tags        projects
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Param       id    path int    true "项目 ID"
//	@Success     204 {object} nil
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/projects/{id} [delete]
func (a *API) deleteProject(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, true)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeCode(w, http.StatusBadRequest, "invalid_id", "invalid id")
		return
	}
	if errors.Is(a.store.DeleteProject(owner, name, id), store.ErrNotFound) {
		writeCode(w, http.StatusNotFound, "project_not_found", "project not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// getProjectBoard 一次性返回项目的列、泳道与卡片（看板渲染数据）。
//
//	@Summary     获取看板数据
//	@Tags        projects
//	@Produce     json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Param       id    path int    true "项目 ID"
//	@Success     200 {object} boardResp
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/projects/{id}/board [get]
func (a *API) getProjectBoard(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, false)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeCode(w, http.StatusBadRequest, "invalid_id", "invalid id")
		return
	}
	if _, err := a.store.GetProject(owner, name, id); errors.Is(err, store.ErrNotFound) {
		writeCode(w, http.StatusNotFound, "project_not_found", "project not found")
		return
	} else if err != nil {
		internalError(w, err)
		return
	}
	cols, err := a.store.ListProjectColumns(id)
	if err != nil {
		internalError(w, err)
		return
	}
	lanes, err := a.store.ListProjectSwimlanes(id)
	if err != nil {
		internalError(w, err)
		return
	}
	cards, err := a.store.ListProjectCards(id)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, boardResp{Project: mustProject(a, owner, name, id), Columns: cols, Swimlanes: lanes, Cards: cards})
}

func mustProject(a *API, owner, repo string, id int64) store.Project {
	p, _ := a.store.GetProject(owner, repo, id)
	return p
}

type boardResp struct {
	Project   store.Project           `json:"project"`
	Columns   []store.ProjectColumn   `json:"columns"`
	Swimlanes []store.ProjectSwimlane `json:"swimlanes"`
	Cards     []store.ProjectCard     `json:"cards"`
}

type createProjectReq struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type updateProjectReq struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// ---- columns ----

// listProjectColumns 列出项目列。
//
//	@Summary     列出看板列
//	@Tags        projects
//	@Produce     json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Param       id    path int    true "项目 ID"
//	@Success     200 {array} store.ProjectColumn
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/projects/{id}/columns [get]
func (a *API) listProjectColumns(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, false)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeCode(w, http.StatusBadRequest, "invalid_id", "invalid id")
		return
	}
	if _, err := a.store.GetProject(owner, name, id); errors.Is(err, store.ErrNotFound) {
		writeCode(w, http.StatusNotFound, "project_not_found", "project not found")
		return
	} else if err != nil {
		internalError(w, err)
		return
	}
	cols, err := a.store.ListProjectColumns(id)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, cols)
}

// createProjectColumn 创建看板列。
//
//	@Summary     创建看板列
//	@Tags        projects
//	@Accept      json
//	@Produce     json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Param       id    path int    true "项目 ID"
//	@Param       body  body createProjectColumnReq true "列名"
//	@Success     201 {object} store.ProjectColumn
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/projects/{id}/columns [post]
func (a *API) createProjectColumn(w http.ResponseWriter, r *http.Request) {
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
		Name string `json:"name"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		writeCode(w, http.StatusBadRequest, "column_name_required", "column name is required")
		return
	}
	col, err := a.store.CreateProjectColumn(owner, name, id, in.Name)
	if errors.Is(err, store.ErrNotFound) {
		writeCode(w, http.StatusNotFound, "project_not_found", "project not found")
		return
	}
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, col)
}

// updateProjectColumn 重命名 / 排序看板列。
//
//	@Summary     更新看板列
//	@Tags        projects
//	@Accept      json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Param       id    path int    true "项目 ID"
//	@Param       cid   path int    true "列 ID"
//	@Param       body  body updateProjectColumnReq true "列名与顺序"
//	@Success     204 {object} nil
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/projects/{id}/columns/{cid} [patch]
func (a *API) updateProjectColumn(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, true)
	if !ok {
		return
	}
	pid, cid, ok2 := projectPathIDs(w, r)
	if !ok2 || !a.checkProjectExists(w, owner, name, pid) {
		return
	}
	var in struct {
		Name     string `json:"name"`
		Position *int   `json:"position"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	if err := a.store.UpdateProjectColumn(pid, cid, strings.TrimSpace(in.Name), in.Position); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeCode(w, http.StatusNotFound, "column_not_found", "column not found")
			return
		}
		internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// deleteProjectColumn 删除看板列（其卡片一并删除）。
//
//	@Summary     删除看板列
//	@Tags        projects
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Param       id    path int    true "项目 ID"
//	@Param       cid   path int    true "列 ID"
//	@Success     204 {object} nil
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/projects/{id}/columns/{cid} [delete]
func (a *API) deleteProjectColumn(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, true)
	if !ok {
		return
	}
	pid, cid, ok2 := projectPathIDs(w, r)
	if !ok2 || !a.checkProjectExists(w, owner, name, pid) {
		return
	}
	if errors.Is(a.store.DeleteProjectColumn(pid, cid), store.ErrNotFound) {
		writeCode(w, http.StatusNotFound, "column_not_found", "column not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- swimlanes ----

// listProjectSwimlanes 列出项目泳道。
//
//	@Summary     列出看板泳道
//	@Tags        projects
//	@Produce     json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Param       id    path int    true "项目 ID"
//	@Success     200 {array} store.ProjectSwimlane
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/projects/{id}/swimlanes [get]
func (a *API) listProjectSwimlanes(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, false)
	if !ok {
		return
	}
	pid, ok2 := projectOnly(w, r)
	if !ok2 || !a.checkProjectExists(w, owner, name, pid) {
		return
	}
	lanes, err := a.store.ListProjectSwimlanes(pid)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, lanes)
}

// createProjectSwimlane 创建泳道。
//
//	@Summary     创建看板泳道
//	@Tags        projects
//	@Accept      json
//	@Produce     json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Param       id    path int    true "项目 ID"
//	@Param       body  body createProjectSwimlaneReq true "泳道名"
//	@Success     201 {object} store.ProjectSwimlane
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/projects/{id}/swimlanes [post]
func (a *API) createProjectSwimlane(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, true)
	if !ok {
		return
	}
	pid, ok2 := projectOnly(w, r)
	if !ok2 {
		return
	}
	var in struct {
		Name string `json:"name"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		writeCode(w, http.StatusBadRequest, "swimlane_name_required", "swimlane name is required")
		return
	}
	lane, err := a.store.CreateProjectSwimlane(owner, name, pid, in.Name)
	if errors.Is(err, store.ErrNotFound) {
		writeCode(w, http.StatusNotFound, "project_not_found", "project not found")
		return
	}
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, lane)
}

// updateProjectSwimlane 重命名 / 排序泳道。
//
//	@Summary     更新看板泳道
//	@Tags        projects
//	@Accept      json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Param       id    path int    true "项目 ID"
//	@Param       lid   path int    true "泳道 ID"
//	@Param       body  body updateProjectSwimlaneReq true "泳道名与顺序"
//	@Success     204 {object} nil
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/projects/{id}/swimlanes/{lid} [patch]
func (a *API) updateProjectSwimlane(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, true)
	if !ok {
		return
	}
	pid, lid, ok2 := projectPathIDs(w, r)
	if !ok2 || !a.checkProjectExists(w, owner, name, pid) {
		return
	}
	var in struct {
		Name     string `json:"name"`
		Position *int   `json:"position"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	if err := a.store.UpdateProjectSwimlane(pid, lid, strings.TrimSpace(in.Name), in.Position); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeCode(w, http.StatusNotFound, "swimlane_not_found", "swimlane not found")
			return
		}
		internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// deleteProjectSwimlane 删除泳道（卡片退回未分组）。
//
//	@Summary     删除看板泳道
//	@Tags        projects
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Param       id    path int    true "项目 ID"
//	@Param       lid   path int    true "泳道 ID"
//	@Success     204 {object} nil
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/projects/{id}/swimlanes/{lid} [delete]
func (a *API) deleteProjectSwimlane(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, true)
	if !ok {
		return
	}
	pid, lid, ok2 := projectPathIDs(w, r)
	if !ok2 || !a.checkProjectExists(w, owner, name, pid) {
		return
	}
	if errors.Is(a.store.DeleteProjectSwimlane(pid, lid), store.ErrNotFound) {
		writeCode(w, http.StatusNotFound, "swimlane_not_found", "swimlane not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- cards ----

// listProjectCards 列出项目卡片（含 issue 标题与状态）。
//
//	@Summary     列出看板卡片
//	@Tags        projects
//	@Produce     json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Param       id    path int    true "项目 ID"
//	@Success     200 {array} store.ProjectCard
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/projects/{id}/cards [get]
func (a *API) listProjectCards(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, false)
	if !ok {
		return
	}
	pid, ok2 := projectOnly(w, r)
	if !ok2 || !a.checkProjectExists(w, owner, name, pid) {
		return
	}
	cards, err := a.store.ListProjectCards(pid)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, cards)
}

// createProjectCard 创建卡片（issue 卡片或文本卡片）。
//
//	@Summary     创建看板卡片
//	@Tags        projects
//	@Accept      json
//	@Produce     json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Param       id    path int    true "项目 ID"
//	@Param       body  body createProjectCardReq true "列 / 泳道 / issue / 文本"
//	@Success     201 {object} store.ProjectCard
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/projects/{id}/cards [post]
func (a *API) createProjectCard(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, true)
	if !ok {
		return
	}
	pid, ok2 := projectOnly(w, r)
	if !ok2 {
		return
	}
	var in struct {
		ColumnID    int64  `json:"column_id"`
		SwimlaneID  int64  `json:"swimlane_id"`
		IssueNumber int64  `json:"issue_number"`
		Note        string `json:"note"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	if in.ColumnID < 1 {
		writeCode(w, http.StatusBadRequest, "column_required", "column_id is required")
		return
	}
	if in.IssueNumber == 0 && strings.TrimSpace(in.Note) == "" {
		writeCode(w, http.StatusBadRequest, "card_content_required", "issue_number or note is required")
		return
	}
	c, err := a.store.CreateProjectCard(owner, name, pid, in.ColumnID, in.SwimlaneID, in.IssueNumber, strings.TrimSpace(in.Note))
	if errors.Is(err, store.ErrNotFound) {
		writeCode(w, http.StatusBadRequest, "invalid_card_target", "column, swimlane or issue not found")
		return
	}
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

// updateProjectCard 移动卡片（换列 / 换泳道 / 排序）或编辑文本。
//
//	@Summary     更新看板卡片
//	@Tags        projects
//	@Accept      json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Param       id    path int    true "项目 ID"
//	@Param       card  path int    true "卡片 ID"
//	@Param       body  body updateProjectCardReq true "移动 / 编辑内容"
//	@Success     200 {object} store.ProjectCard
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/projects/{id}/cards/{card} [patch]
func (a *API) updateProjectCard(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, true)
	if !ok {
		return
	}
	pid, cid, ok2 := projectCardPathIDs(w, r)
	if !ok2 || !a.checkProjectExists(w, owner, name, pid) {
		return
	}
	var in struct {
		ColumnID   *int64  `json:"column_id"`
		SwimlaneID *int64  `json:"swimlane_id"`
		Position   *int    `json:"position"`
		Note       *string `json:"note"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	if in.ColumnID != nil {
		pos := 0
		if in.Position != nil {
			pos = *in.Position
		}
		lane := int64(0)
		if in.SwimlaneID != nil {
			lane = *in.SwimlaneID
		}
		if err := a.store.MoveProjectCard(pid, cid, *in.ColumnID, lane, pos); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeCode(w, http.StatusBadRequest, "invalid_move_target", "column, swimlane or card not found")
				return
			}
			internalError(w, err)
			return
		}
	}
	if in.Note != nil {
		if err := a.store.UpdateProjectCard(pid, cid, strings.TrimSpace(*in.Note)); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeCode(w, http.StatusNotFound, "card_not_found", "card not found")
				return
			}
			internalError(w, err)
			return
		}
	}
	cards, err := a.store.ListProjectCards(pid)
	if err != nil {
		internalError(w, err)
		return
	}
	for _, c := range cards {
		if c.ID == cid {
			writeJSON(w, http.StatusOK, c)
			return
		}
	}
	writeCode(w, http.StatusNotFound, "card_not_found", "card not found")
}

// deleteProjectCard 删除卡片。
//
//	@Summary     删除看板卡片
//	@Tags        projects
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Param       id    path int    true "项目 ID"
//	@Param       card  path int    true "卡片 ID"
//	@Success     204 {object} nil
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/projects/{id}/cards/{card} [delete]
func (a *API) deleteProjectCard(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, true)
	if !ok {
		return
	}
	pid, cid, ok2 := projectCardPathIDs(w, r)
	if !ok2 || !a.checkProjectExists(w, owner, name, pid) {
		return
	}
	if errors.Is(a.store.DeleteProjectCard(pid, cid), store.ErrNotFound) {
		writeCode(w, http.StatusNotFound, "card_not_found", "card not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- helpers ----

func projectPathIDs(w http.ResponseWriter, r *http.Request) (int64, int64, bool) {
	return twoPathIDs(w, r, "id", "cid", "lid")
}

// projectOnly 解析仅含项目 ID 的路径。
func projectOnly(w http.ResponseWriter, r *http.Request) (int64, bool) {
	pid, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || pid < 1 {
		writeCode(w, http.StatusBadRequest, "invalid_id", "invalid id")
		return 0, false
	}
	return pid, true
}

func projectCardPathIDs(w http.ResponseWriter, r *http.Request) (int64, int64, bool) {
	return twoPathIDs(w, r, "id", "card", "card")
}

func twoPathIDs(w http.ResponseWriter, r *http.Request, primary, aName, bName string) (int64, int64, bool) {
	pid, err := strconv.ParseInt(r.PathValue(primary), 10, 64)
	if err != nil || pid < 1 {
		writeCode(w, http.StatusBadRequest, "invalid_id", "invalid id")
		return 0, 0, false
	}
	secStr := r.PathValue(aName)
	if secStr == "" {
		secStr = r.PathValue(bName)
	}
	sec, err := strconv.ParseInt(secStr, 10, 64)
	if err != nil || sec < 1 {
		writeCode(w, http.StatusBadRequest, "invalid_id", "invalid id")
		return 0, 0, false
	}
	return pid, sec, true
}

func (a *API) checkProjectExists(w http.ResponseWriter, owner, repo string, id int64) bool {
	if _, err := a.store.GetProject(owner, repo, id); errors.Is(err, store.ErrNotFound) {
		writeCode(w, http.StatusNotFound, "project_not_found", "project not found")
		return false
	} else if err != nil {
		internalError(w, err)
		return false
	}
	return true
}

type createProjectColumnReq struct {
	Name string `json:"name"`
}

type updateProjectColumnReq struct {
	Name     string `json:"name"`
	Position *int   `json:"position"`
}

type createProjectSwimlaneReq struct {
	Name string `json:"name"`
}

type updateProjectSwimlaneReq struct {
	Name     string `json:"name"`
	Position *int   `json:"position"`
}

type createProjectCardReq struct {
	ColumnID    int64  `json:"column_id"`
	SwimlaneID  int64  `json:"swimlane_id"`
	IssueNumber int64  `json:"issue_number"`
	Note        string `json:"note"`
}

type updateProjectCardReq struct {
	ColumnID   *int64  `json:"column_id"`
	SwimlaneID *int64  `json:"swimlane_id"`
	Position   *int    `json:"position"`
	Note       *string `json:"note"`
}
