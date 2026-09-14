package api

import (
	"os"
	"strings"

	"gitdash/backend/internal/gitsvc"
	"gitdash/backend/internal/logx"
)

// profileRepoEnabled 是否为新用户 / 新组织自动创建同名仓库。
// 默认开启；GITDASH_PROFILE_REPO=0/false/no/off 可关闭（测试夹具会关闭以保证隔离）。
func profileRepoEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("GITDASH_PROFILE_REPO"))) {
	case "0", "false", "no", "off":
		return false
	}
	return true
}

// provisionProfileRepo 用户首次创建时创建同名公开仓库（<username>/<username>）并初始化 README。
func (a *API) provisionProfileRepo(username string) {
	a.provisionSameNameRepo(username, username)
}

// provisionSameNameRepo 创建 <owner>/<owner> 同名公开仓库并初始化 README。
// 用于新用户（profile repo）与新组织。watcher 为自动订阅该仓库的用户（可为空）。
// 幂等：已存在则跳过；任何失败只记录日志，不阻断调用方主流程。
func (a *API) provisionSameNameRepo(owner, watcher string) {
	if !profileRepoEnabled() || !gitsvc.ValidName(owner) {
		return
	}
	// git 服务未初始化（如未调用 gitsvc.Init）时跳过，避免在进程工作目录下建仓。
	if gitsvc.ReposDir() == "" {
		logx.Infof("same-name repo: git service not initialized; skip %s", owner)
		return
	}
	if _, err := a.store.GetRepo(owner, owner); err == nil {
		return
	}
	if _, err := a.store.CreateRepo(owner, owner, "", false); err != nil {
		logx.Infof("same-name repo: create %s/%s: %v", owner, owner, err)
		return
	}
	if watcher != "" {
		_ = a.store.WatchRepo(watcher, owner, owner)
	}
	if err := gitsvc.CreateBare(owner, owner); err != nil {
		_ = a.store.DeleteRepo(owner, owner)
		logx.Infof("same-name repo: create bare %s/%s: %v", owner, owner, err)
		return
	}
	if err := gitsvc.InitTemplate(owner, owner); err != nil {
		logx.Infof("same-name repo: init template %s/%s: %v", owner, owner, err)
	}
}
