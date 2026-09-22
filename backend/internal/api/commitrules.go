package api

import (
	"net/http"
	"strings"

	"gitdash/backend/internal/gitsvc"
)

// 仓库提交身份校验：限制 push 引入提交的作者/提交者“姓名/邮箱”格式（正则）。
// 仅仓库 owner 可配置；规则在 pre-receive 阶段执行，不符者整个 push 被拒绝。

type repoCommitRulesResp struct {
	Enabled      bool   `json:"enabled"`
	NamePattern  string `json:"name_pattern"`
	EmailPattern string `json:"email_pattern"`
}

// getRepoCommitRules 读取仓库提交身份规则（仅 owner）。
//
//	@Summary     读取提交身份规则
//	@Tags        repos
//	@Produce     json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Success     200 {object} object "enabled/name_pattern/email_pattern"
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/commit-rules [get]
func (a *API) getRepoCommitRules(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireOwner(w, r)
	if !ok {
		return
	}
	rules, found, err := a.store.GetRepoCommitRules(owner, name)
	if err != nil {
		internalError(w, err)
		return
	}
	resp := repoCommitRulesResp{}
	if found {
		resp = repoCommitRulesResp{
			Enabled:      true,
			NamePattern:  rules.NamePattern,
			EmailPattern: rules.EmailPattern,
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

// setRepoCommitRules 更新仓库提交身份规则（仅 owner）。两项均为空即关闭。
//
//	@Summary     更新提交身份规则
//	@Tags        repos
//	@Accept      json
//	@Produce     json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Param       body  body object true "name_pattern/email_pattern"
//	@Success     200 {object} object "enabled/name_pattern/email_pattern"
//	@Failure     400 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/commit-rules [put]
func (a *API) setRepoCommitRules(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireOwner(w, r)
	if !ok {
		return
	}
	var in struct {
		NamePattern  string `json:"name_pattern"`
		EmailPattern string `json:"email_pattern"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	in.NamePattern = strings.TrimSpace(in.NamePattern)
	in.EmailPattern = strings.TrimSpace(in.EmailPattern)
	if tooLong(w, "name_pattern", in.NamePattern, 512) || tooLong(w, "email_pattern", in.EmailPattern, 512) {
		return
	}
	if _, err := gitsvc.CompileCommitIdentityRule(in.NamePattern, in.EmailPattern); err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_pattern", err.Error())
		return
	}
	if in.NamePattern == "" && in.EmailPattern == "" {
		if err := a.store.DeleteRepoCommitRules(owner, name); err != nil {
			internalError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, repoCommitRulesResp{})
		return
	}
	if err := a.store.SetRepoCommitRules(owner, name, in.NamePattern, in.EmailPattern); err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, repoCommitRulesResp{
		Enabled:      true,
		NamePattern:  in.NamePattern,
		EmailPattern: in.EmailPattern,
	})
}
