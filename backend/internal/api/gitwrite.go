package api

import (
	"errors"
	"gitdash/backend/internal/gitsvc"
	"gitdash/backend/internal/webhooks"
	"net/http"
	"strings"
)

// ---- git refs & write commit ----

// listTags 列出仓库标签。
//
//	@Summary     列出标签
//	@Tags        repos
//	@Produce     json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Success     200 {array} gitsvc.Tag
//	@Failure     500 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/tags [get]
func (a *API) listTags(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, false)
	if !ok {
		return
	}
	tags, err := gitsvc.Tags(owner, name)
	if err != nil {
		internalError(w, err)
		return
	}
	notes, _ := a.store.RefNotes(owner, name)
	out := make([]gitsvc.Tag, 0, len(tags))
	for _, tg := range tags {
		tg.Note = notes["tag/"+tg.Name]
		out = append(out, tg)
	}
	total := len(out)
	limit, offset := pageParams(r)
	setTotal(w, total)
	writeJSON(w, http.StatusOK, pageSlice(out, limit, offset))
}

// createRef 创建分支或标签。
//
//	@Summary     创建分支/标签
//	@Description type 必须为 branch 或 tag；from 缺省为 HEAD。
//	@Tags        repos
//	@Accept      json
//	@Produce     json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Param       body  body createRefReq true "type、name、from（可选）"
//	@Success     201 {object} map[string]any "type、name 与 sha"
//	@Failure     400 {object} map[string]string
//	@Failure     409 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/refs [post]
func (a *API) createRef(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireRole(w, r, "write")
	if !ok {
		return
	}
	var in createRefReq
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	if in.Type != "branch" && in.Type != "tag" {
		writeCode(w, http.StatusBadRequest, "invalid_ref_type", "type must be 'branch' or 'tag'")
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	in.From = strings.TrimSpace(in.From)
	if in.Name == "" {
		writeCode(w, http.StatusBadRequest, "invalid_ref_name", "ref name is required")
		return
	}
	if in.From == "" {
		in.From = "HEAD"
	}
	sha, err := gitsvc.CreateRef(owner, name, in.Type, in.Name, in.From)
	if errors.Is(err, gitsvc.ErrRefExists) {
		writeCode(w, http.StatusConflict, "ref_exists", in.Type+" already exists: "+in.Name)
		return
	}
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_ref_name", err.Error())
		return
	}
	full := "refs/heads/" + in.Name
	if in.Type == "tag" {
		full = "refs/tags/" + in.Name
	}
	a.emitWebhook(webhooks.Event{
		Event: "create", Owner: owner, Repo: name, Kind: in.Type,
		Ref: full, Actor: userFrom(r), Title: in.Name,
	})
	writeJSON(w, http.StatusCreated, map[string]any{"type": in.Type, "name": in.Name, "sha": sha})
}

// deleteRef 删除分支或标签。
//
//	@Summary     删除分支/标签
//	@Description 不能删除默认（HEAD）分支。
//	@Tags        repos
//	@Param       owner   path string true "仓库所有者"
//	@Param       name    path string true "仓库名"
//	@Param       kind    path string true "类型：branches 或 tags"
//	@Param       refname path string true "分支/标签名"
//	@Success     204 {string} string ""
//	@Failure     400 {object} map[string]string
//	@Failure     404 {object} map[string]string
//	@Failure     409 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/refs/{kind}/{refname} [delete]
func (a *API) deleteRef(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireRole(w, r, "write")
	if !ok {
		return
	}
	kind := r.PathValue("kind")
	refName := r.PathValue("refname")
	// 分支保护：被保护的分支禁止通过 web API 删除
	if kind == "branches" {
		if _, prot := a.branchProtectionGuard(owner, name, refName); prot {
			writeCode(w, http.StatusConflict, "branch_protected",
				"branch is protected: deletion is not allowed")
			return
		}
	}
	err := gitsvc.DeleteRef(owner, name, kind, refName)
	if errors.Is(err, gitsvc.ErrHeadBranch) {
		writeCode(w, http.StatusConflict, "branch_is_head", "cannot delete the default (HEAD) branch")
		return
	}
	if errors.Is(err, gitsvc.ErrRefNotFound) {
		writeCode(w, http.StatusNotFound, "ref_not_found", kind+" not found")
		return
	}
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_ref_name", err.Error())
		return
	}
	// 删除引用的同时清理其备注（兼容 kind 传 branches/tags 的单复数写法）。
	noteKind := kind
	switch noteKind {
	case "branches":
		noteKind = "branch"
	case "tags":
		noteKind = "tag"
	}
	_ = a.store.DeleteRefNote(owner, name, noteKind, refName)
	full := "refs/heads/" + refName
	if kind == "tag" {
		full = "refs/tags/" + refName
	}
	a.emitWebhook(webhooks.Event{
		Event: "delete", Owner: owner, Repo: name, Kind: kind,
		Ref: full, Actor: userFrom(r), Title: refName,
	})
	w.WriteHeader(http.StatusNoContent)
}

