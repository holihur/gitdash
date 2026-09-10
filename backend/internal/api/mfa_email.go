package api

import (
	"crypto/rand"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"strings"
	"time"
)

// ---- email MFA（独立 MFA 方式：向已验证邮箱发送 6 位验证码）----

// genEmailCode 生成 6 位数字验证码（crypto/rand）。
func genEmailCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n), nil
}

// issueEmailMFACode 生成验证码并持久化，随后发送邮件（发送失败仅记日志）。
// key 区分用途：enroll:<username> / disable:<username> / <mfa_token>（登录挑战）。
func (a *API) issueEmailMFACode(key, username, email, subject string) (err error) {
	code, err := genEmailCode()
	if err != nil {
		return err
	}
	expires := time.Now().Add(10 * time.Minute).UTC().Format(time.RFC3339)
	if err := a.store.PutEmailMFACode(key, code, expires); err != nil {
		return err
	}
	body := fmt.Sprintf("Hi %s,\n\nYour gitdash verification code is:\n\n%s\n\nIt expires in 10 minutes. If you did not request it, ignore this email.\n\n-- gitdash", username, code)
	if serr := a.emailSender.Send(email, subject, body); serr != nil {
		log.Printf("email mfa to %s: %v", email, serr)
	}
	return nil
}

// checkEmailMFACode 校验验证码；命中则消费（删除）并返回 true。
func (a *API) checkEmailMFACode(key, code string) bool {
	code = strings.TrimSpace(code)
	if code == "" {
		return false
	}
	stored, expires, err := a.store.GetEmailMFACode(key)
	if err != nil || expires <= time.Now().UTC().Format(time.RFC3339) {
		return false
	}
	if stored != code {
		return false
	}
	_ = a.store.DeleteEmailMFACode(key)
	return true
}

// mfaEmailEnroll 开始绑定 email MFA：向已验证邮箱发送激活验证码。
//
//	@Summary     绑定 email MFA
//	@Description 要求已验证邮箱；发送 6 位激活码（10 分钟有效）。
//	@Tags        users
//	@Produce     json
//	@Security    BearerAuth
//	@Success     200 {object} map[string]any
//	@Failure     400 {object} map[string]string
//	@Failure     401 {object} map[string]string
//	@Failure     409 {object} map[string]string
//	@Router      /me/mfa/email/enroll [post]
func (a *API) mfaEmailEnroll(w http.ResponseWriter, r *http.Request) {
	username := userFrom(r)
	ua, err := a.store.GetByUsername(username)
	if err != nil {
		internalError(w, err)
		return
	}
	if a.emailSender == nil {
		writeCode(w, http.StatusBadRequest, "smtp_not_configured", "SMTP is not configured")
		return
	}
	if ua.Email == "" || !ua.EmailVerified {
		writeCode(w, http.StatusBadRequest, "email_not_verified", "set and verify your email first")
		return
	}
	if ua.MFAEnabled {
		writeCode(w, http.StatusConflict, "mfa_already_enabled", "mfa is already enabled")
		return
	}
	if err := a.issueEmailMFACode("enroll:"+username, username, ua.Email, "gitdash: enable email two-factor code"); err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sent": true})
}

// mfaEmailActivate 激活 email MFA。
//
//	@Summary     激活 email MFA
//	@Description 校验邮箱激活码后启用 MFA（方式为 email）。返回 204。
//	@Tags        users
//	@Accept      json
//	@Produce     json
//	@Security    BearerAuth
//	@Param       body body map[string]string true "code"
//	@Success     204 {object} nil
//	@Failure     400 {object} map[string]string
//	@Failure     401 {object} map[string]string
//	@Failure     409 {object} map[string]string
//	@Router      /me/mfa/email/activate [post]
func (a *API) mfaEmailActivate(w http.ResponseWriter, r *http.Request) {
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
	if a.emailSender == nil {
		writeCode(w, http.StatusBadRequest, "smtp_not_configured", "SMTP is not configured")
		return
	}
	var in struct {
		Code string `json:"code"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	if !a.checkEmailMFACode("enroll:"+username, in.Code) {
		writeCode(w, http.StatusBadRequest, "invalid_mfa_code", "invalid or expired verification code")
		return
	}
	if err := a.store.SetMFAMethod(username, "email", true); err != nil {
		internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// mfaEmailSend 发送关闭 email MFA 所需的验证码（已启用 email MFA 时调用）。
//
//	@Summary     发送 email MFA 验证码
//	@Description 向已验证邮箱发送验证码，用于 /me/mfa/disable 校验。
//	@Tags        users
//	@Produce     json
//	@Security    BearerAuth
//	@Success     204 {object} nil
//	@Failure     400 {object} map[string]string
//	@Failure     401 {object} map[string]string
//	@Failure     409 {object} map[string]string
//	@Router      /me/mfa/email/send [post]
func (a *API) mfaEmailSend(w http.ResponseWriter, r *http.Request) {
	username := userFrom(r)
	ua, err := a.store.GetByUsername(username)
	if err != nil {
		internalError(w, err)
		return
	}
	if !ua.MFAEnabled || ua.MFAMethod != "email" {
		writeCode(w, http.StatusConflict, "mfa_not_enabled", "email mfa is not enabled")
		return
	}
	if a.emailSender == nil || ua.Email == "" || !ua.EmailVerified {
		writeCode(w, http.StatusBadRequest, "email_not_verified", "verified email and SMTP are required")
		return
	}
	if err := a.issueEmailMFACode("disable:"+username, username, ua.Email, "gitdash: confirm code to disable email two-factor"); err != nil {
		internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
