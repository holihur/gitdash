package api

import (
	"context"
	"net/http"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

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
		internalError(w, err)
		return
	}
	if err := a.store.CreateAdminSession(token, id); err != nil {
		internalError(w, err)
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
	googleOn, googleID, _ := a.googleSettings()
	oidcOn := a.store.GetSetting("oidc_enabled") == "1"
	writeJSON(w, http.StatusOK, map[string]any{
		"github_oauth_enabled": enabled,
		"github_client_id":     id,
		"github_has_secret":    a.store.GetSetting("github_client_secret") != "",
		"google_oauth_enabled": googleOn,
		"google_client_id":     googleID,
		"google_has_secret":    a.store.GetSetting("google_client_secret") != "",
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
//	@Param       body body map[string]any true "设置项（github/google/oidc 开关与配置）"
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
	setBool("google_oauth_enabled", in["google_oauth_enabled"])
	writeStr("google_client_id", in["google_client_id"])
	if v, ok := in["google_client_secret"].(string); ok && v != "" {
		_ = a.store.SetSetting("google_client_secret", strings.TrimSpace(v))
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
	if code, msg := passwordIssue(in.New); code != "" {
		writeCode(w, http.StatusBadRequest, code, msg)
		return
	}
	h, err := bcrypt.GenerateFromPassword([]byte(in.New), bcrypt.DefaultCost)
	if err != nil {
		internalError(w, err)
		return
	}
	if err := a.store.UpdateAdminPassword(username, string(h)); err != nil {
		internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
	_ = id
}
