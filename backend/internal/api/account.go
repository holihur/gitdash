package api

// 账号自助注销：彻底删除当前用户的账号与全部数据。

import (
	"errors"
	"gitdash/backend/internal/gitsvc"
	"gitdash/backend/internal/logx"
	"gitdash/backend/internal/pipeline"
	"gitdash/backend/internal/store"
	"gitdash/backend/internal/totp"
	"net/http"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// purgeUserRepoFiles 删除用户名下所有仓库的磁盘数据（git 对象 + 流水线日志）。
// DB 记录由 store.DeleteUserAccount 负责。
func (a *API) purgeUserRepoFiles(username string) {
	repos, err := a.store.ListRepos(username)
	if err != nil {
		logx.Infof("list repos for %q: %v", username, err)
		return
	}
	for _, rp := range repos {
		if err := gitsvc.Delete(username, rp.Name); err != nil {
			logx.Infof("delete git repo %s/%s: %v", username, rp.Name, err)
		}
		if err := pipeline.DeleteLogs(username, rp.Name); err != nil {
			logx.Infof("delete pipeline logs %s/%s: %v", username, rp.Name, err)
		}
	}
}

// deleteMe 当前用户自助注销（彻底删除账号与全部数据）。
//
//	@Summary     注销账号
//	@Description 校验密码（启用 MFA 时还需二次验证码）后，永久删除当前账号及其全部数据。返回 204。
//	@Tags        users
//	@Accept      json
//	@Produce     json
//	@Security    BearerAuth
//	@Param       body body deleteAccountReq true "当前密码与（启用 MFA 时的）验证码"
//	@Success     204 {object} nil
//	@Failure     400 {object} map[string]string
//	@Failure     401 {object} map[string]string
//	@Failure     403 {object} map[string]string
//	@Failure     429 {object} map[string]string
//	@Router      /me [delete]
func (a *API) deleteMe(w http.ResponseWriter, r *http.Request) {
	username := userFrom(r)
	// 账号注销是高风险操作：仅允许交互式会话，禁止 PAT。
	if _, isPAT := r.Context().Value(ctxPatScopes{}).([]string); isPAT {
		writeCode(w, http.StatusForbidden, "session_required", "account deletion requires an interactive session, not a personal access token")
		return
	}
	var in deleteAccountReq
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	key := a.rateKey(username, clientIP(r))
	if a.rateBlocked(key) {
		writeCode(w, http.StatusTooManyRequests, "too_many_attempts", "too many failed attempts, try again later")
		return
	}
	ua, err := a.store.GetByUsername(username)
	if err != nil {
		internalError(w, err)
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(ua.PasswordHash), []byte(in.Password)) != nil {
		a.rateFail(key)
		writeCode(w, http.StatusUnauthorized, "invalid_current_password", "current password is incorrect")
		return
	}
	if ua.MFAEnabled {
		ok := false
		if ua.MFAMethod == "email" {
			// 复用 /me/mfa/email/send 下发的验证码（key: disable:<username>）
			ok = a.checkEmailMFACode("disable:"+username, in.Code)
		} else {
			ok = totp.Verify(ua.MFASecret, strings.TrimSpace(in.Code), 1)
		}
		if !ok {
			a.rateFail(key)
			writeCode(w, http.StatusBadRequest, "invalid_mfa_code", "invalid or expired two-factor code")
			return
		}
	}
	a.rateReset(key)
	// 先清理磁盘数据（git 仓库 / 流水线日志），再删除数据库记录
	a.purgeUserRepoFiles(username)
	if err := a.store.DeleteUserAccount(username); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			a.clearSessionCookie(w)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		internalError(w, err)
		return
	}
	logx.Infof("user %q deleted their account", username)
	a.clearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}
