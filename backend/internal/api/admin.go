package api

import (
	"context"
	"net/http"
	"strings"

	"gitdash/backend/internal/codesearch"
	"gitdash/backend/internal/jobs"
	"gitdash/backend/internal/metrics"

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

// adminCodeSearch 代码搜索 / 索引运行态指标（聚合，不含仓库维度）。
//
//	@Summary     代码搜索指标
//	@Description 返回检索来源计数、索引重建次数/耗时/全量增量、索引仓库与文档规模、待重建仓库数。
//	@Tags        admin
//	@Produce     json
//	@Success     200 {object} object
//	@Security    BearerAuth
//	@Router      /admin/codesearch [get]
func (a *API) adminCodeSearch(w http.ResponseWriter, r *http.Request) {
	c := metrics.SnapshotCodeSearch()
	avg := 0.0
	if c.IndexRuns > 0 {
		avg = float64(c.IndexDurMs) / float64(c.IndexRuns)
	}
	_, isRemote := a.codeSearch.(*codesearch.Remote)
	resp := map[string]any{
		"backend":               codesearch.ModeGrep,
		"remote":                isRemote,
		"search_requests":       map[string]any{"index": c.SearchIndex, "grep": c.SearchGrep, "indexing": c.SearchIndexing},
		"index_runs":            map[string]any{"full": c.IndexFull, "incremental": c.IndexIncr, "ok": c.IndexOK, "error": c.IndexErr},
		"index_avg_duration_ms": avg,
		"last_index_at":         c.LastIndexAt,
		"index":                 map[string]any{"repos": 0, "documents": 0, "dirty": 0},
	}
	if sr, ok := a.codeSearch.(codesearch.StatsReporter); ok {
		st, err := sr.IndexStats(r.Context())
		if err != nil {
			resp["index_error"] = err.Error()
		}
		resp["backend"] = st.Backend
		resp["index"] = map[string]any{"repos": st.Repos, "documents": st.Documents, "dirty": st.Dirty}
	}
	writeJSON(w, http.StatusOK, resp)
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
		if _, _, err := a.store.ValidatePAT(bt, ""); err == nil {
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
	http.SetCookie(w, &http.Cookie{Name: adminCookie, Value: token, Path: "/", HttpOnly: true, Secure: r.TLS != nil || forceSecureCookies, SameSite: http.SameSiteLaxMode, MaxAge: 12 * 3600})
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
		// 访问控制开关（默认开启，管理端可关闭）
		"swagger_enabled":        a.store.GetSetting("swagger_enabled") != "0",
		"password_login_enabled": a.store.GetSetting("password_login_enabled") != "0",
		"version_visible":        a.store.GetSetting("version_visible") != "0",
		"registration_disabled":  a.store.GetSetting("registration_disabled") == "1",
		// 代码成分（语言）分析：默认开启，可关闭以省资源。
		"language_stats_enabled": a.store.GetSetting(jobs.SettingLanguageStats) != "0",
		"smtp_enabled":           a.store.GetSetting("smtp_enabled") == "1",
		"smtp_host":              a.store.GetSetting("smtp_host"),
		"smtp_port":              a.store.GetSetting("smtp_port"),
		"smtp_user":              a.store.GetSetting("smtp_user"),
		"smtp_from":              a.store.GetSetting("smtp_from"),
		"smtp_has_pass":          a.store.GetSetting("smtp_pass") != "",
		"feedback_enabled":       a.store.GetSetting("feedback_enabled") == "1",
		"feedback_repo":          a.store.GetSetting("feedback_repo"),
		"feedback_has_token":     a.store.GetSetting("feedback_token") != "",
		"feedback_local":         a.feedbackSelfTarget(a.store.GetSetting("feedback_repo")),
		"gitlab_enabled":         a.store.GetSetting("gitlab_enabled") == "1",
		"gitlab_client_id":       a.store.GetSetting("gitlab_client_id"),
		"gitlab_has_secret":      a.store.GetSetting("gitlab_client_secret") != "",
		"gitlab_base_url":        a.store.GetSetting("gitlab_base_url"),
		"gitea_enabled":          a.store.GetSetting("gitea_enabled") == "1",
		"gitea_client_id":        a.store.GetSetting("gitea_client_id"),
		"gitea_has_secret":       a.store.GetSetting("gitea_client_secret") != "",
		"gitea_base_url":         a.store.GetSetting("gitea_base_url"),
		"bitbucket_enabled":      a.store.GetSetting("bitbucket_enabled") == "1",
		"bitbucket_client_id":    a.store.GetSetting("bitbucket_client_id"),
		"bitbucket_has_secret":   a.store.GetSetting("bitbucket_client_secret") != "",
		"docs_url":               a.store.GetSetting("docs_url"),
		// 全站通知（公告条）：管理端配置，登录页与已登录界面均展示。
		"announcement_enabled": a.store.GetSetting("announcement_enabled") == "1",
		"announcement_level":   a.store.GetSetting("announcement_level"),
		"announcement_title":   a.store.GetSetting("announcement_title"),
		"announcement_message": a.store.GetSetting("announcement_message"),
	})
}

