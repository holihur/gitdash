package api

import (
	"log"
	"os"
	"strings"

	"gitdash/backend/internal/gitsvc"
)

// profileRepoEnabled 是否为新用户自动创建同名仓库。
// 默认开启；GITDASH_PROFILE_REPO=0/false/no/off 可关闭（测试夹具会关闭以保证隔离）。
func profileRepoEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("GITDASH_PROFILE_REPO"))) {
	case "0", "false", "no", "off":
		return false
	}
	return true
}

// provisionProfileRepo 用户首次创建时创建同名公开仓库（<username>/<username>）并初始化 README。
// 幂等：已存在则跳过；任何失败只记录日志，不阻断用户创建。
func (a *API) provisionProfileRepo(username string) {
	if !profileRepoEnabled() || !gitsvc.ValidName(username) {
		return
	}
	// git 服务未初始化（如未调用 gitsvc.Init）时跳过，避免在进程工作目录下建仓。
	if gitsvc.ReposDir() == "" {
		log.Printf("profile repo: git service not initialized; skip %s", username)
		return
	}
	if _, err := a.store.GetRepo(username, username); err == nil {
		return
	}
	if _, err := a.store.CreateRepo(username, username, "", false); err != nil {
		log.Printf("profile repo: create %s/%s: %v", username, username, err)
		return
	}
	_ = a.store.WatchRepo(username, username, username)
	if err := gitsvc.CreateBare(username, username); err != nil {
		_ = a.store.DeleteRepo(username, username)
		log.Printf("profile repo: create bare %s/%s: %v", username, username, err)
		return
	}
	if err := gitsvc.InitTemplate(username, username); err != nil {
		log.Printf("profile repo: init template %s/%s: %v", username, username, err)
	}
}