// setRefNote 设置分支/标签备注。
//
//	@Summary     设置引用备注
//	@Description note 为空时清除备注；备注与 git refs 分离存储，不影响仓库内容。
//	@Tags        repos
//	@Accept      json
//	@Produce     json
//	@Param       owner   path string true "仓库所有者"
//	@Param       name    path string true "仓库名"
//	@Param       kind    path string true "类型：branch 或 tag"
//	@Param       refname path string true "分支/标签名"
//	@Param       body    body setRefNoteReq true "note 备注内容"
//	@Success     200 {object} map[string]any "kind、name 与 note"
//	@Failure     400 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/refs/{kind}/{refname}/note [put]
func (a *API) setRefNote(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireRole(w, r, "write")
	if !ok {
		return
	}
	kind := r.PathValue("kind")
	switch kind {
	case "branches":
		kind = "branch"
	case "tags":
		kind = "tag"
	}
	if kind != "branch" && kind != "tag" {
		writeCode(w, http.StatusBadRequest, "invalid_ref_kind", "kind must be 'branch' or 'tag'")
		return
	}
	refName := r.PathValue("refname")
	// 引用必须存在，避免为拼写错误的分支/标签留下孤儿备注。
	full := "refs/heads/" + refName
	if kind == "tag" {
		full = "refs/tags/" + refName
	}
	if _, err := gitsvc.RevSHA(owner, name, full); err != nil {
		writeCode(w, http.StatusNotFound, "ref_not_found", kind+" not found")
		return
	}
	var in setRefNoteReq
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	in.Note = strings.TrimSpace(in.Note)
	if tooLong(w, "note", in.Note, maxBodyRunes) {
		return
	}
	if err := a.store.SetRefNote(owner, name, kind, refName, in.Note); err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"kind": kind, "name": refName, "note": in.Note})
}

// writeCommit 写入一次提交。
//
//	@Summary     创建提交
//	@Description 支持批量文件变更（create/update/delete/delete_tree/move），总内容不超过 2MB。move 通过 from 指定原路径。
//	@Tags        repos
//	@Accept      json
//	@Produce     json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Param       body  body writeCommitReq true "branch（默认 main）、message 与 changes"
//	@Success     201 {object} map[string]any "sha、branch 与 message"
//	@Failure     400 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/commits [post]
func (a *API) writeCommit(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireRole(w, r, "write")
	if !ok {
		return
	}
	var in writeCommitReq
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	in.Branch = strings.TrimSpace(in.Branch)
	in.Message = strings.TrimSpace(in.Message)
	if tooLong(w, "message", in.Message, maxBodyRunes) {
		return
	}
	if in.Branch == "" {
		repo, rerr := a.store.GetRepo(owner, name)
		if rerr != nil {
			internalError(w, rerr)
			return
		}
		in.Branch = repo.DefaultBranch
		if in.Branch == "" {
			in.Branch = "main"
		}
	}
	if in.Message == "" {
		writeCode(w, http.StatusBadRequest, "message_required", "commit message is required")
		return
	}
	if len(in.Changes) == 0 {
		writeCode(w, http.StatusBadRequest, "no_changes", "no file changes provided")
		return
	}
	if len(in.Changes) > 100 {
		writeCode(w, http.StatusBadRequest, "too_many_changes", "too many changes (max 100)")
		return
	}
	total := 0
	for i := range in.Changes {
		c := &in.Changes[i]
		c.Path = strings.TrimSpace(c.Path)
		c.Path = strings.TrimPrefix(c.Path, "/")
		if _, err := gitsvc.CleanPath(c.Path); err != nil {
			writeCode(w, http.StatusBadRequest, "invalid_path", err.Error())
			return
		}
		c.From = strings.TrimSpace(c.From)
		c.From = strings.TrimPrefix(c.From, "/")
		if c.Action == "" {
			c.Action = "update"
		}
		switch c.Action {
		case "create", "update", "delete", "delete_tree":
		case "move":
			if c.From == "" {
				writeCode(w, http.StatusBadRequest, "from_required", "move requires a source path")
				return
			}
			if _, err := gitsvc.CleanPath(c.From); err != nil {
				writeCode(w, http.StatusBadRequest, "invalid_from", err.Error())
				return
			}
		default:
			writeCode(w, http.StatusBadRequest, "invalid_action", "action must be create/update/delete/delete_tree/move")
			return
		}
		total += len(c.Content)
	}
	if total > 2<<20 { // 2MB
		writeCode(w, http.StatusBadRequest, "content_too_large", "content too large (max 2MB per commit)")
		return
	}
	sha, err := gitsvc.WriteCommit(owner, name, in.Branch, in.Message, userFrom(r), in.Changes)
	if err != nil {
		writeCode(w, http.StatusBadRequest, "commit_failed", err.Error())
		return
	}
	a.maybeAnalyzeLanguages(owner, name, in.Branch, sha)
	writeJSON(w, http.StatusCreated, map[string]any{"sha": sha, "branch": in.Branch, "message": in.Message})
}

