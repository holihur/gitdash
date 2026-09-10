package api

import (
	"gitdash/backend/internal/totp"
	"net/http"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// ---- user profile & mfa ----

// changePassword 修改当前用户密码。
//
//	@Summary     修改密码
//	@Description 校验当前密码后更新，并撤销其它会话（仅保留当前会话）。返回 204。
//	@Tags        users
//	@Accept      json
//	@Produce     json
//	@Security    BearerAuth
//	@Param       body body changePasswordReq true "当前密码与新密码"
//	@Success     204 {object} nil
//	@Failure     400 {object} map[string]string
//	@Failure     401 {object} map[string]string
//	@Router      /me/password [post]
func (a *API) changePassword(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Current string `json:"current_password"`
		New     string `json:"new_password"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	ua, err := a.store.GetByUsername(userFrom(r))
	if err != nil {
		internalError(w, err)
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(ua.PasswordHash), []byte(in.Current)) != nil {
		writeCode(w, http.StatusUnauthorized, "invalid_current_password", "current password is incorrect")
		return
	}
	if code, msg := passwordIssue(in.New); code != "" {
		writeCode(w, http.StatusBadRequest, code, msg)
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(in.New), BcryptCost)
	if err != nil {
		internalError(w, err)
		return
	}
	if err := a.store.UpdatePassword(ua.Username, string(hash)); err != nil {
		internalError(w, err)
		return
	}
	// 改密后撤销其它会话，仅保留当前会话
	if tok := bearerToken(r); tok != "" {
		_ = a.store.DeleteSessionsExcept(ua.Username, tok)
	}
	w.WriteHeader(http.StatusNoContent)
}

// mfaStatus 查询当前用户 MFA 状态。
//
//	@Summary     MFA 状态
//	@Description 返回是否已启用；若存在待激活的 secret，则附带 pending_secret 与 otpauth_url。
//	@Tags        users
//	@Produce     json
//	@Security    BearerAuth
//	@Success     200 {object} map[string]any
//	@Failure     401 {object} map[string]string
//	@Failure     500 {object} map[string]string
//	@Router      /me/mfa [get]
func (a *API) mfaStatus(w http.ResponseWriter, r *http.Request) {
	ua, err := a.store.GetByUsername(userFrom(r))
	if err != nil {
		internalError(w, err)
		return
	}
	method := ua.MFAMethod
	if method == "" {
		method = "totp"
	}
	resp := map[string]any{"enabled": ua.MFAEnabled, "method": method}
	if !ua.MFAEnabled && ua.MFASecret != "" { // 待激活的 secret（页面刷新后仍可继续）
		resp["pending_secret"] = ua.MFASecret
		resp["otpauth_url"] = totp.URI("gitdash", ua.Username, ua.MFASecret)
	}
	writeJSON(w, http.StatusOK, resp)
}

// mfaEnroll 开始 MFA 绑定，生成（或复用）secret。
//
//	@Summary     开始 MFA 绑定
//	@Description 返回 secret 与 otpauth URL，供认证器扫码；尚未激活。
//	@Tags        users
//	@Produce     json
//	@Security    BearerAuth
//	@Success     200 {object} map[string]any
//	@Failure     401 {object} map[string]string
//	@Failure     409 {object} map[string]string
//	@Failure     500 {object} map[string]string
//	@Router      /me/mfa/enroll [post]
func (a *API) mfaEnroll(w http.ResponseWriter, r *http.Request) {
	username := userFrom(r)
	ua, err := a.store.GetByUsername(username)
	if err != nil {
		internalError(w, err)
		return
	}
	if ua.MFAEnabled {
		writeCode(w, http.StatusConflict, "mfa_already_enabled", "mfa is already enabled")
		return
	}
	secret := ua.MFASecret
	if secret == "" {
		secret, err = totp.GenerateSecret()
		if err != nil {
			internalError(w, err)
			return
		}
		if err := a.store.SetMFASecret(username, secret, false); err != nil {
			internalError(w, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"secret":      secret,
		"otpauth_url": totp.URI("gitdash", username, secret),
	})
}

// mfaActivate 激活 MFA。
//
//	@Summary     激活 MFA
//	@Description 校验 TOTP 代码后启用 MFA。返回 204。
//	@Tags        users
//	@Accept      json
//	@Produce     json
//	@Security    BearerAuth
//	@Param       body body mfaActivateReq true "TOTP code"
//	@Success     204 {object} nil
//	@Failure     400 {object} map[string]string
//	@Failure     401 {object} map[string]string
//	@Failure     409 {object} map[string]string
//	@Router      /me/mfa/activate [post]
func (a *API) mfaActivate(w http.ResponseWriter, r *http.Request) {
	username := userFrom(r)
	ua, err := a.store.GetByUsername(username)
	if err != nil {
		internalError(w, err)
		return
	}
	if ua.MFAEnabled {
		writeCode(w, http.StatusConflict, "mfa_already_enabled", "mfa is already enabled")
		return
	}
	if ua.MFASecret == "" {
		writeCode(w, http.StatusBadRequest, "mfa_not_enrolled", "enroll first to get a secret")
		return
	}
	var in struct {
		Code string `json:"code"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	if !totp.Verify(ua.MFASecret, strings.TrimSpace(in.Code), 1) {
		writeCode(w, http.StatusBadRequest, "invalid_mfa_code", "invalid authenticator code")
		return
	}
	if err := a.store.SetMFASecret(username, ua.MFASecret, true); err != nil {
		internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// mfaDisable 关闭 MFA。
//
//	@Summary     关闭 MFA
//	@Description 需提供当前密码与有效 TOTP 代码。返回 204。
//	@Tags        users
//	@Accept      json
//	@Produce     json
//	@Security    BearerAuth
//	@Param       body body mfaDisableReq true "当前密码与 TOTP code"
//	@Success     204 {object} nil
//	@Failure     400 {object} map[string]string
//	@Failure     401 {object} map[string]string
//	@Failure     409 {object} map[string]string
//	@Router      /me/mfa/disable [post]
func (a *API) mfaDisable(w http.ResponseWriter, r *http.Request) {
	username := userFrom(r)
	ua, err := a.store.GetByUsername(username)
	if err != nil {
		internalError(w, err)
		return
	}
	if !ua.MFAEnabled {
		writeCode(w, http.StatusConflict, "mfa_not_enabled", "mfa is not enabled")
		return
	}
	var in struct {
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(ua.PasswordHash), []byte(in.Password)) != nil {
		writeCode(w, http.StatusUnauthorized, "invalid_current_password", "current password is incorrect")
		return
	}
	if ua.MFAMethod == "email" { // email 方式：校验 /me/mfa/email/send 下发的验证码
		if !a.checkEmailMFACode("disable:"+username, in.Code) {
			writeCode(w, http.StatusBadRequest, "invalid_mfa_code", "invalid or expired verification code")
			return
		}
	} else if !totp.Verify(ua.MFASecret, strings.TrimSpace(in.Code), 1) {
		writeCode(w, http.StatusBadRequest, "invalid_mfa_code", "invalid authenticator code")
		return
	}
	if err := a.store.ClearMFA(username); err != nil {
		internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// logout 登出并作废当前会话。
//
//	@Summary     登出
//	@Description 作废当前 token（或 cookie 会话）并清除 cookie。返回 204。
//	@Tags        auth
//	@Produce     json
//	@Security    BearerAuth
//	@Success     204 {object} nil
//	@Router      /auth/logout [post]
func (a *API) logout(w http.ResponseWriter, r *http.Request) {
	tok := bearerToken(r)
	if tok == "" {
		if c, err := r.Cookie(sessionCookie); err == nil {
			tok = c.Value
		}
	}
	if tok != "" {
		_ = a.store.DeleteSession(tok)
	}
	a.clearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

// listGPGKeys 列出当前用户的 GPG 公钥。
//
//	@Summary     列出 GPG 公钥
//	@Description 返回当前用户注册的所有 GPG 公钥。
//	@Tags        users
//	@Produce     json
//	@Security    BearerAuth
//	@Success     200 {array}  store.GPGKey
//	@Failure     401 {object} map[string]string
//	@Failure     500 {object} map[string]string
//	@Router      /gpg [get]
