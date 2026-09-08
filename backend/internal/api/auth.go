package api

import (
	"errors"
	"fmt"
	"gitdash/backend/internal/store"
	"gitdash/backend/internal/totp"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// register 用户注册。
//
//	@Summary     注册新用户
//	@Description 注册成功返回 201 与会话 token。
//	@Tags        auth
//	@Accept      json
//	@Produce     json
//	@Param       body body registerReq true "用户名与密码"
//	@Success     201 {object} map[string]string
//	@Failure     400 {object} map[string]string
//	@Failure     409 {object} map[string]string
//	@Failure     429 {object} map[string]string
//	@Router      /auth/register [post]
func (a *API) register(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	ipKey := "register|" + clientIP(r)
	username := strings.ToLower(strings.TrimSpace(in.Username))
	if a.rateBlocked(ipKey) {
		writeCode(w, http.StatusTooManyRequests, "too_many_attempts", "too many attempts, try again later")
		return
	}
	if a.store.IsOrg(username) { // 组织占用同名命名空间
		writeCode(w, http.StatusConflict, "username_taken", "username is already taken")
		return
	}
	if !usernameRe.MatchString(username) {
		a.rateFail(ipKey)
		writeCode(w, http.StatusBadRequest, "username_invalid", "username must be 2-32 chars: lowercase letters, digits, '_' or '-', starting alphanumeric")
		return
	}
	if len(in.Password) < 8 {
		a.rateFail(ipKey)
		writeCode(w, http.StatusBadRequest, "password_too_short", "password must be at least 8 characters")
		return
	}
	if _, err := a.store.GetByUsername(username); err == nil {
		a.rateFail(ipKey)
		writeCode(w, http.StatusConflict, "username_taken", "username is already taken")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		internalError(w, err)
		return
	}
	u, err := a.store.CreateUser(username, string(hash))
	if errors.Is(err, store.ErrExists) { // 并发注册竞态：唯一约束兜底
		a.rateFail(ipKey)
		writeCode(w, http.StatusConflict, "username_taken", "username is already taken")
		return
	}
	if err != nil {
		internalError(w, err)
		return
	}
	a.rateReset(ipKey)
	a.startSession(w, r, http.StatusCreated, u.Username)
}

// login 用户登录。
//
//	@Summary     登录
//	@Description 返回会话 token；若启用 MFA 则返回 mfa_required 与临时 mfa_token。
//	@Tags        auth
//	@Accept      json
//	@Produce     json
//	@Param       body body loginReq true "用户名与密码"
//	@Success     200 {object} map[string]any "会话 token 或 MFA 挑战"
//	@Failure     401 {object} map[string]string
//	@Failure     429 {object} map[string]string
//	@Router      /auth/login [post]
func (a *API) login(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	username := strings.ToLower(strings.TrimSpace(in.Username))
	key := a.rateKey(username, clientIP(r))
	if a.rateBlocked(key) {
		writeCode(w, http.StatusTooManyRequests, "too_many_attempts", "too many failed attempts, try again later")
		return
	}
	ua, err := a.store.GetByUsername(username)
	if err != nil {
		a.rateFail(key)
		writeCode(w, http.StatusUnauthorized, "invalid_credentials", "invalid username or password")
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(ua.PasswordHash), []byte(in.Password)) != nil {
		a.rateFail(key)
		writeCode(w, http.StatusUnauthorized, "invalid_credentials", "invalid username or password")
		return
	}
	a.rateReset(key)
	if ua.MFAEnabled {
		token, err := newSessionToken()
		if err != nil {
			internalError(w, err)
			return
		}
		expires := time.Now().Add(10 * time.Minute).UTC().Format(time.RFC3339)
		if err := a.store.PutMFAChallenge(token, ua.Username, expires); err != nil {
			internalError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"mfa_required": true,
			"mfa_token":    token,
		})
		return
	}
	a.startSession(w, r, http.StatusOK, ua.Username)
}

