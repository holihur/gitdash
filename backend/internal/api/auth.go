package api

import (
	"log"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"gitdash/backend/internal/gpgsig"
	"gitdash/backend/internal/store"
	"gitdash/backend/internal/totp"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
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
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	u, err := a.store.CreateUser(username, string(hash))
	if errors.Is(err, store.ErrExists) { // 并发注册竞态：唯一约束兜底
		a.rateFail(ipKey)
		writeCode(w, http.StatusConflict, "username_taken", "username is already taken")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
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
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		a.mu.Lock()
		a.mfaPending[token] = mfaChallenge{username: ua.Username, expires: time.Now().Add(10 * time.Minute)}
		a.mu.Unlock()
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
	a.mu.Lock()
	ch, ok := a.mfaPending[in.MFAToken]
	a.mu.Unlock()
	if !ok || time.Now().After(ch.expires) {
		a.mu.Lock()
		delete(a.mfaPending, in.MFAToken)
		a.mu.Unlock()
		writeCode(w, http.StatusUnauthorized, "mfa_challenge_expired", "mfa challenge expired, sign in again")
		return
	}
	ua, err := a.store.GetByUsername(ch.username)
	if err != nil || !ua.MFAEnabled {
		writeCode(w, http.StatusUnauthorized, "invalid_credentials", "invalid username or password")
		return
	}
	if !totp.Verify(ua.MFASecret, strings.TrimSpace(in.Code), 1) {
		a.mu.Lock()
		ch.attempts++
		expired := ch.attempts >= 5
		if expired {
			delete(a.mfaPending, in.MFAToken)
		} else {
			a.mfaPending[in.MFAToken] = ch
		}
		a.mu.Unlock()
		if expired {
			writeCode(w, http.StatusTooManyRequests, "mfa_too_many_attempts", "too many attempts, sign in again")
			return
		}
		writeCode(w, http.StatusUnauthorized, "invalid_mfa_code", "invalid authenticator code")
		return
	}
	// 校验通过：令牌一次性作废并签发正式会话
	a.mu.Lock()
	delete(a.mfaPending, in.MFAToken)
	a.mu.Unlock()
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
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"username":     ua.Username,
		"email":        ua.Email,
		"created_at":   ua.CreatedAt,
		"mfa_enabled":    ua.MFAEnabled,
		"notify_email":   ua.NotifyEmail,
		"email_verified": ua.EmailVerified,
	})
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
				writeErr(w, http.StatusInternalServerError, err.Error())
				return
			}
		}
		if err := a.store.SetUserEmailWithToken(me, email, token, time.Now().Add(24*time.Hour).UTC().Format(time.RFC3339)); err != nil {
			if errors.Is(err, store.ErrExists) {
				writeErr(w, http.StatusConflict, "email already in use")
			} else {
				writeErr(w, http.StatusInternalServerError, err.Error())
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
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	ua, err := a.store.GetByUsername(me)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
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
		writeErr(w, http.StatusInternalServerError, err.Error())
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
		writeErr(w, http.StatusInternalServerError, err.Error())
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
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := a.store.ResendEmailToken(me, token, time.Now().Add(24*time.Hour).UTC().Format(time.RFC3339)); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.sendEmailVerification(me, ua.Email, token, reqBase(r))
	writeJSON(w, http.StatusOK, map[string]any{"sent": true})
}

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
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(ua.PasswordHash), []byte(in.Current)) != nil {
		writeCode(w, http.StatusUnauthorized, "invalid_current_password", "current password is incorrect")
		return
	}
	if len(in.New) < 8 {
		writeCode(w, http.StatusBadRequest, "password_too_short", "password must be at least 8 characters")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(in.New), bcrypt.DefaultCost)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := a.store.UpdatePassword(ua.Username, string(hash)); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
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
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	resp := map[string]any{"enabled": ua.MFAEnabled}
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
		writeErr(w, http.StatusInternalServerError, err.Error())
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
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		if err := a.store.SetMFASecret(username, secret, false); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
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
		writeErr(w, http.StatusInternalServerError, err.Error())
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
		writeErr(w, http.StatusInternalServerError, err.Error())
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
		writeErr(w, http.StatusInternalServerError, err.Error())
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
	if !totp.Verify(ua.MFASecret, strings.TrimSpace(in.Code), 1) {
		writeCode(w, http.StatusBadRequest, "invalid_mfa_code", "invalid authenticator code")
		return
	}
	if err := a.store.ClearMFA(username); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
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
func (a *API) listGPGKeys(w http.ResponseWriter, r *http.Request) {
	keys, err := a.store.ListGPGKeys(userFrom(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, keys)
}

// addGPGKey 添加 GPG 公钥。
//
//	@Summary     添加 GPG 公钥
//	@Description 解析 armored 公钥并注册。返回 201 与新键。
//	@Tags        users
//	@Accept      json
//	@Produce     json
//	@Security    BearerAuth
//	@Param       body body addGPGKeyReq true "armored 公钥"
//	@Success     201 {object} store.GPGKey
//	@Failure     400 {object} map[string]string
//	@Failure     401 {object} map[string]string
//	@Failure     409 {object} map[string]string
//	@Router      /gpg [post]
func (a *API) addGPGKey(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Armor string `json:"armor"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	armor := strings.TrimSpace(in.Armor)
	if armor == "" {
		writeCode(w, http.StatusBadRequest, "gpg_key_required", "armored public key is required")
		return
	}
	fp, err := gpgsig.ParseArmoredKey(armor)
	if err != nil {
		writeCode(w, http.StatusBadRequest, "gpg_key_invalid", err.Error())
		return
	}
	k, err := a.store.AddGPGKey(userFrom(r), fp, armor)
	a.invalidateGPGKeys()
	if errors.Is(err, store.ErrExists) {
		writeCode(w, http.StatusConflict, "gpg_key_exists", "this gpg key is already registered")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, k)
}

// deleteGPGKey 删除指定 GPG 公钥。
//
//	@Summary     删除 GPG 公钥
//	@Description 按 id 删除当前用户的 GPG 公钥。返回 204。
//	@Tags        users
//	@Produce     json
//	@Security    BearerAuth
//	@Param       id path int true "GPG key id"
//	@Success     204 {object} nil
//	@Failure     400 {object} map[string]string
//	@Failure     401 {object} map[string]string
//	@Failure     404 {object} map[string]string
//	@Router      /gpg/{id} [delete]
func (a *API) deleteGPGKey(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_id", "invalid id")
		return
	}
	if errors.Is(a.store.DeleteGPGKey(userFrom(r), id), store.ErrNotFound) {
		writeCode(w, http.StatusNotFound, "gpg_key_not_found", "gpg key not found")
		return
	}
	a.invalidateGPGKeys()
	w.WriteHeader(http.StatusNoContent)
}

// oauthIssueSession 为第三方登录用户签发 cookie 会话并跳回前端（不写 JSON）。

func (a *API) adminEnabled() bool {
	n, err := a.store.AdminCount()
	return err == nil && n > 0
}

func reqBase(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if fwd := r.Header.Get("X-Forwarded-Proto"); fwd != "" {
		if i := strings.IndexByte(fwd, ','); i > 0 {
			fwd = fwd[:i]
		}
		if fwd = strings.TrimSpace(fwd); fwd != "" {
			scheme = fwd
		}
	}
	host := r.Host
	if h := r.Header.Get("X-Forwarded-Host"); h != "" {
		host = strings.TrimSpace(strings.Split(h, ",")[0])
	}
	return scheme + "://" + host
}

func (a *API) githubBase() string {
	if v := os.Getenv("GITDASH_GITHUB_BASE"); v != "" {
		return strings.TrimRight(v, "/")
	}
	return "https://github.com"
}

func (a *API) githubAPIBase() string {
	if v := os.Getenv("GITDASH_GITHUB_API_BASE"); v != "" {
		return strings.TrimRight(v, "/")
	}
	return "https://api.github.com"
}

func (a *API) oauthSettings() (enabled bool, clientID, clientSecret string) {
	return a.store.GetSetting("github_oauth_enabled") == "1",
		a.store.GetSetting("github_client_id"),
		a.store.GetSetting("github_client_secret")
}

// providers 公开列出可用的第三方登录（未启用不暴露配置）。
//
//	@Summary     列出第三方登录方式
//	@Description 返回 github 与 oidc 的启用状态及授权 URL（未启用不暴露配置）。
//	@Tags        auth
//	@Produce     json
//	@Success     200 {object} map[string]any
//	@Router      /auth/providers [get]
func (a *API) providers(w http.ResponseWriter, r *http.Request) {
	resp := map[string]any{
		"github": map[string]any{"enabled": false},
		"oidc":   map[string]any{"enabled": false},
	}
	ghEnabled, ghID, _ := a.oauthSettings()
	if ghEnabled && ghID != "" {
		cb := reqBase(r) + "/api/auth/github/callback"
		q := url.Values{}
		q.Set("client_id", ghID)
		q.Set("scope", "read:user user:email")
		q.Set("redirect_uri", cb)
		q.Set("state", "STATE")
		resp["github"] = map[string]any{
			"enabled": true, "client_id": ghID,
			"authorize_url": a.githubBase() + "/login/oauth/authorize?" + q.Encode(),
		}
	}
	if a.store.GetSetting("oidc_enabled") == "1" && a.store.GetSetting("oidc_client_id") != "" {
		resp["oidc"] = map[string]any{
			"enabled":       true,
			"name":          defaultStr(a.store.GetSetting("oidc_name"), "OIDC"),
			"issuer":        a.store.GetSetting("oidc_issuer"),
			"authorize_url": reqBase(r) + "/api/auth/oidc/start",
			"callback":      reqBase(r) + "/api/auth/oidc/callback",
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

func defaultStr(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

func (a *API) githubStart(w http.ResponseWriter, r *http.Request) {
	enabled, id, _ := a.oauthSettings()
	if !enabled || id == "" {
		writeCode(w, http.StatusNotFound, "oauth_disabled", "github login is not enabled")
		return
	}
	state, err := newSessionToken()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := a.saveOAuthState(state); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	q := url.Values{}
	q.Set("client_id", id)
	q.Set("scope", "read:user user:email")
	q.Set("redirect_uri", reqBase(r)+"/api/auth/github/callback")
	q.Set("state", state)
	http.Redirect(w, r, a.githubBase()+"/login/oauth/authorize?"+q.Encode(), http.StatusFound)
}

func (a *API) githubCallback(w http.ResponseWriter, r *http.Request) {
	enabled, id, secret := a.oauthSettings()
	fail := func(msg string) {
		http.Redirect(w, r, "/?auth_error="+url.QueryEscape(msg), http.StatusFound)
	}
	if !enabled || id == "" || secret == "" {
		fail("github login is not enabled")
		return
	}
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	if code == "" || state == "" {
		fail("invalid oauth response")
		return
	}
	if !a.checkOAuthState(state) {
		fail("oauth state expired, try again")
		return
	}
	// 换取 access token
	tokForm := url.Values{}
	tokForm.Set("client_id", id)
	tokForm.Set("client_secret", secret)
	tokForm.Set("code", code)
	tokForm.Set("redirect_uri", reqBase(r)+"/api/auth/github/callback")
	treq, _ := http.NewRequest(http.MethodPost, a.githubBase()+"/login/oauth/access_token", strings.NewReader(tokForm.Encode()))
	treq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	treq.Header.Set("Accept", "application/json")
	tres, err := http.DefaultClient.Do(treq)
	if err != nil {
		fail("token exchange failed")
		return
	}
	defer func() { _ = tres.Body.Close() }()
	var tok struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err := json.NewDecoder(tres.Body).Decode(&tok); err != nil || tok.AccessToken == "" {
		fail("token exchange failed: " + tok.Error)
		return
	}
	// 获取用户信息
	ureq, _ := http.NewRequest(http.MethodGet, a.githubAPIBase()+"/user", nil)
	ureq.Header.Set("Authorization", "Bearer "+tok.AccessToken)
	ureq.Header.Set("Accept", "application/vnd.github+json")
	ures, err := http.DefaultClient.Do(ureq)
	if err != nil {
		fail("fetch github user failed")
		return
	}
	defer func() { _ = ures.Body.Close() }()
	if ures.StatusCode != http.StatusOK {
		fail("github user fetch failed")
		return
	}
	var gu struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
		Name  string `json:"name"`
	}
	if err := json.NewDecoder(ures.Body).Decode(&gu); err != nil || gu.ID == 0 {
		fail("invalid github user")
		return
	}
	username, err := a.loginOrCreateOAuthUser(r, "github", fmt.Sprint(gu.ID), gu.Login)
	if err != nil {
		fail("account provisioning failed")
		return
	}
	a.oauthIssueSession(w, r, username)
}

func (a *API) loginOrCreateOAuthUser(r *http.Request, provider, externalID, login string) (string, error) {
	if _, username, err := a.store.OAuthUser(provider, externalID); err == nil {
		return username, nil
	}
	// 未绑定：按 GitHub login 关联或新建账号
	username := sanitizeGithubLogin(login, externalID)
	if _, err := a.store.GetByUsername(username); err == nil {
		// 存在同名用户：绑定后登录
		uid, uerr := a.store.UserID(username)
		if uerr != nil {
			return "", uerr
		}
		if err := a.store.LinkOAuth(provider, externalID, uid); err != nil {
			return "", err
		}
		return username, nil
	}
	randPass := randomPassword()
	hash, err := bcrypt.GenerateFromPassword([]byte(randPass), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	u, err := a.store.CreateUser(username, string(hash))
	if err != nil {
		if errors.Is(err, store.ErrExists) { // 竞态
			uid, uerr := a.store.UserID(username)
			if uerr != nil {
				return "", uerr
			}
			if err := a.store.LinkOAuth(provider, externalID, uid); err != nil {
				return "", err
			}
			return username, nil
		}
		return "", err
	}
	if err := a.store.LinkOAuth(provider, externalID, u.ID); err != nil {
		return "", err
	}
	return u.Username, nil
}

func sanitizeGithubLogin(login, externalID string) string {
	u := strings.ToLower(strings.TrimSpace(login))
	if !usernameRe.MatchString(u) {
		u = "gh" + externalID
		if len(u) > 32 {
			u = u[:32]
		}
	}
	return u
}

func randomPassword() string {
	b := make([]byte, 18)
	if _, err := rand.Read(b); err != nil {
		return "fallback-rand-pass"
	}
	return hex.EncodeToString(b)
}

// ---- oidc ----

type oidcDiscovery struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	UserinfoEndpoint      string `json:"userinfo_endpoint"`
}

func (a *API) oidcSettings() (enabled bool, name, issuer, clientID, clientSecret string) {
	return a.store.GetSetting("oidc_enabled") == "1",
		defaultStr(a.store.GetSetting("oidc_name"), "OIDC"),
		strings.TrimRight(a.store.GetSetting("oidc_issuer"), "/"),
		a.store.GetSetting("oidc_client_id"),
		a.store.GetSetting("oidc_client_secret")
}

func (a *API) oidcDiscover(issuer string) (*oidcDiscovery, error) {
	u := issuer + "/.well-known/openid-configuration"
	req, _ := http.NewRequest(http.MethodGet, u, nil)
	req.Header.Set("Accept", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("oidc discovery failed: %d", res.StatusCode)
	}
	var d oidcDiscovery
	if err := json.NewDecoder(res.Body).Decode(&d); err != nil {
		return nil, err
	}
	// 防混淆：校验返回 issuer 与配置一致
	if !strings.EqualFold(strings.TrimRight(d.Issuer, "/"), issuer) {
		return nil, fmt.Errorf("oidc issuer mismatch (configured %s, got %s)", issuer, d.Issuer)
	}
	return &d, nil
}

func (a *API) oidcStart(w http.ResponseWriter, r *http.Request) {
	enabled, name, issuer, id, _ := a.oidcSettings()
	if !enabled || issuer == "" || id == "" {
		writeCode(w, http.StatusNotFound, "oidc_disabled", "oidc login is not enabled")
		return
	}
	d, err := a.oidcDiscover(issuer)
	if err != nil {
		writeCode(w, http.StatusBadGateway, "oidc_discovery_failed", err.Error())
		return
	}
	state, err := newSessionToken()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := a.saveOAuthState(state); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	q := url.Values{}
	q.Set("client_id", id)
	q.Set("response_type", "code")
	q.Set("scope", "openid profile email")
	q.Set("redirect_uri", reqBase(r)+"/api/auth/oidc/callback")
	q.Set("state", state)
	_ = name
	http.Redirect(w, r, d.AuthorizationEndpoint+"?"+q.Encode(), http.StatusFound)
}

func (a *API) oidcCallback(w http.ResponseWriter, r *http.Request) {
	enabled, _, issuer, id, secret := a.oidcSettings()
	fail := func(msg string) {
		http.Redirect(w, r, "/?auth_error="+url.QueryEscape(msg), http.StatusFound)
	}
	if !enabled || issuer == "" || id == "" || secret == "" {
		fail("oidc login is not enabled")
		return
	}
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	if code == "" || state == "" {
		fail("invalid oidc response")
		return
	}
	if !a.checkOAuthState(state) {
		fail("oauth state expired, try again")
		return
	}
	d, err := a.oidcDiscover(issuer)
	if err != nil {
		fail("oidc discovery failed")
		return
	}
	// 换 token
	tf := url.Values{}
	tf.Set("grant_type", "authorization_code")
	tf.Set("code", code)
	tf.Set("redirect_uri", reqBase(r)+"/api/auth/oidc/callback")
	tf.Set("client_id", id)
	tf.Set("client_secret", secret)
	treq, _ := http.NewRequest(http.MethodPost, d.TokenEndpoint, strings.NewReader(tf.Encode()))
	treq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	treq.Header.Set("Accept", "application/json")
	tres, err := http.DefaultClient.Do(treq)
	if err != nil {
		fail("token exchange failed")
		return
	}
	defer func() { _ = tres.Body.Close() }()
	var tok struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err := json.NewDecoder(tres.Body).Decode(&tok); err != nil || tok.AccessToken == "" {
		fail("token exchange failed")
		return
	}
	// userinfo
	ureq, _ := http.NewRequest(http.MethodGet, d.UserinfoEndpoint, nil)
	ureq.Header.Set("Authorization", "Bearer "+tok.AccessToken)
	ures, err := http.DefaultClient.Do(ureq)
	if err != nil {
		fail("userinfo failed")
		return
	}
	defer func() { _ = ures.Body.Close() }()
	if ures.StatusCode != http.StatusOK {
		fail("userinfo failed")
		return
	}
	var ui struct {
		Sub               string `json:"sub"`
		Email             string `json:"email"`
		PreferredUsername string `json:"preferred_username"`
	}
	if err := json.NewDecoder(ures.Body).Decode(&ui); err != nil || ui.Sub == "" {
		fail("invalid userinfo")
		return
	}
	loginHint := ui.PreferredUsername
	if loginHint == "" && ui.Email != "" {
		loginHint = strings.Split(ui.Email, "@")[0]
	}
	username, err := a.loginOrCreateOAuthUser(r, "oidc", ui.Sub, loginHint)
	if err != nil {
		fail("account provisioning failed")
		return
	}
	a.oauthIssueSession(w, r, username)
}

// ---- admin handlers ----

func (a *API) adminAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !a.adminEnabled() {
			writeCode(w, http.StatusNotFound, "admin_disabled", "admin panel is disabled")
			return
		}
		var tok string
		if c, err := r.Cookie(adminCookie); err == nil {
			tok = c.Value
		}
		if tok == "" || bearerToken(r) != "" {
			if bt := bearerToken(r); bt != "" {
				tok = bt
			}
		}
		if tok == "" {
			writeCode(w, http.StatusUnauthorized, "admin_unauthorized", "admin sign in required")
			return
		}
		_, username, err := a.store.GetAdminSession(tok)
		if err != nil {
			writeCode(w, http.StatusUnauthorized, "admin_unauthorized", "admin session expired")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), ctxUser{}, username)))
	}
}

// adminLogin 管理端登录（需先启用管理面板）。
//
//	@Summary     管理端登录
//	@Tags        admin
//	@Accept      json
//	@Produce     json
//	@Param       body body adminLoginReq true "管理员用户名与密码"
//	@Success     200 {object} map[string]string
//	@Failure     401 {object} map[string]string
//	@Failure     404 {object} map[string]string
//	@Failure     429 {object} map[string]string
//	@Router      /admin/login [post]
func (a *API) adminLogin(w http.ResponseWriter, r *http.Request) {
	if bt := bearerToken(r); bt != "" {
		if _, _, err := a.store.ValidatePAT(bt); err == nil {
			writeCode(w, http.StatusForbidden, "pat_not_allowed", "personal access tokens cannot be used for the admin panel")
			return
		}
	}
	if !a.adminEnabled() {
		writeCode(w, http.StatusNotFound, "admin_disabled", "admin panel is disabled (set GITDASH_ADMIN_PASSWORD on first boot)")
		return
	}
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	key := a.rateKey("admin|"+strings.ToLower(strings.TrimSpace(in.Username)), clientIP(r))
	if a.rateBlocked(key) {
		writeCode(w, http.StatusTooManyRequests, "too_many_attempts", "too many attempts, try again later")
		return
	}
	id, hash, err := a.store.AdminAuth(strings.TrimSpace(in.Username))
	if err != nil || bcrypt.CompareHashAndPassword([]byte(hash), []byte(in.Password)) != nil {
		a.rateFail(key)
		writeCode(w, http.StatusUnauthorized, "invalid_credentials", "invalid username or password")
		return
	}
	a.rateReset(key)
	token, err := newSessionToken()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := a.store.CreateAdminSession(token, id); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	http.SetCookie(w, &http.Cookie{Name: adminCookie, Value: token, Path: "/", HttpOnly: true, Secure: r.TLS != nil, SameSite: http.SameSiteLaxMode, MaxAge: 12 * 3600})
	writeJSON(w, http.StatusOK, map[string]string{"username": in.Username})
}

// adminLogout 管理端登出。
//
//	@Summary     管理端登出
//	@Tags        admin
//	@Produce     json
//	@Success     204 {object} nil
//	@Security    BearerAuth
//	@Router      /admin/logout [post]
func (a *API) adminLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(adminCookie); err == nil {
		_ = a.store.DeleteAdminSession(c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: adminCookie, Value: "", Path: "/", HttpOnly: true, MaxAge: -1})
	w.WriteHeader(http.StatusNoContent)
}

// adminMe 获取当前管理员用户名。
//
//	@Summary     当前管理员
//	@Tags        admin
//	@Produce     json
//	@Success     200 {object} map[string]string
//	@Failure     401 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /admin/me [get]
func (a *API) adminMe(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"username": userFrom(r)})
}

// adminSettings 获取管理端系统设置（OAuth/OIDC 配置）。
//
//	@Summary     获取系统设置
//	@Tags        admin
//	@Produce     json
//	@Success     200 {object} map[string]any
//	@Failure     401 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /admin/settings [get]
func (a *API) adminSettings(w http.ResponseWriter, r *http.Request) {
	enabled, id, _ := a.oauthSettings()
	oidcOn := a.store.GetSetting("oidc_enabled") == "1"
	writeJSON(w, http.StatusOK, map[string]any{
		"github_oauth_enabled": enabled,
		"github_client_id":     id,
		"github_has_secret":    a.store.GetSetting("github_client_secret") != "",
		"oidc_enabled":         oidcOn,
		"oidc_name":            a.store.GetSetting("oidc_name"),
		"oidc_issuer":          a.store.GetSetting("oidc_issuer"),
		"oidc_client_id":       a.store.GetSetting("oidc_client_id"),
		"oidc_has_secret":      a.store.GetSetting("oidc_client_secret") != "",
	})
}

// adminSaveSettings 保存管理端系统设置。
//
//	@Summary     保存系统设置
//	@Tags        admin
//	@Accept      json
//	@Produce     json
//	@Param       body body map[string]any true "设置项（github/oidc 开关与配置）"
//	@Success     200 {object} map[string]any
//	@Failure     400 {object} map[string]string
//	@Failure     401 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /admin/settings [post]
func (a *API) adminSaveSettings(w http.ResponseWriter, r *http.Request) {
	var in map[string]any
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	writeStr := func(key string, val any) {
		if v, ok := val.(string); ok {
			_ = a.store.SetSetting(key, strings.TrimSpace(v))
		}
	}
	setBool := func(key string, val any) {
		if v, ok := val.(bool); ok {
			if v {
				_ = a.store.SetSetting(key, "1")
			} else {
				_ = a.store.SetSetting(key, "0")
			}
		}
	}
	setBool("github_oauth_enabled", in["github_oauth_enabled"])
	writeStr("github_client_id", in["github_client_id"])
	if v, ok := in["github_client_secret"].(string); ok && v != "" {
		_ = a.store.SetSetting("github_client_secret", strings.TrimSpace(v))
	}
	setBool("oidc_enabled", in["oidc_enabled"])
	writeStr("oidc_name", in["oidc_name"])
	writeStr("oidc_issuer", in["oidc_issuer"])
	writeStr("oidc_client_id", in["oidc_client_id"])
	if v, ok := in["oidc_client_secret"].(string); ok && v != "" {
		_ = a.store.SetSetting("oidc_client_secret", strings.TrimSpace(v))
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// adminChangePassword 修改管理员密码。
//
//	@Summary     修改管理员密码
//	@Tags        admin
//	@Accept      json
//	@Produce     json
//	@Param       body body adminChangePasswordReq true "当前密码与新密码"
//	@Success     204 {object} nil
//	@Failure     400 {object} map[string]string
//	@Failure     401 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /admin/password [post]
func (a *API) adminChangePassword(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Current string `json:"current_password"`
		New     string `json:"new_password"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	username := userFrom(r)
	id, hash, err := a.store.AdminAuth(username)
	if err != nil || bcrypt.CompareHashAndPassword([]byte(hash), []byte(in.Current)) != nil {
		writeCode(w, http.StatusUnauthorized, "invalid_current_password", "current password is incorrect")
		return
	}
	if len(in.New) < 8 {
		writeCode(w, http.StatusBadRequest, "password_too_short", "password must be at least 8 characters")
		return
	}
	h, err := bcrypt.GenerateFromPassword([]byte(in.New), bcrypt.DefaultCost)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := a.store.UpdateAdminPassword(username, string(h)); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
	_ = id
}

// ---- plumbing ----