// adminSaveSettings 保存管理端系统设置。
//
//	@Summary     保存系统设置
//	@Tags        admin
//	@Accept      json
//	@Produce     json
//	@Param       body body map[string]any true "设置项（github/google/oidc/smtp/feedback 开关与配置）"
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
	// 访问控制开关：仅当请求显式携带字段时更新，避免旧客户端保存其它设置时误重置。
	setBool("swagger_enabled", in["swagger_enabled"])
	setBool("password_login_enabled", in["password_login_enabled"])
	setBool("version_visible", in["version_visible"])
	setBool("registration_disabled", in["registration_disabled"])
	// 代码成分分析开关：从关闭变为开启时，异步回填尚未分析的仓库。
	langBefore := a.store.GetSetting(jobs.SettingLanguageStats) != "0"
	setBool(jobs.SettingLanguageStats, in[jobs.SettingLanguageStats])
	langAfter := a.store.GetSetting(jobs.SettingLanguageStats) != "0"
	if !langBefore && langAfter && a.jobsMgr != nil {
		a.jobsMgr.BackfillLanguages()
	}
	writeStr("oidc_name", in["oidc_name"])
	writeStr("oidc_issuer", in["oidc_issuer"])
	writeStr("oidc_client_id", in["oidc_client_id"])
	if v, ok := in["oidc_client_secret"].(string); ok && v != "" {
		_ = a.store.SetSetting("oidc_client_secret", strings.TrimSpace(v))
	}
	setBool("smtp_enabled", in["smtp_enabled"])
	writeStr("smtp_host", in["smtp_host"])
	writeStr("smtp_port", in["smtp_port"])
	writeStr("smtp_user", in["smtp_user"])
	writeStr("smtp_from", in["smtp_from"])
	if v, ok := in["smtp_pass"].(string); ok && v != "" {
		_ = a.store.SetSetting("smtp_pass", v)
	}
	// 反馈：开启前校验仓库地址格式，避免保存后再由用户请求时才报错。
	if enable, ok := in["feedback_enabled"].(bool); ok && enable {
		repoVal, _ := in["feedback_repo"].(string)
		if repoVal == "" {
			repoVal = a.store.GetSetting("feedback_repo")
		}
		if _, _, _, valid := feedbackRepoFromURL(repoVal); !valid {
			writeCode(w, http.StatusBadRequest, "invalid_feedback_repo", "feedback repository must look like https://host/owner/repo")
			return
		}
		hasToken := a.store.GetSetting("feedback_token") != ""
		if v, ok := in["feedback_token"].(string); ok && strings.TrimSpace(v) != "" {
			hasToken = true
		}
		if !hasToken && !a.feedbackSelfTarget(repoVal) {
			writeCode(w, http.StatusBadRequest, "feedback_token_required", "feedback access token is required")
			return
		}
	}
	setBool("feedback_enabled", in["feedback_enabled"])
	writeStr("feedback_repo", in["feedback_repo"])
	if v, ok := in["feedback_token"].(string); ok && v != "" {
		_ = a.store.SetSetting("feedback_token", strings.TrimSpace(v))
	}
	// 第三方账号绑定（批量导入）：GitLab / Gitea / Bitbucket。
	writeStr("gitlab_base_url", in["gitlab_base_url"])
	setBool("gitlab_enabled", in["gitlab_enabled"])
	writeStr("gitlab_client_id", in["gitlab_client_id"])
	if v, ok := in["gitlab_client_secret"].(string); ok && v != "" {
		_ = a.store.SetSetting("gitlab_client_secret", strings.TrimSpace(v))
	}
	writeStr("gitea_base_url", in["gitea_base_url"])
	setBool("gitea_enabled", in["gitea_enabled"])
	writeStr("gitea_client_id", in["gitea_client_id"])
	if v, ok := in["gitea_client_secret"].(string); ok && v != "" {
		_ = a.store.SetSetting("gitea_client_secret", strings.TrimSpace(v))
	}
	setBool("bitbucket_enabled", in["bitbucket_enabled"])
	writeStr("bitbucket_client_id", in["bitbucket_client_id"])
	if v, ok := in["bitbucket_client_secret"].(string); ok && v != "" {
		_ = a.store.SetSetting("bitbucket_client_secret", strings.TrimSpace(v))
	}
	writeStr("docs_url", in["docs_url"])
	// 全站通知：标题/正文/级别与开关。内容指纹由公开接口计算，无需额外存储。
	setBool("announcement_enabled", in["announcement_enabled"])
	writeStr("announcement_level", in["announcement_level"])
	writeStr("announcement_title", in["announcement_title"])
	writeStr("announcement_message", in["announcement_message"])
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// adminSMTPTest 使用当前 SMTP 配置发送一封测试邮件。
//
//	@Summary     发送 SMTP 测试邮件
//	@Description 使用当前生效的 SMTP 配置向指定邮箱发送测试邮件，用于校验配置。
//	@Tags        admin
//	@Accept      json
//	@Produce     json
//	@Param       body body map[string]string true "to"
//	@Success     200 {object} map[string]any
//	@Failure     400 {object} map[string]string
//	@Failure     502 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /admin/smtp/test [post]
func (a *API) adminSMTPTest(w http.ResponseWriter, r *http.Request) {
	var in struct {
		To string `json:"to"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	to := strings.TrimSpace(in.To)
	if to == "" || !emailRe.MatchString(to) {
		writeCode(w, http.StatusBadRequest, "invalid_email", "a valid recipient email is required")
		return
	}
	if !a.emailReady() {
		writeCode(w, http.StatusBadRequest, "smtp_not_configured", "SMTP is not configured")
		return
	}
	body := "This is a test email from your gitdash instance.\n\nIf you received it, SMTP is configured correctly.\n\n-- gitdash"
	if err := a.emailSender.Send(to, "gitdash: SMTP test", body); err != nil {
		writeCode(w, http.StatusBadGateway, "smtp_send_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sent": true})
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
	h, err := bcrypt.GenerateFromPassword([]byte(in.New), BcryptCost)
	if err != nil {
		internalError(w, err)
		return
	}
	if err := a.store.UpdateAdminPassword(username, string(h)); err != nil {
		internalError(w, err)
		return
	}
	// 改密后摧销其它管理端会话，仅保留当前会话（与用户改密语义一致）。
	token := ""
	if c, err := r.Cookie(adminCookie); err == nil {
		token = c.Value
	}
	if bt := bearerToken(r); bt != "" {
		token = bt
	}
	_ = a.store.DeleteAdminSessionsExcept(id, token)
	w.WriteHeader(http.StatusNoContent)
}