// revertCommit 撤销某次提交：在目标分支上新建一个反向提交（git revert）。
//
//	@Summary     撤销提交
//	@Description 在 branch 上创建撤销 sha 变更的新提交；merge 提交按第一父提交撤销；冲突时返回 409。
//	@Tags        repos
//	@Accept      json
//	@Produce     json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Param       sha   path string true "commit SHA"
//	@Param       body  body revertCommitReq true "branch 与可选 message"
//	@Success     201 {object} map[string]any "sha、branch 与 message"
//	@Failure     400 {object} map[string]string
//	@Failure     409 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/commits/{sha}/revert [post]
func (a *API) revertCommit(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireRole(w, r, "write")
	if !ok {
		return
	}
	sha := r.PathValue("sha")
	if !shaRe.MatchString(sha) {
		writeCode(w, http.StatusBadRequest, "invalid_sha", "invalid commit sha")
		return
	}
	var in revertCommitReq
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	in.Branch = strings.TrimSpace(in.Branch)
	in.Message = strings.TrimSpace(in.Message)
	if tooLong(w, "message", in.Message, maxBodyRunes) {
		return
	}
	if in.Branch == "" {
		writeCode(w, http.StatusBadRequest, "branch_required", "branch is required")
		return
	}
	newSha, err := gitsvc.RevertCommit(owner, name, in.Branch, sha, in.Message, userFrom(r))
	if err != nil {
		msg := err.Error()
		code := http.StatusBadRequest
		codeStr := "revert_failed"
		if strings.Contains(msg, "conflict") {
			code = http.StatusConflict
			codeStr = "revert_conflict"
		}
		writeCode(w, code, codeStr, msg)
		return
	}
	a.maybeAnalyzeLanguages(owner, name, in.Branch, newSha)
	writeJSON(w, http.StatusCreated, map[string]any{"sha": newSha, "branch": in.Branch})
}

// compareRefs 对比仓库内任意两个引用（分支 / 标签 / 提交）的差异。
//
//	@Summary     比较引用
//	@Description 返回 base..head 之间的文件统计与统一 diff；base / head 可为分支名、标签或 commit SHA。
//	@Tags        repos
//	@Produce     json
//	@Param       owner path  string true  "仓库所有者"
//	@Param       name  path  string true  "仓库名"
//	@Param       base  query string true  "基准引用"
//	@Param       head  query string true  "目标引用"
//	@Success     200 {object} map[string]any "base、head、base_sha、head_sha、files 与 patch"
//	@Failure     400 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/compare [get]
func (a *API) compareRefs(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, false)
	if !ok {
		return
	}
	base := strings.TrimSpace(r.URL.Query().Get("base"))
	head := strings.TrimSpace(r.URL.Query().Get("head"))
	if base == "" || head == "" {
		writeCode(w, http.StatusBadRequest, "missing_ref", "base and head are required")
		return
	}
	baseSHA, err := gitsvc.RevSHA(owner, name, base)
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_base", "base ref cannot be resolved")
		return
	}
	headSHA, err := gitsvc.RevSHA(owner, name, head)
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_head", "head ref cannot be resolved")
		return
	}
	files, err := gitsvc.DiffStats(owner, name, baseSHA, headSHA)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	patch, _ := gitsvc.DiffPatch(owner, name, baseSHA, headSHA)
	writeJSON(w, http.StatusOK, map[string]any{
		"base":     base,
		"head":     head,
		"base_sha": baseSHA,
		"head_sha": headSHA,
		"files":    files,
		"patch":    patch,
	})
}

// commitDiff 查看提交差异。
//
//	@Summary     提交 diff
//	@Tags        repos
//	@Produce     json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Param       sha   path string true "commit SHA"
//	@Success     200 {object} map[string]any "files 与 patch"
//	@Failure     400 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/commits/{sha}/diff [get]
func (a *API) commitDiff(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, false)
	if !ok {
		return
	}
	sha := r.PathValue("sha")
	if !shaRe.MatchString(sha) {
		writeCode(w, http.StatusBadRequest, "invalid_sha", "invalid commit sha")
		return
	}
	files, patch, err := gitsvc.CommitDiff(owner, name, sha)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"files": files, "patch": patch})
}

// commitInfo 查看单个提交的元数据。
//
//	@Summary     提交详情
//	@Description 返回单个提交的 sha / author / date / message / parents / refs，供 blame 深链跳转。
//	@Tags        repos
//	@Produce     json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Param       sha   path string true "commit SHA"
//	@Success     200 {object} gitsvc.Commit
//	@Failure     400 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/commits/{sha} [get]
func (a *API) commitInfo(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, false)
	if !ok {
		return
	}
	sha := r.PathValue("sha")
	if !shaRe.MatchString(sha) {
		writeCode(w, http.StatusBadRequest, "invalid_sha", "invalid commit sha")
		return
	}
	c, err := gitsvc.CommitInfo(owner, name, sha)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, c)
}
