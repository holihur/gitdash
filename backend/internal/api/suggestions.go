package api

import (
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"gitdash/backend/internal/gitsvc"
)

// suggestionBlockRe 匹配行内评论中的 ```suggestion fenced 代码块。
var suggestionBlockRe = regexp.MustCompile("(?ms)^[ \t]*```suggestion[ \t]*\n(.*?)^[ \t]*```[ \t]*$")

// parseSuggestion 从评论正文里提取唯一的 suggestion 代码块内容。
func parseSuggestion(body string) (string, bool) {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	matches := suggestionBlockRe.FindAllStringSubmatch(body, -1)
	if len(matches) != 1 {
		return "", false
	}
	return strings.TrimSuffix(matches[0][1], "\n"), true
}

// applySuggestion 应用行内评论里的 ```suggestion：在 PR 源分支上以建议内容替换被评论行并提交。
//
//	@Summary     应用代码建议
//	@Description 需要仓库写权限；在 PR 源分支上创建提交，替换被评论行，并记录应用的提交 SHA。
//	@Tags        pulls
//	@Produce     json
//	@Param       owner  path string true "仓库所有者"
//	@Param       name   path string true "仓库名"
//	@Param       number path int    true "PR 编号"
//	@Param       id     path int    true "评论 ID"
//	@Success     200 {object} map[string]any "sha 与更新后的评论"
//	@Failure     400 {object} map[string]string
//	@Failure     409 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/pulls/{number}/comments/{id}/apply [post]
func (a *API) applySuggestion(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, true)
	if !ok {
		return
	}
	pr, err := a.getPullOr404(w, owner, name, r.PathValue("number"))
	if err != nil {
		return
	}
	if pr.State != "open" {
		writeCode(w, http.StatusBadRequest, "pull_not_open", "suggestions can only be applied on open pull requests")
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeCode(w, http.StatusBadRequest, "invalid_id", "invalid comment id")
		return
	}
	comment, err := a.store.GetComment(owner, name, id)
	if err != nil || comment.Number != pr.Number || comment.Kind != "pull" {
		writeNotFound(w, "comment")
		return
	}
	if comment.SuggestionAppliedSHA != "" {
		writeCode(w, http.StatusConflict, "suggestion_applied", "suggestion already applied")
		return
	}
	if comment.FilePath == nil || comment.Line == nil || comment.LineSide != "new" {
		writeCode(w, http.StatusBadRequest, "not_inline", "suggestion requires an inline comment on the new side")
		return
	}
	replacement, ok := parseSuggestion(comment.Body)
	if !ok {
		writeCode(w, http.StatusBadRequest, "no_suggestion", "comment must contain exactly one ```suggestion block")
		return
	}
	head := pr.HeadSHA
	if h, herr := gitsvc.RevSHA(owner, name, "refs/heads/"+pr.SourceBranch); herr == nil {
		head = h
	}
	blob, err := gitsvc.ReadBlob(owner, name, head, *comment.FilePath)
	if err != nil || blob.Encoding != "utf-8" {
		writeCode(w, http.StatusBadRequest, "file_not_found", "cannot read file "+*comment.FilePath)
		return
	}
	lines := strings.Split(strings.TrimSuffix(blob.Content, "\n"), "\n")
	idx := int(*comment.Line) - 1
	if idx < 0 || idx >= len(lines) {
		writeCode(w, http.StatusBadRequest, "line_out_of_range", "commented line is out of range")
		return
	}
	replacementLines := strings.Split(replacement, "\n")
	merged := make([]string, 0, len(lines)+len(replacementLines))
	merged = append(merged, lines[:idx]...)
	merged = append(merged, replacementLines...)
	merged = append(merged, lines[idx+1:]...)
	newContent := strings.Join(merged, "\n") + "\n"

	sha, err := gitsvc.WriteCommit(owner, name, pr.SourceBranch,
		fmt.Sprintf("Apply suggestion from pull request #%d", pr.Number),
		userFrom(r),
		[]gitsvc.FileChange{{Path: *comment.FilePath, Action: "update", Content: newContent}})
	if err != nil {
		writeCode(w, http.StatusBadRequest, "commit_failed", err.Error())
		return
	}
	updated, err := a.store.MarkSuggestionApplied(owner, name, id, sha)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sha": sha, "comment": updated})
}
