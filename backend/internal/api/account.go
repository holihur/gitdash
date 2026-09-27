package api

// 账号自助注销：彻底删除当前用户的账号与全部数据。

import (
	"errors"
	"gitdash/backend/internal/gitsvc"
	"gitdash/backend/internal/logx"
	"gitdash/backend/internal/pipeline"
	"gitdash/backend/internal/store"
	"net/http"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// userRepoNames 收集用户名下的仓库名。必须在删除 DB 行之前调用（删库后查询为空），
// 供随后 best-effort 清理磁盘数据。
func (a *API) userRepoNames(username string) []string {
	repos, err := a.store.ListRepos(username)
	if err != nil {
		logx.Infof("list repos for %q: %v", username, err)
		return nil
	}
	names := make([]string, 0, len(repos))
	for _, rp := range repos {
		names = append(names, rp.Name)
	}
	return names
}

// purgeUserRepoFiles 删除用户名下仓库的磁盘数据（git 对象 + 流水线日志 + 索引）。
// 仅在 DB 事务成功提交后调用：DB 是唯一可安全重试的环节，磁盘删除失败只记日志。
// DB 记录由 store.DeleteUserAccount 负责。
func (a *API) purgeUserRepoFiles(username string, names []string) {
	for _, name := range names {
		if err := gitsvc.Delete(username, name); err != nil {
			logx.Infof("delete git repo %s/%s: %v", username, name, err)
		}
		if a.codeIndex != nil {
			a.codeIndex.Forget(username, name)
		}
		if err := pipeline.DeleteLogs(username, name); err != nil {
			logx.Infof("delete pipeline logs %s/%s: %v", username, name, err)
		}
		if err := pipeline.DeleteArtifacts(username, name); err != nil {
			logx.Infof("delete pipeline artifacts %s/%s: %v", username, name, err)
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
			ok = a.store.AcceptTOTP(username, ua.MFASecret, strings.TrimSpace(in.Code), 1)
		}
		if !ok {
			a.rateFail(key)
			writeCode(w, http.StatusBadRequest, "invalid_mfa_code", "invalid or expired two-factor code")
			return
		}
	}
	a.rateReset(key)
	// 先收集仓库名，再删 DB（大事务，失败可安全重试）；仅 DB 提交成功后才
	// best-effort 清理磁盘，避免事务回滚时 git 数据已被不可恢复地删除。
	names := a.userRepoNames(username)
	if err := a.store.DeleteUserAccount(username); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			a.purgeUserRepoFiles(username, names)
			a.clearSessionCookie(w)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		internalError(w, err)
		return
	}
	a.purgeUserRepoFiles(username, names)
	logx.Infof("user %q deleted their account", username)
	a.clearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}