// mfaVerify 完成 MFA 二次验证并签发正式会话。
//
//	@Summary     MFA 二次验证
//	@Description 校验 TOTP 代码后签发正式会话 token。
//	@Tags        auth
//	@Accept      json
//	@Produce     json
//	@Param       body body mfaVerifyReq true "mfa_token 与 TOTP code"
//	@Success     200 {object} map[string]string
//	@Failure     401 {object} map[string]string
//	@Failure     429 {object} map[string]string
//	@Router      /auth/mfa-verify [post]
func (a *API) mfaVerify(w http.ResponseWriter, r *http.Request) {
	var in struct {
		MFAToken string `json:"mfa_token"`
		Code     string `json:"code"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	now := time.Now().UTC()
	username, expires, attempts, err := a.store.GetMFAChallenge(in.MFAToken)
	if err != nil || expires <= now.Format(time.RFC3339) {
		_ = a.store.DeleteMFAChallenge(in.MFAToken)
		writeCode(w, http.StatusUnauthorized, "mfa_challenge_expired", "mfa challenge expired, sign in again")
		return
	}
	ua, err := a.store.GetByUsername(username)
	if err != nil || !ua.MFAEnabled {
		writeCode(w, http.StatusUnauthorized, "invalid_credentials", "invalid username or password")
		return
	}
	if !totp.Verify(ua.MFASecret, strings.TrimSpace(in.Code), 1) {
		attempts++
		expired := attempts >= 5
		if expired {
			_ = a.store.DeleteMFAChallenge(in.MFAToken)
			writeCode(w, http.StatusTooManyRequests, "mfa_too_many_attempts", "too many attempts, sign in again")
			return
		}
		if err := a.store.SaveMFAChallenge(in.MFAToken, username, expires, attempts); err != nil {
			internalError(w, err)
			return
		}
		writeCode(w, http.StatusUnauthorized, "invalid_mfa_code", "invalid authenticator code")
		return
	}
	// 校验通过：令牌一次性作废并签发正式会话
	_ = a.store.DeleteMFAChallenge(in.MFAToken)
	a.startSession(w, r, http.StatusOK, ua.Username)
}

// emailRe 宽松的邮箱格式校验（本地@域名）。
var emailRe = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)

// me 获取当前登录用户信息。
//
//	@Summary     当前用户信息
//	@Description 返回当前会话用户的用户名、邮箱、创建时间与 MFA 状态。
//	@Tags        users
//	@Produce     json
//	@Security    BearerAuth
//	@Success     200 {object} map[string]any
//	@Failure     401 {object} map[string]string
//	@Failure     500 {object} map[string]string
//	@Router      /me [get]
func (a *API) me(w http.ResponseWriter, r *http.Request) {
	ua, err := a.store.GetByUsername(userFrom(r))
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"username":       ua.Username,
		"email":          ua.Email,
		"created_at":     ua.CreatedAt,
		"mfa_enabled":    ua.MFAEnabled,
		"notify_email":   ua.NotifyEmail,
		"email_verified": ua.EmailVerified,
	})
}

// exportMe 导出本人全部个人数据（GDPR Art. 20 data portability）。
//
//	@Summary     导出个人数据
//	@Description 以 JSON 附件形式下载本账号在实例上的个人数据与自产内容。
//	@Tags        users
//	@Produce     json
//	@Success     200 {object} store.UserExport
//	@Failure     500 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /me/export [get]
func (a *API) exportMe(w http.ResponseWriter, r *http.Request) {
	data, err := a.store.ExportUserData(userFrom(r))
	if err != nil {
		internalError(w, err)
		return
	}
	w.Header().Set("Content-Disposition", `attachment; filename="gitdash-export.json"`)
	writeJSON(w, http.StatusOK, data)
}

// updateProfile 更新个人资料（邮箱与邮件通知开关；邮箱空串清除）。
//
//	@Summary     更新个人资料
//	@Description 更新当前用户邮箱与邮件通知开关；邮箱空串表示清除。返回 200 与更新后的资料。
//	@Tags        users
//	@Accept      json
//	@Produce     json
//	@Security    BearerAuth
//	@Param       body body updateProfileReq true "邮箱（可为空串）与 notify_email"
//	@Success     200 {object} map[string]any
//	@Failure     400 {object} map[string]string
//	@Failure     401 {object} map[string]string
//	@Failure     409 {object} map[string]string
//	@Router      /me/profile [post]
func (a *API) updateProfile(w http.ResponseWriter, r *http.Request) {
	me := userFrom(r)
	var in updateProfileReq
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	if in.Email != nil {
		email := strings.TrimSpace(*in.Email)
		if email != "" && !emailRe.MatchString(email) {
			writeErr(w, http.StatusBadRequest, "invalid email address")
			return
		}
		// 邮箱验证流程：生成 24h 令牌；SMTP 未配置时无验证途径，直接视为已验证
		verified := a.emailSender == nil
		token := ""
		if !verified && email != "" {
			var err error
			if token, err = newSessionToken(); err != nil {
				internalError(w, err)
				return
			}
		}
		if err := a.store.SetUserEmailWithToken(me, email, token, time.Now().Add(24*time.Hour).UTC().Format(time.RFC3339)); err != nil {
			if errors.Is(err, store.ErrExists) {
				writeErr(w, http.StatusConflict, "email already in use")
			} else {
				internalError(w, err)
			}
			return
		}
		if verified {
			_ = a.store.MarkEmailVerifiedByUsername(me)
		} else if email != "" {
			a.sendEmailVerification(me, email, token, reqBase(r))
		}
	}
	if in.NotifyEmail != nil {
		if err := a.store.SetNotifyEmail(me, *in.NotifyEmail); err != nil {
			internalError(w, err)
			return
		}
	}
	ua, err := a.store.GetByUsername(me)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"username":       ua.Username,
		"email":          ua.Email,
		"notify_email":   ua.NotifyEmail,
		"email_verified": ua.EmailVerified,
	})
}

// sendEmailVerification 发送验证邮件（失败仅记日志，不影响邮箱保存）。
func (a *API) sendEmailVerification(username, email, token, base string) {
	link := base + "/?verify_email=" + url.QueryEscape(token)
	subject := "gitdash: verify your email / 邮箱验证"
	body := fmt.Sprintf("Hi %s,\n\nPlease verify your email address:\n%s\n\nThis link expires in 24 hours.\n\n-- gitdash", username, link)
	if err := a.emailSender.Send(email, subject, body); err != nil {
		log.Printf("email verification to %s: %v", email, err)
	}
}

// verifyEmail 验证邮箱（令牌一次性，24h 有效）。
//
//	@Summary     验证邮箱
//	@Description 用验证令牌完成邮箱验证（令牌来自验证邮件链接）。
//	@Tags        users
//	@Accept      json
//	@Produce     json
//	@Param       body body map[string]string true "token"
//	@Success     200 {object} map[string]any
//	@Failure     400 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /me/email/verify [post]
func (a *API) verifyEmail(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Token string `json:"token"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	in.Token = strings.TrimSpace(in.Token)
	if in.Token == "" {
		writeCode(w, http.StatusBadRequest, "token_required", "token is required")
		return
	}
	username, err := a.store.MarkEmailVerified(in.Token)
	if errors.Is(err, store.ErrNotFound) {
		writeCode(w, http.StatusBadRequest, "invalid_token", "token is invalid or expired")
		return
	}
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"username": username, "email_verified": true})
}

// resendEmailVerification 重发验证邮件（未验证邮箱才可重发）。
//
//	@Summary     重发验证邮件
//	@Tags        users
//	@Produce     json
//	@Success     200 {object} map[string]any
//	@Failure     400 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /me/email/resend [post]
func (a *API) resendEmailVerification(w http.ResponseWriter, r *http.Request) {
	me := userFrom(r)
	ua, err := a.store.GetByUsername(me)
	if err != nil {
		internalError(w, err)
		return
	}
	if a.emailSender == nil {
		writeCode(w, http.StatusBadRequest, "smtp_not_configured", "SMTP is not configured")
		return
	}
	if ua.Email == "" || ua.EmailVerified {
		writeCode(w, http.StatusBadRequest, "nothing_to_verify", "email is empty or already verified")
		return
	}
	token, err := newSessionToken()
	if err != nil {
		internalError(w, err)
		return
	}
	if err := a.store.ResendEmailToken(me, token, time.Now().Add(24*time.Hour).UTC().Format(time.RFC3339)); err != nil {
		internalError(w, err)
		return
	}
	a.sendEmailVerification(me, ua.Email, token, reqBase(r))
	writeJSON(w, http.StatusOK, map[string]any{"sent": true})
}
