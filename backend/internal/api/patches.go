package api

import (
	"io"
	"net/http"
	"strings"

	"gitdash/backend/internal/gitsvc"
)

// maxPatchBytes 单次提交补丁的最大体积（20 MiB）。
const maxPatchBytes = 20 << 20

// patchSubjects 从 mbox 文本中提取每封补丁的 Subject 行（去掉 [PATCH ...] 前缀）。
func patchSubjects(data string) []string {
	var out []string
	for _, ln := range strings.Split(data, "\n") {
		ln = strings.TrimRight(ln, "\r")
		if !strings.HasPrefix(ln, "Subject:") {
			continue
		}
		s := strings.TrimSpace(strings.TrimPrefix(ln, "Subject:"))
		// 反复去掉开头的 Re:/Fwd: 与 [PATCH ...] 等方括号标签
		for {
			changed := false
			for _, p := range []string{"Re:", "Fwd:"} {
				if strings.HasPrefix(s, p) {
					s = strings.TrimSpace(strings.TrimPrefix(s, p))
					changed = true
				}
			}
			if strings.HasPrefix(s, "[") {
				if end := strings.IndexByte(s, ']'); end >= 0 {
					s = strings.TrimSpace(s[end+1:])
					changed = true
				}
			}
			if !changed {
				break
			}
		}
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

// receivePatches 接收 git format-patch / git send-email 产出的 mbox，应用到
// 目标分支的新分支上并自动创建 PR。
//
//	@Summary     通过补丁邮件创建 PR
//	@Description 提交 `git format-patch` / `git send-email` 风格的 mbox（text/plain）。
//	@Description 服务端 `git am --3way` 到新分支 patches/<ts> 后自动开 PR。
//	@Tags        pulls
//	@Accept      text/plain
//	@Produce     json
//	@Param       owner  path  string true  "仓库所有者"
//	@Param       name   path  string true  "仓库名"
//	@Param       target query string false "目标分支（默认仓库默认分支）"
//	@Param       title  query string false "PR 标题（默认取补丁首个 Subject）"
//	@Success     201 {object} store.PullRequest
//	@Failure     400 {object} map[string]string
//	@Failure     404 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/patches [post]
func (a *API) receivePatches(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, true)
	if !ok {
		return
	}
	target := strings.TrimSpace(strings.TrimPrefix(r.URL.Query().Get("target"), "refs/heads/"))
	if target == "" {
		info, err := a.store.GetRepo(owner, name)
		if err != nil {
			writeNotFound(w, "repo")
			return
		}
		target = info.DefaultBranch
		if target == "" {
			target = "main"
		}
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxPatchBytes))
	if err != nil {
		writeCode(w, http.StatusBadRequest, "patch_too_large", "patch series too large")
		return
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		writeCode(w, http.StatusBadRequest, "empty_patch", "patch series is empty")
		return
	}
	subjects := patchSubjects(string(data))
	title := strings.TrimSpace(r.URL.Query().Get("title"))
	if title == "" && len(subjects) > 0 {
		title = subjects[0]
	}
	if title == "" {
		title = "Patch series"
	}
	branch, sha, err := gitsvc.ApplyPatchSeries(owner, name, target, data)
	if err != nil {
		writeCode(w, http.StatusBadRequest, "patch_failed", err.Error())
		return
	}
	baseSHA, err := gitsvc.RevSHA(owner, name, "refs/heads/"+target)
	if err != nil {
		writeCode(w, http.StatusBadRequest, "branch_not_found", "target branch not found: "+target)
		return
	}
	body := ""
	if len(subjects) > 0 {
		body = "Patch series submitted via `git send-email`:\n\n"
		for _, s := range subjects {
			body += "- " + s + "\n"
		}
	}
	pr, err := a.store.CreatePull(owner, name, userFrom(r), title, body, branch, target, baseSHA, sha, false)
	if err != nil {
		internalError(w, err)
		return
	}
	a.notify(owner, name, "pull", "opened", userFrom(r), pr.Number, pr.Title, "")
	writeJSON(w, http.StatusCreated, pr)
}
