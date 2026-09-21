package api

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"gitdash/backend/internal/logx"

	"golang.org/x/crypto/bcrypt"
)

// pwResetTTL 密码重置链接有效期。
const pwResetTTL = time.Hour

// sendPasswordReset 发送密码重置邮件（失败仅记日志，不向调用方暴露）。
func (a *API) sendPasswordReset(username, email, token, base string) {
	link := base + "/?reset_password=" + url.QueryEscape(token)
	subject := "gitdash: reset your password / 重置密码"
	body := fmt.Sprintf("Hi %s,\n\nWe received a request to reset your gitdash password.\nClick the link below to choose a new password:\n\n%s\n\nThis link expires in 1 hour. If you did not request it, ignore this email — your password stays unchanged.\n\n-- gitdash", username, link)
	if err := a.emailSender.Send(email, subject, body); err != nil {
		logx.Infof("password reset to %s: %v", email, err)
	}
}

// forgotPassword 请求发送密码重置邮件。
//
//	@Summary     请求密码重置
//	@Description 根据邮箱发送重置链接。无论邮箱是否存在都返回 200（防止账户枚举）；
//	@Description SMTP 未配置或邮箱非法时同样返回 200 但不发送。
//	@Tags        auth
//	@Accept      json
//	@Produce     json
//	@Param       body body map[string]string true "email"
//	@Success     200 {object} map[string]any
//	@Failure     429 {object} map[string]string
//	@Router      /auth/forgot-password [post]
func (a *API) forgotPassword(w http.ResponseWriter, r *http.Request) {
	if !a.passwordLoginEnabled() {
		writeCode(w, http.StatusForbidden, "password_login_disabled", "password login is disabled")
		return
	}
	var in struct {
		Email string `json:"email"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	key := "pwreset|" + clientIP(r)
	if a.rateBlocked(key) {
		writeCode(w, http.StatusTooManyRequests, "too_many_attempts", "too many attempts, try again later")
		return
	}
	a.rateFail(key)

	email := strings.TrimSpace(in.Email)
	if a.emailReady() && email != "" && emailRe.MatchString(email) {
		ua, err := a.store.GetByEmail(email)
		// 仅向已验证邮箱发送：避免未验证邮箱被他人占用后用于劫持账号。
		if err == nil && !ua.Banned && ua.Email != "" && ua.EmailVerified {
			token, terr := newSessionToken()
			if terr != nil {
				internalError(w, terr)
				return
			}
			_ = a.store.ClearPasswordResets(ua.Username)
			expires := time.Now().Add(pwResetTTL).UTC().Format(time.RFC3339)
			if perr := a.store.PutPasswordReset(token, ua.Username, expires); perr != nil {
				internalError(w, perr)
				return
			}
			a.sendPasswordReset(ua.Username, ua.Email, token, reqBase(r))
		}
	}
	// 响应恒为 200，避免通过响应差异枚举已注册邮箱。
	writeJSON(w, http.StatusOK, map[string]any{"sent": true})
}

// resetPassword 用重置令牌设置新密码。
//
//	@Summary     重置密码
//	@Description 校验邮件链接中的一次性令牌，设置新密码并撤销该用户的全部会话。
//	@Tags        auth
//	@Accept      json
//	@Produce     json
//	@Param       body body map[string]string true "token 与新密码"
//	@Success     200 {object} map[string]any
//	@Failure     400 {object} map[string]string
//	@Failure     429 {object} map[string]string
//	@Router      /auth/reset-password [post]
func (a *API) resetPassword(w http.ResponseWriter, r *http.Request) {
	if !a.passwordLoginEnabled() {
		writeCode(w, http.StatusForbidden, "password_login_disabled", "password login is disabled")
		return
	}
	var in struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	token := strings.TrimSpace(in.Token)
	if token == "" {
		writeCode(w, http.StatusBadRequest, "token_required", "token is required")
		return
	}
	if code, msg := passwordIssue(in.Password); code != "" {
		writeCode(w, http.StatusBadRequest, code, msg)
		return
	}
	key := "pwreset-confirm|" + clientIP(r)
	if a.rateBlocked(key) {
		writeCode(w, http.StatusTooManyRequests, "too_many_attempts", "too many attempts, try again later")
		return
	}
	username, err := a.store.TakePasswordReset(token)
	if err != nil {
		a.rateFail(key)
		writeCode(w, http.StatusBadRequest, "invalid_token", "reset token is invalid or expired")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), BcryptCost)
	if err != nil {
		internalError(w, err)
		return
	}
	if err := a.store.UpdatePassword(username, string(hash)); err != nil {
		internalError(w, err)
		return
	}
	// 改密后撤销该用户全部会话（keepToken 为空 = 全部撤销）。
	_ = a.store.DeleteSessionsExcept(username, "")
	_ = a.store.ClearPasswordResets(username)
	a.rateReset(key)
	logx.Infof("password reset completed for %s", username)
	writeJSON(w, http.StatusOK, map[string]any{"username": username, "reset": true})
}
