package api

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/crypto/ssh"

	"gitdash/backend/internal/gitsvc"
	"gitdash/backend/internal/gpgsig"
	"gitdash/backend/internal/store"
	"gitdash/backend/internal/totp"
	"gitdash/backend/internal/webui"
)

var shaRe = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)

var usernameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{1,31}$`)

type API struct {
	store   *store.Store
	version string

	mu         sync.Mutex
	mfaPending map[string]mfaChallenge // mfa_token -> 待二次验证的登录

	rateMu    sync.Mutex
	rateFails map[string]rateRec // 登录失败计数（username|ip）

	oauthMu    sync.Mutex
	oauthState map[string]oauthPending // github oauth state
}

type oauthPending struct {
	expires time.Time
}

// 登录限速：15 分钟窗口内最多 5 次失败
const (
	loginMaxFails   = 5
	loginWindow     = 15 * time.Minute
	loginRateCutoff = 5000
)

type rateRec struct {
	count int
	until time.Time
}

func (a *API) rateKey(username, ip string) string { return username + "|" + ip }

func (a *API) rateBlocked(key string) bool {
	a.rateMu.Lock()
	defer a.rateMu.Unlock()
	if len(a.rateFails) > loginRateCutoff { // 防止内存无限增长
		a.rateFails = map[string]rateRec{}
	}
	rec, ok := a.rateFails[key]
	if !ok {
		return false
	}
	if time.Now().After(rec.until) {
		delete(a.rateFails, key)
		return false
	}
	return rec.count >= loginMaxFails
}

func (a *API) rateFail(key string) {
	a.rateMu.Lock()
	defer a.rateMu.Unlock()
	rec, ok := a.rateFails[key]
	if !ok || time.Now().After(rec.until) {
		rec = rateRec{until: time.Now().Add(loginWindow)}
	}
	rec.count++
	a.rateFails[key] = rec
}

func (a *API) rateReset(key string) {
	a.rateMu.Lock()
	delete(a.rateFails, key)
	a.rateMu.Unlock()
}

func clientIP(r *http.Request) string {
	if h := r.Header.Get("X-Forwarded-For"); h != "" {
		if i := strings.IndexByte(h, ','); i > 0 {
			return strings.TrimSpace(h[:i])
		}
		return strings.TrimSpace(h)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

type mfaChallenge struct {
	username string
	expires  time.Time
	attempts int
}

func New(s *store.Store, version string) *API {
	return &API{
		store:      s,
		version:    version,
		mfaPending: map[string]mfaChallenge{},
		rateFails:  map[string]rateRec{},
		oauthState: map[string]oauthPending{},
	}
}

type ctxUser struct{}

func userFrom(r *http.Request) string {
	v, _ := r.Context().Value(ctxUser{}).(string)
	return v
}

func (a *API) Handler(staticDir string) http.Handler {
	mux := http.NewServeMux()

	// auth providers (public) & github oauth
	mux.HandleFunc("GET /api/auth/providers", a.providers)
	mux.HandleFunc("GET /api/auth/github", a.githubStart)
	mux.HandleFunc("GET /api/auth/github/callback", a.githubCallback)
	mux.HandleFunc("GET /api/auth/oidc/start", a.oidcStart)
	mux.HandleFunc("GET /api/auth/oidc/callback", a.oidcCallback)

	// admin（默认未启用：未引导时一律 404）
	mux.HandleFunc("POST /api/admin/login", a.adminLogin)
	mux.HandleFunc("POST /api/admin/logout", a.adminAuth(a.adminLogout))
	mux.HandleFunc("GET /api/admin/me", a.adminAuth(a.adminMe))
	mux.HandleFunc("GET /api/admin/settings", a.adminAuth(a.adminSettings))
	mux.HandleFunc("POST /api/admin/settings", a.adminAuth(a.adminSaveSettings))
	mux.HandleFunc("POST /api/admin/password", a.adminAuth(a.adminChangePassword))
	// auth
	mux.HandleFunc("POST /api/auth/register", a.register)
	mux.HandleFunc("POST /api/auth/login", a.login)
	mux.HandleFunc("POST /api/auth/mfa-verify", a.mfaVerify)
	mux.HandleFunc("POST /api/auth/logout", a.auth(a.logout))
	mux.HandleFunc("GET /api/me", a.auth(a.me))

	// user profile & mfa
	mux.HandleFunc("POST /api/me/password", a.auth(a.changePassword))
	mux.HandleFunc("GET /api/me/mfa", a.auth(a.mfaStatus))
	mux.HandleFunc("POST /api/me/mfa/enroll", a.auth(a.mfaEnroll))
	mux.HandleFunc("POST /api/me/mfa/activate", a.auth(a.mfaActivate))
	mux.HandleFunc("POST /api/me/mfa/disable", a.auth(a.mfaDisable))

	// repos
	mux.HandleFunc("GET /api/repos", a.auth(a.listRepos))
	mux.HandleFunc("POST /api/repos", a.auth(a.createRepo))
	mux.HandleFunc("GET /api/repos/{name}", a.auth(a.getRepo))
	mux.HandleFunc("DELETE /api/repos/{name}", a.auth(a.deleteRepo))
	mux.HandleFunc("GET /api/repos/{name}/branches", a.auth(a.branches))
	mux.HandleFunc("GET /api/repos/{name}/tree", a.auth(a.tree))
	mux.HandleFunc("GET /api/repos/{name}/blob", a.auth(a.blob))
	mux.HandleFunc("GET /api/repos/{name}/commits", a.auth(a.commits))
	// repos（owner 限定版：供协作者 / 跨用户访问，owner 显式声明）
	mux.HandleFunc("GET /api/users/{owner}/repos/{name}", a.auth(a.getRepo))
	mux.HandleFunc("DELETE /api/users/{owner}/repos/{name}", a.auth(a.deleteRepo))
	mux.HandleFunc("GET /api/users/{owner}/repos/{name}/branches", a.auth(a.branches))
	mux.HandleFunc("GET /api/users/{owner}/repos/{name}/tree", a.auth(a.tree))
	mux.HandleFunc("GET /api/users/{owner}/repos/{name}/blob", a.auth(a.blob))
	mux.HandleFunc("GET /api/users/{owner}/repos/{name}/commits", a.auth(a.commits))

	// star & fork & import
	mux.HandleFunc("GET /api/starred", a.auth(a.listStarred))
	mux.HandleFunc("PUT /api/users/{owner}/repos/{name}/star", a.auth(a.starRepo))
	mux.HandleFunc("DELETE /api/users/{owner}/repos/{name}/star", a.auth(a.unstarRepo))
	mux.HandleFunc("POST /api/users/{owner}/repos/{name}/fork", a.auth(a.forkRepo))
	mux.HandleFunc("POST /api/imports", a.auth(a.importRepo))

	// push mirror（同步到第三方）
	mux.HandleFunc("GET /api/users/{owner}/repos/{name}/mirror", a.auth(a.getMirror))
	mux.HandleFunc("PUT /api/users/{owner}/repos/{name}/mirror", a.auth(a.setMirror))
	mux.HandleFunc("DELETE /api/users/{owner}/repos/{name}/mirror", a.auth(a.deleteMirror))
	mux.HandleFunc("POST /api/users/{owner}/repos/{name}/mirror/sync", a.auth(a.syncMirror))

	// issues
	mux.HandleFunc("GET /api/repos/{name}/issues", a.auth(a.listIssues))
	mux.HandleFunc("POST /api/repos/{name}/issues", a.auth(a.createIssue))
	mux.HandleFunc("PATCH /api/repos/{name}/issues/{number}", a.auth(a.setIssueState))
	mux.HandleFunc("GET /api/users/{owner}/repos/{name}/issues", a.auth(a.listIssues))
	mux.HandleFunc("POST /api/users/{owner}/repos/{name}/issues", a.auth(a.createIssue))
	mux.HandleFunc("PATCH /api/users/{owner}/repos/{name}/issues/{number}", a.auth(a.setIssueState))
	mux.HandleFunc("POST /api/users/{owner}/repos/{name}/issues/{number}/labels", a.auth(a.setIssueLabels))
	mux.HandleFunc("POST /api/users/{owner}/repos/{name}/issues/{number}/milestone", a.auth(a.setIssueMilestone))

	// issue labels & milestones
	mux.HandleFunc("GET /api/users/{owner}/repos/{name}/labels", a.auth(a.listLabels))
	mux.HandleFunc("POST /api/users/{owner}/repos/{name}/labels", a.auth(a.createLabel))
	mux.HandleFunc("PATCH /api/users/{owner}/repos/{name}/labels/{id}", a.auth(a.updateLabel))
	mux.HandleFunc("DELETE /api/users/{owner}/repos/{name}/labels/{id}", a.auth(a.deleteLabel))
	mux.HandleFunc("GET /api/users/{owner}/repos/{name}/milestones", a.auth(a.listMilestones))
	mux.HandleFunc("POST /api/users/{owner}/repos/{name}/milestones", a.auth(a.createMilestone))
	mux.HandleFunc("PATCH /api/users/{owner}/repos/{name}/milestones/{id}", a.auth(a.updateMilestone))
	mux.HandleFunc("DELETE /api/users/{owner}/repos/{name}/milestones/{id}", a.auth(a.deleteMilestone))

	mux.HandleFunc("GET /api/users/{owner}/repos/{name}/commits/{sha}/diff", a.auth(a.commitDiff))
	mux.HandleFunc("POST /api/users/{owner}/repos/{name}/commits", a.auth(a.writeCommit))
	mux.HandleFunc("GET /api/users/{owner}/repos/{name}/tags", a.auth(a.listTags))
	mux.HandleFunc("POST /api/users/{owner}/repos/{name}/refs", a.auth(a.createRef))
	mux.HandleFunc("DELETE /api/users/{owner}/repos/{name}/refs/{kind}/{refname}", a.auth(a.deleteRef))

	// pull requests
	mux.HandleFunc("GET /api/users/{owner}/repos/{name}/pulls", a.auth(a.listPulls))
	mux.HandleFunc("POST /api/users/{owner}/repos/{name}/pulls", a.auth(a.createPull))
	mux.HandleFunc("GET /api/users/{owner}/repos/{name}/pulls/{number}", a.auth(a.getPull))
	mux.HandleFunc("GET /api/users/{owner}/repos/{name}/pulls/{number}/diff", a.auth(a.pullDiff))
	mux.HandleFunc("POST /api/users/{owner}/repos/{name}/pulls/{number}/merge", a.auth(a.mergePull))
	mux.HandleFunc("POST /api/users/{owner}/repos/{name}/pulls/{number}/state", a.auth(a.setPullState))

	// collaborators
	mux.HandleFunc("POST /api/users/{owner}/repos/{name}/visibility", a.auth(a.setRepoVisibility))
	mux.HandleFunc("GET /api/explore/repos", a.auth(a.exploreRepos))

	// orgs (namespace)
	mux.HandleFunc("POST /api/orgs", a.auth(a.createOrg))
	mux.HandleFunc("GET /api/orgs", a.auth(a.listOrgs))
	mux.HandleFunc("GET /api/orgs/{org}", a.auth(a.getOrg))
	mux.HandleFunc("DELETE /api/orgs/{org}", a.auth(a.deleteOrg))
	mux.HandleFunc("GET /api/orgs/{org}/members", a.auth(a.listOrgMembers))
	mux.HandleFunc("POST /api/orgs/{org}/members", a.auth(a.addOrgMember))
	mux.HandleFunc("DELETE /api/orgs/{org}/members/{username}", a.auth(a.removeOrgMember))
	mux.HandleFunc("GET /api/orgs/{org}/repos", a.auth(a.listOrgRepos))
	mux.HandleFunc("GET /api/users/{owner}/repos/{name}/collabs", a.auth(a.listCollabs))
	mux.HandleFunc("POST /api/users/{owner}/repos/{name}/collabs", a.auth(a.addCollab))
	mux.HandleFunc("DELETE /api/users/{owner}/repos/{name}/collabs/{username}", a.auth(a.removeCollab))

	// webhooks
	mux.HandleFunc("GET /api/users/{owner}/repos/{name}/webhooks", a.auth(a.listWebhooks))
	mux.HandleFunc("POST /api/users/{owner}/repos/{name}/webhooks", a.auth(a.createWebhook))
	mux.HandleFunc("DELETE /api/users/{owner}/repos/{name}/webhooks/{id}", a.auth(a.deleteWebhook))

	// ssh keys
	mux.HandleFunc("GET /api/keys", a.auth(a.listKeys))
	mux.HandleFunc("POST /api/keys", a.auth(a.createKey))
	mux.HandleFunc("DELETE /api/keys/{id}", a.auth(a.deleteKey))

	// gpg keys
	mux.HandleFunc("GET /api/gpg", a.auth(a.listGPGKeys))
	mux.HandleFunc("POST /api/gpg", a.auth(a.addGPGKey))
	mux.HandleFunc("DELETE /api/gpg/{id}", a.auth(a.deleteGPGKey))

	// public
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /api/version", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"version": a.version})
	})

	switch {
	case staticDir != "":
		mux.HandleFunc("/", a.staticHandler(staticDir))
	case webui.HasAssets():
		mux.HandleFunc("/", a.embeddedHandler())
	}

	return secureHeaders(logMiddleware(csrfGuard(mux)))
}

// csrfGuard 校验跨站请求：带 Origin 的非安全方法必须与本站同源（cookie 会话的 CSRF 防线）。
func csrfGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions {
			if origin := r.Header.Get("Origin"); origin != "" && origin != "null" {
				u, err := url.Parse(origin)
				if err != nil || !strings.EqualFold(u.Host, r.Host) {
					writeCode(w, http.StatusForbidden, "invalid_origin", "cross-origin request rejected")
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

// secureHeaders 基础安全响应头（CSP 允许内联主题脚本，其 sha256 随 index.html 保持同步）。
func secureHeaders(next http.Handler) http.Handler {
	csp := "default-src 'self'; script-src 'self' 'sha256-3ErQTYhfRUcdQMKUwZWjeUj+0gLQwEdW3gtvOjOALlg=' 'sha256-5N7k7wNTDShVptRxTM9+DDLf2WYyHnUno1d06dT7Cic='; " +
		"style-src 'self' 'unsafe-inline'; img-src 'self' data: blob: https: http:; " +
		"font-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'"
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", csp)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

// ---- auth ----

func (a *API) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		username := a.sessionUser(r)
		if username == "" {
			writeCode(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), ctxUser{}, username)))
	}
}

const sessionCookie = "gitdash_session"

func (a *API) setSessionCookie(w http.ResponseWriter, r *http.Request, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(store.SessionTTL.Seconds()),
	})
}

func (a *API) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", HttpOnly: true, MaxAge: -1})
}

func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if len(h) > 7 && strings.EqualFold(h[:7], "Bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

func (a *API) sessionUser(r *http.Request) string {
	tok := bearerToken(r)
	if tok == "" {
		if c, err := r.Cookie(sessionCookie); err == nil {
			tok = c.Value
		}
	}
	if tok == "" {
		return ""
	}
	username, err := a.store.GetSession(tok)
	if err != nil {
		return ""
	}
	return username
}

func newSessionToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (a *API) startSession(w http.ResponseWriter, r *http.Request, status int, username string) {
	ua, err := a.store.GetByUsername(username)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	token, err := newSessionToken()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := a.store.CreateSession(token, ua.ID); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.setSessionCookie(w, r, token)
	writeJSON(w, status, map[string]string{"token": token, "username": ua.Username})
}

func (a *API) register(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	username := strings.ToLower(strings.TrimSpace(in.Username))
	if a.store.IsOrg(username) { // 组织占用同名命名空间
		writeCode(w, http.StatusConflict, "username_taken", "username is already taken")
		return
	}
	if !usernameRe.MatchString(username) {
		writeCode(w, http.StatusBadRequest, "username_invalid", "username must be 2-32 chars: lowercase letters, digits, '_' or '-', starting alphanumeric")
		return
	}
	if len(in.Password) < 8 {
		writeCode(w, http.StatusBadRequest, "password_too_short", "password must be at least 8 characters")
		return
	}
	if _, err := a.store.GetByUsername(username); err == nil {
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
		writeCode(w, http.StatusConflict, "username_taken", "username is already taken")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.startSession(w, r, http.StatusCreated, u.Username)
}

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

func (a *API) me(w http.ResponseWriter, r *http.Request) {
	ua, err := a.store.GetByUsername(userFrom(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"username":    ua.Username,
		"created_at":  ua.CreatedAt,
		"mfa_enabled": ua.MFAEnabled,
	})
}

// ---- user profile & mfa ----

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
func (a *API) listGPGKeys(w http.ResponseWriter, r *http.Request) {
	keys, err := a.store.ListGPGKeys(userFrom(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, keys)
}

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
	w.WriteHeader(http.StatusNoContent)
}

// oauthIssueSession 为第三方登录用户签发 cookie 会话并跳回前端（不写 JSON）。
func (a *API) oauthIssueSession(w http.ResponseWriter, r *http.Request, username string) {
	if ua, err := a.store.GetByUsername(username); err == nil {
		if token, err := newSessionToken(); err == nil {
			if err := a.store.CreateSession(token, ua.ID); err == nil {
				a.setSessionCookie(w, r, token)
			}
		}
	}
	http.Redirect(w, r, "/", http.StatusFound)
}

// ---- admin & oauth providers ----

const adminCookie = "gitdash_admin"

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
	a.oauthMu.Lock()
	a.oauthState[state] = oauthPending{expires: time.Now().Add(10 * time.Minute)}
	a.oauthMu.Unlock()
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
	a.oauthMu.Lock()
	pending, ok := a.oauthState[state]
	if ok {
		delete(a.oauthState, state)
	}
	a.oauthMu.Unlock()
	if !ok || time.Now().After(pending.expires) {
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
	defer tres.Body.Close()
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
	defer ures.Body.Close()
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
	defer res.Body.Close()
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
	a.oauthMu.Lock()
	a.oauthState[state] = oauthPending{expires: time.Now().Add(10 * time.Minute)}
	a.oauthMu.Unlock()
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
	a.oauthMu.Lock()
	pending, ok := a.oauthState[state]
	if ok {
		delete(a.oauthState, state)
	}
	a.oauthMu.Unlock()
	if !ok || time.Now().After(pending.expires) {
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
	defer tres.Body.Close()
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
	defer ures.Body.Close()
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

func (a *API) adminLogin(w http.ResponseWriter, r *http.Request) {
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

func (a *API) adminLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(adminCookie); err == nil {
		_ = a.store.DeleteAdminSession(c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: adminCookie, Value: "", Path: "/", HttpOnly: true, MaxAge: -1})
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) adminMe(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"username": userFrom(r)})
}

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

func logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rw := &statusWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(rw, r)
		log.Printf("%s %s -> %d", r.Method, r.URL.Path, rw.status)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (a *API) staticHandler(dir string) http.HandlerFunc {
	fs := http.FileServer(http.Dir(dir))
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		p := filepath.Join(dir, filepath.Clean("/"+r.URL.Path))
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			fs.ServeHTTP(w, r)
			return
		}
		fallback := "index.html"
		if strings.HasPrefix(r.URL.Path, "/admin") {
			if _, err := os.Stat(filepath.Join(dir, "admin.html")); err == nil {
				fallback = "admin.html"
			}
		}
		http.ServeFile(w, r, filepath.Join(dir, fallback))
	}
}

func (a *API) embeddedHandler() http.HandlerFunc {
	fsys := webui.Dist()
	fileServer := http.FileServerFS(fsys)
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		p := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if p != "" {
			if f, err := fsys.Open(p); err == nil {
				_ = f.Close()
				fileServer.ServeHTTP(w, r)
				return
			}
		}
		// SPA fallback（/admin 管理端回退 admin.html，其余回 index.html）
		fallback := "index.html"
		if strings.HasPrefix(r.URL.Path, "/admin") {
			if _, err := fsys.Open("admin.html"); err == nil {
				fallback = "admin.html"
			}
		}
		index, err := fs.ReadFile(fsys, fallback)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(index)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// writeCode 返回带稳定错误码的错误（前端可据此 i18n；message 为英文兜底文案）
func writeCode(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]string{"error": msg, "code": code})
}

func writeNotFound(w http.ResponseWriter, resource string) {
	writeCode(w, http.StatusNotFound, "not_found", resource+" not found")
}

func readJSON(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(v); err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_json_body", "invalid json body")
		return err
	}
	return nil
}

// ---- repos ----

// resolveTarget 解析请求目标仓库 (owner, name)。
// 新式路由带 {owner}；旧式单段路由解析为当前用户“自己拥有的或作为协作者可访问的”仓库。
func (a *API) resolveTarget(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	name := r.PathValue("name")
	if owner := r.PathValue("owner"); owner != "" {
		if _, err := a.store.GetRepo(owner, name); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeNotFound(w, "repo")
			} else {
				writeErr(w, http.StatusInternalServerError, err.Error())
			}
			return "", "", false
		}
		return owner, name, true
	}
	me := userFrom(r)
	if o, err := a.store.OwnedByName(me, name); err == nil {
		return o, name, true
	}
	if o, err := a.store.SharedByName(me, name); err == nil {
		return o, name, true
	}
	writeNotFound(w, "repo")
	return "", "", false
}

// requireAccess 校验目标仓库存在且当前用户拥有所需权限（无权限一律 404）。
// 公开仓库（private=false）：任意登录用户可读；写操作仍要求 owner/可写协作者。
func (a *API) requireAccess(w http.ResponseWriter, r *http.Request, write bool) (string, string, bool) {
	owner, name, ok := a.resolveTarget(w, r)
	if !ok {
		return "", "", false
	}
	me := userFrom(r)
	repo, err := a.store.GetRepo(owner, name)
	if err != nil {
		writeNotFound(w, "repo")
		return "", "", false
	}
	can := a.store.CanWrite(owner, name, me)
	if !write {
		can = a.store.CanRead(owner, name, me) || !repo.Private
	}
	if !can {
		writeNotFound(w, "repo")
		return "", "", false
	}
	return owner, name, true
}

// attachStars 批量填充仓库的 star 数与当前用户是否已 star。
func (a *API) attachStars(repos []store.Repo, me string) {
	if len(repos) == 0 {
		return
	}
	pairs := make([][2]string, 0, len(repos))
	for _, r := range repos {
		pairs = append(pairs, [2]string{r.Owner, r.Name})
	}
	counts := a.store.StarCounts(pairs)
	for i := range repos {
		repos[i].Stars = counts[[2]string{repos[i].Owner, repos[i].Name}]
		repos[i].Starred = a.store.IsStarred(me, repos[i].Owner, repos[i].Name)
	}
}

func (a *API) listRepos(w http.ResponseWriter, r *http.Request) {
	me := userFrom(r)
	repos, err := a.store.AccessibleRepos(me)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.attachStars(repos, me)
	writeJSON(w, http.StatusOK, repos)
}

func (a *API) createRepo(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Template    string `json:"template"`  // "" = 空仓库；"readme" = 默认模版（README.md）
		Private     *bool  `json:"private"`   // 默认 true（私有）
		Namespace   string `json:"namespace"` // 可选：组织名（成员可把仓库建到组织下）
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	owner := userFrom(r)
	if ns := strings.TrimSpace(in.Namespace); ns != "" && ns != owner {
		if !a.store.IsOrg(ns) || a.store.OrgRole(ns, owner) == "" {
			writeCode(w, http.StatusForbidden, "org_forbidden", "you are not a member of this organization")
			return
		}
		owner = ns
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Template = strings.ToLower(strings.TrimSpace(in.Template))
	if !gitsvc.ValidName(in.Name) {
		writeCode(w, http.StatusBadRequest, "repo_name_invalid", "invalid name: use letters, digits, '.', '_' or '-' (must start alphanumeric)")
		return
	}
	if in.Template != "" && in.Template != "readme" {
		writeCode(w, http.StatusBadRequest, "invalid_template", "template must be empty or 'readme'")
		return
	}
	if _, err := a.store.GetRepo(owner, in.Name); err == nil || gitsvc.Exists(owner, in.Name) {
		writeCode(w, http.StatusConflict, "repo_exists", "repo already exists")
		return
	}
	private := true
	if in.Private != nil {
		private = *in.Private
	}
	repo, err := a.store.CreateRepo(owner, in.Name, strings.TrimSpace(in.Description), private)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := gitsvc.CreateBare(owner, in.Name); err != nil {
		_ = a.store.DeleteRepo(owner, in.Name)
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if in.Template == "readme" {
		if err := gitsvc.InitReadme(owner, in.Name); err != nil {
			_ = a.store.DeleteRepo(owner, in.Name)
			_ = gitsvc.Delete(owner, in.Name)
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	writeJSON(w, http.StatusCreated, repo)
}

func (a *API) getRepo(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, false)
	if !ok {
		return
	}
	repo, err := a.store.GetRepo(owner, name)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	me := userFrom(r)
	list := []store.Repo{repo}
	a.attachStars(list, me)
	repo = list[0]
	if fo, fr, err := a.store.ForkSource(owner, name); err == nil {
		repo.ForkOwner, repo.ForkRepo = fo, fr
	}
	if iu, err := a.store.ImportSource(owner, name); err == nil {
		repo.ImportURL = iu
	}
	writeJSON(w, http.StatusOK, repo)
}

// ---- star & fork ----

func (a *API) writeStarState(w http.ResponseWriter, owner, name, me string) {
	counts := a.store.StarCounts([][2]string{{owner, name}})
	writeJSON(w, http.StatusOK, map[string]any{
		"starred": a.store.IsStarred(me, owner, name),
		"stars":   counts[[2]string{owner, name}],
	})
}

func (a *API) starRepo(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, false)
	if !ok {
		return
	}
	me := userFrom(r)
	if !a.store.IsStarred(me, owner, name) {
		if err := a.store.StarRepo(me, owner, name); err != nil && !errors.Is(err, store.ErrExists) {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	a.writeStarState(w, owner, name, me)
}

func (a *API) unstarRepo(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, false)
	if !ok {
		return
	}
	me := userFrom(r)
	if err := a.store.UnstarRepo(me, owner, name); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.writeStarState(w, owner, name, me)
}

func (a *API) listStarred(w http.ResponseWriter, r *http.Request) {
	me := userFrom(r)
	repos, err := a.store.StarredRepos(me)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.attachStars(repos, me)
	writeJSON(w, http.StatusOK, repos)
}

func (a *API) forkRepo(w http.ResponseWriter, r *http.Request) {
	srcOwner, srcName, ok := a.requireAccess(w, r, false)
	if !ok {
		return
	}
	var in struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	me := userFrom(r)
	targetOwner := me
	if ns := strings.TrimSpace(in.Namespace); ns != "" && ns != me {
		if !a.store.IsOrg(ns) || a.store.OrgRole(ns, me) == "" {
			writeCode(w, http.StatusForbidden, "org_forbidden", "you are not a member of this organization")
			return
		}
		targetOwner = ns
	}
	targetName := strings.TrimSpace(in.Name)
	if targetName == "" {
		targetName = srcName
	}
	if !gitsvc.ValidName(targetName) {
		writeCode(w, http.StatusBadRequest, "repo_name_invalid", "invalid name: use letters, digits, '.', '_' or '-' (must start alphanumeric)")
		return
	}
	if _, err := a.store.GetRepo(targetOwner, targetName); err == nil || gitsvc.Exists(targetOwner, targetName) {
		writeCode(w, http.StatusConflict, "repo_exists", "repo already exists")
		return
	}
	srcRepo, err := a.store.GetRepo(srcOwner, srcName)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	// fork 保持源仓库可见性（私有源 -> 私有 fork；公开源 -> 公开 fork）
	repo, err := a.store.CreateRepo(targetOwner, targetName, srcRepo.Description, srcRepo.Private)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := gitsvc.ForkRepo(srcOwner, srcName, targetOwner, targetName); err != nil {
		_ = a.store.DeleteRepo(targetOwner, targetName)
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := a.store.SetForkSource(targetOwner, targetName, srcOwner, srcName); err != nil {
		_ = a.store.DeleteRepo(targetOwner, targetName)
		_ = gitsvc.Delete(targetOwner, targetName)
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, repo)
}

// ---- import ----

// validImportURL 校验导入地址并返回规范化后的 URL（http/https/ssh/scp-like）。
func validImportURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("empty url")
	}
	switch {
	case strings.HasPrefix(raw, "http://"), strings.HasPrefix(raw, "https://"):
		u, err := url.Parse(raw)
		if err != nil || u.Hostname() == "" {
			return "", fmt.Errorf("invalid http url")
		}
		if importHostBlocked(u) {
			return "", fmt.Errorf("blocked host")
		}
		return raw, nil
	case strings.HasPrefix(raw, "ssh://"):
		u, err := url.Parse(raw)
		if err != nil || u.Hostname() == "" {
			return "", fmt.Errorf("invalid ssh url")
		}
		if importHostBlocked(u) {
			return "", fmt.Errorf("blocked host")
		}
		return raw, nil
	case strings.HasPrefix(raw, "git://"):
		u, err := url.Parse(raw)
		if err != nil || u.Hostname() == "" {
			return "", fmt.Errorf("invalid git url")
		}
		// git:// 明文且无认证：仅允许回环/内网（本地测试 / 内网 Git 服务器）
		if importHostBlocked(u) || !importHostLoopbackOrPrivate(u) {
			return "", fmt.Errorf("git:// only allowed for loopback/private hosts")
		}
		return raw, nil
	default:
		// scp-like: git@host:path（无 :// 前缀）
		if strings.Contains(raw, "@") && strings.Contains(raw, ":") {
			return raw, nil
		}
		return "", fmt.Errorf("unsupported url scheme")
	}
}

// importHostBlocked 防 SSRF：阻止 link-local / 云元数据网段。
func importHostBlocked(u *url.URL) bool {
	host := u.Hostname()
	if host == "" {
		return true
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return true
	}
	for _, ip := range ips {
		addr, ok := netip.AddrFromSlice(ip)
		if !ok {
			continue
		}
		addr = addr.Unmap()
		if addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() {
			return true
		}
		s := addr.String()
		if addr.IsPrivate() && strings.HasPrefix(s, "169.254.") {
			return true
		}
		if s == "100.100.100.200" {
			return true
		}
	}
	return false
}

func importHostLoopbackOrPrivate(u *url.URL) bool {
	host := u.Hostname()
	ips, err := net.LookupIP(host)
	if err != nil {
		return false
	}
	for _, ip := range ips {
		addr, ok := netip.AddrFromSlice(ip)
		if ok {
			addr = addr.Unmap()
			if addr.IsLoopback() || addr.IsPrivate() {
				return true
			}
		}
	}
	return false
}

// repoNameFromURL 从仓库 URL 推断默认名称（去 .git 后缀取最后一段）。
func repoNameFromURL(raw string) string {
	if u, err := url.Parse(raw); err == nil && u.Path != "" && u.Path != "/" {
		p := strings.TrimSuffix(strings.TrimSuffix(u.Path, "/"), ".git")
		if i := strings.LastIndex(p, "/"); i >= 0 {
			p = p[i+1:]
		}
		return p
	}
	// scp-like fallback: git@host:owner/repo.git
	s := raw
	if i := strings.LastIndex(s, ":"); i >= 0 {
		s = s[i+1:]
	}
	s = strings.TrimSuffix(strings.TrimSuffix(s, "/"), ".git")
	if i := strings.LastIndex(s, "/"); i >= 0 {
		s = s[i+1:]
	}
	return s
}

func (a *API) importRepo(w http.ResponseWriter, r *http.Request) {
	var in struct {
		URL        string `json:"url"`
		Name       string `json:"name"`
		Namespace  string `json:"namespace"`
		Private    *bool  `json:"private"`
		PrivateKey string `json:"private_key"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	raw, err := validImportURL(in.URL)
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_url", "URL must be a valid http(s)/ssh/git repository URL")
		return
	}
	me := userFrom(r)
	targetOwner := me
	if ns := strings.TrimSpace(in.Namespace); ns != "" && ns != me {
		if !a.store.IsOrg(ns) || a.store.OrgRole(ns, me) == "" {
			writeCode(w, http.StatusForbidden, "org_forbidden", "you are not a member of this organization")
			return
		}
		targetOwner = ns
	}
	targetName := strings.TrimSpace(in.Name)
	if targetName == "" {
		targetName = repoNameFromURL(raw)
	}
	if !gitsvc.ValidName(targetName) {
		writeCode(w, http.StatusBadRequest, "repo_name_invalid", "invalid name: use letters, digits, '.', '_' or '-' (must start alphanumeric)")
		return
	}
	if _, err := a.store.GetRepo(targetOwner, targetName); err == nil || gitsvc.Exists(targetOwner, targetName) {
		writeCode(w, http.StatusConflict, "repo_exists", "repo already exists")
		return
	}
	private := true
	if in.Private != nil {
		private = *in.Private
	}
	repo, err := a.store.CreateRepo(targetOwner, targetName, "", private)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := gitsvc.ImportRepo(raw, targetOwner, targetName, in.PrivateKey); err != nil {
		_ = a.store.DeleteRepo(targetOwner, targetName)
		writeCode(w, http.StatusBadRequest, "import_failed", err.Error())
		return
	}
	if err := a.store.SetImportSource(targetOwner, targetName, raw); err != nil {
		_ = a.store.DeleteRepo(targetOwner, targetName)
		_ = gitsvc.Delete(targetOwner, targetName)
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, repo)
}

// ---- push mirror ----

func (a *API) getMirror(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, false)
	if !ok {
		return
	}
	m, err := a.store.GetMirror(owner, name)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"url":        m.URL,
		"created_at": m.CreatedAt,
	})
}

func (a *API) setMirror(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireOwner(w, r)
	if !ok {
		return
	}
	var in struct {
		URL        string `json:"url"`
		PrivateKey string `json:"private_key"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	raw, err := validImportURL(in.URL)
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_url", "URL must be a valid http(s)/ssh/git repository URL")
		return
	}
	if err := a.store.SetMirror(owner, name, raw, strings.TrimSpace(in.PrivateKey)); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	m, err := a.store.GetMirror(owner, name)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"url": m.URL, "created_at": m.CreatedAt})
}

func (a *API) deleteMirror(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireOwner(w, r)
	if !ok {
		return
	}
	if err := a.store.DeleteMirror(owner, name); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) syncMirror(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireOwner(w, r)
	if !ok {
		return
	}
	m, err := a.store.GetMirror(owner, name)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if m.URL == "" {
		writeCode(w, http.StatusBadRequest, "mirror_not_configured", "no mirror target configured")
		return
	}
	if err := gitsvc.PushMirror(owner, name, m.URL, m.PrivateKey); err != nil {
		writeCode(w, http.StatusBadGateway, "mirror_sync_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *API) deleteRepo(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, false)
	if !ok {
		return
	}
	if !a.store.IsRepoOwner(owner, userFrom(r)) { // 仅仓库所有者可删除
		writeNotFound(w, "repo")
		return
	}
	if err := a.store.DeleteRepo(owner, name); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeNotFound(w, "repo")
		} else {
			writeErr(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	_ = gitsvc.Delete(owner, name)
	w.WriteHeader(http.StatusNoContent)
}

// ---- git browsing ----

func (a *API) branches(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, false)
	if !ok {
		return
	}
	bs, err := gitsvc.Branches(owner, name)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, bs)
}

func (a *API) tree(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, false)
	if !ok {
		return
	}
	ref := r.URL.Query().Get("ref")
	dir, err := gitsvc.CleanPath(r.URL.Query().Get("path"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	entries, err := gitsvc.Tree(owner, name, ref, dir)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	// 超大目录分页上限，防止每次请求产生过多子进程
	const maxEntries = 1000
	truncated := len(entries) > maxEntries
	if truncated {
		entries = entries[:maxEntries]
	}
	writeJSON(w, http.StatusOK, map[string]any{"path": dir, "entries": entries, "truncated": truncated})
}

func (a *API) blob(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, false)
	if !ok {
		return
	}
	b, err := gitsvc.ReadBlob(owner, name, r.URL.Query().Get("ref"), r.URL.Query().Get("path"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, b)
}

func (a *API) commits(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, false)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	cs, err := gitsvc.Commits(owner, name, r.URL.Query().Get("ref"), limit)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	// GPG 签名校验（对已注册公钥；失败不影响列表展示）
	keys := []gpgsig.Key{}
	if gks, err := a.store.AllGPGKeys(); err == nil {
		for _, k := range gks {
			keys = append(keys, gpgsig.Key{Username: k.Username, Fingerprint: k.Fingerprint, Armor: k.Armor})
		}
	}
	out := make([]commitResp, 0, len(cs))
	for _, c := range cs {
		r := commitResp{SHA: c.SHA, Author: c.Author, Date: c.Date, Message: c.Message}
		if raw, err := gitsvc.RawCommit(owner, name, c.SHA); err == nil {
			if user, _, ok := gpgsig.VerifyCommit(raw, keys); ok && user != "" {
				r.GPGVerified = user
			}
		}
		out = append(out, r)
	}
	writeJSON(w, http.StatusOK, out)
}

type commitResp struct {
	SHA         string `json:"sha"`
	Author      string `json:"author"`
	Date        string `json:"date"`
	Message     string `json:"message"`
	GPGVerified string `json:"gpg_verified,omitempty"`
}

// ---- issues ----

func (a *API) listIssues(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, false)
	if !ok {
		return
	}
	issues, err := a.store.ListIssues(owner, name)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, a.enrichIssues(owner, name, issues))
}

func (a *API) createIssue(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, true)
	if !ok {
		return
	}
	var in struct {
		Title string `json:"title"`
		Body  string `json:"body"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	title := strings.TrimSpace(in.Title)
	if title == "" {
		writeCode(w, http.StatusBadRequest, "title_required", "title is required")
		return
	}
	if len([]rune(title)) > 200 {
		writeCode(w, http.StatusBadRequest, "title_too_long", "title too long (max 200 chars)")
		return
	}
	if len([]rune(in.Body)) > 10000 {
		writeCode(w, http.StatusBadRequest, "body_too_long", "body too long (max 10000 chars)")
		return
	}
	me := userFrom(r)
	issue, err := a.store.CreateIssue(owner, name, me, title, in.Body)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, a.enrichIssues(owner, name, []store.Issue{issue})[0])
}

func (a *API) setIssueState(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, true)
	if !ok {
		return
	}
	number, err := strconv.ParseInt(r.PathValue("number"), 10, 64)
	if err != nil || number < 1 {
		writeCode(w, http.StatusBadRequest, "invalid_issue_number", "invalid issue number")
		return
	}
	var in struct {
		State string `json:"state"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	if in.State != "open" && in.State != "closed" {
		writeCode(w, http.StatusBadRequest, "invalid_state", "state must be 'open' or 'closed'")
		return
	}
	issue, err := a.store.SetIssueState(owner, name, number, in.State)
	if errors.Is(err, store.ErrNotFound) {
		writeCode(w, http.StatusNotFound, "issue_not_found", "issue not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, a.enrichIssues(owner, name, []store.Issue{issue})[0])
}

func (a *API) listTags(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, false)
	if !ok {
		return
	}
	tags, err := gitsvc.Tags(owner, name)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, tags)
}

func (a *API) createRef(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, true)
	if !ok {
		return
	}
	var in struct {
		Type string `json:"type"`
		Name string `json:"name"`
		From string `json:"from"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	if in.Type != "branch" && in.Type != "tag" {
		writeCode(w, http.StatusBadRequest, "invalid_ref_type", "type must be 'branch' or 'tag'")
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	in.From = strings.TrimSpace(in.From)
	if in.Name == "" {
		writeCode(w, http.StatusBadRequest, "invalid_ref_name", "ref name is required")
		return
	}
	if in.From == "" {
		in.From = "HEAD"
	}
	sha, err := gitsvc.CreateRef(owner, name, in.Type, in.Name, in.From)
	if errors.Is(err, gitsvc.ErrRefExists) {
		writeCode(w, http.StatusConflict, "ref_exists", in.Type+" already exists: "+in.Name)
		return
	}
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_ref_name", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"type": in.Type, "name": in.Name, "sha": sha})
}

func (a *API) deleteRef(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, true)
	if !ok {
		return
	}
	kind := r.PathValue("kind")
	refName := r.PathValue("refname")
	err := gitsvc.DeleteRef(owner, name, kind, refName)
	if errors.Is(err, gitsvc.ErrHeadBranch) {
		writeCode(w, http.StatusConflict, "branch_is_head", "cannot delete the default (HEAD) branch")
		return
	}
	if errors.Is(err, gitsvc.ErrRefNotFound) {
		writeCode(w, http.StatusNotFound, "ref_not_found", kind+" not found")
		return
	}
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_ref_name", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) writeCommit(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, true)
	if !ok {
		return
	}
	var in struct {
		Branch  string              `json:"branch"`
		Message string              `json:"message"`
		Changes []gitsvc.FileChange `json:"changes"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	in.Branch = strings.TrimSpace(in.Branch)
	in.Message = strings.TrimSpace(in.Message)
	if in.Branch == "" {
		in.Branch = "main"
	}
	if in.Message == "" {
		writeCode(w, http.StatusBadRequest, "message_required", "commit message is required")
		return
	}
	if len(in.Changes) == 0 {
		writeCode(w, http.StatusBadRequest, "no_changes", "no file changes provided")
		return
	}
	if len(in.Changes) > 100 {
		writeCode(w, http.StatusBadRequest, "too_many_changes", "too many changes (max 100)")
		return
	}
	total := 0
	for i := range in.Changes {
		c := &in.Changes[i]
		c.Path = strings.TrimSpace(c.Path)
		c.Path = strings.TrimPrefix(c.Path, "/")
		if _, err := gitsvc.CleanPath(c.Path); err != nil {
			writeCode(w, http.StatusBadRequest, "invalid_path", err.Error())
			return
		}
		if c.Action == "" {
			c.Action = "update"
		}
		switch c.Action {
		case "create", "update", "delete", "delete_tree":
		default:
			writeCode(w, http.StatusBadRequest, "invalid_action", "action must be create/update/delete/delete_tree")
			return
		}
		total += len(c.Content)
	}
	if total > 2<<20 { // 2MB
		writeCode(w, http.StatusBadRequest, "content_too_large", "content too large (max 2MB per commit)")
		return
	}
	sha, err := gitsvc.WriteCommit(owner, name, in.Branch, in.Message, userFrom(r), in.Changes)
	if err != nil {
		writeCode(w, http.StatusBadRequest, "commit_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"sha": sha, "branch": in.Branch, "message": in.Message})
}

func (a *API) commitDiff(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, false)
	if !ok {
		return
	}
	sha := r.PathValue("sha")
	if !shaRe.MatchString(sha) {
		writeCode(w, http.StatusBadRequest, "invalid_sha", "invalid commit sha")
		return
	}
	files, patch, err := gitsvc.CommitDiff(owner, name, sha)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"files": files, "patch": patch})
}

// ---- pull requests ----

func (a *API) listPulls(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, false)
	if !ok {
		return
	}
	pulls, err := a.store.ListPulls(owner, name, r.URL.Query().Get("state"))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, pulls)
}

func (a *API) createPull(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, true)
	if !ok {
		return
	}
	var in struct {
		Title        string `json:"title"`
		Body         string `json:"body"`
		SourceBranch string `json:"source_branch"`
		TargetBranch string `json:"target_branch"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	in.Title = strings.TrimSpace(in.Title)
	in.SourceBranch = strings.TrimSpace(strings.TrimPrefix(in.SourceBranch, "refs/heads/"))
	in.TargetBranch = strings.TrimSpace(strings.TrimPrefix(in.TargetBranch, "refs/heads/"))
	if in.Title == "" {
		writeCode(w, http.StatusBadRequest, "title_required", "title is required")
		return
	}
	if in.SourceBranch == "" || in.TargetBranch == "" {
		writeCode(w, http.StatusBadRequest, "branch_not_found", "source and target branches are required")
		return
	}
	if in.SourceBranch == in.TargetBranch {
		writeCode(w, http.StatusBadRequest, "same_branch", "source and target branch must differ")
		return
	}
	srcSHA, err := gitsvc.RevSHA(owner, name, "refs/heads/"+in.SourceBranch)
	if err != nil {
		writeCode(w, http.StatusBadRequest, "branch_not_found", "source branch not found: "+in.SourceBranch)
		return
	}
	baseSHA, err := gitsvc.RevSHA(owner, name, "refs/heads/"+in.TargetBranch)
	if err != nil {
		writeCode(w, http.StatusBadRequest, "branch_not_found", "target branch not found: "+in.TargetBranch)
		return
	}
	pr, err := a.store.CreatePull(owner, name, userFrom(r), in.Title, in.Body, in.SourceBranch, in.TargetBranch, baseSHA, srcSHA)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, pr)
}

func (a *API) getPull(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, false)
	if !ok {
		return
	}
	pr, err := a.getPullOr404(w, owner, name, r.PathValue("number"))
	if err != nil {
		return
	}
	// 实时状态：base 是否仍可快进到 head（对 open 有意义）
	writeJSON(w, http.StatusOK, pr)
}

func (a *API) getPullOr404(w http.ResponseWriter, owner, name, num string) (store.PullRequest, error) {
	n, err := strconv.ParseInt(num, 10, 64)
	if err != nil || n < 1 {
		writeCode(w, http.StatusBadRequest, "invalid_issue_number", "invalid pull request number")
		return store.PullRequest{}, err
	}
	pr, err := a.store.GetPull(owner, name, n)
	if errors.Is(err, store.ErrNotFound) {
		writeCode(w, http.StatusNotFound, "pr_not_found", "pull request not found")
		return store.PullRequest{}, err
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return store.PullRequest{}, err
	}
	return pr, nil
}

func (a *API) pullDiff(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, false)
	if !ok {
		return
	}
	pr, err := a.getPullOr404(w, owner, name, r.PathValue("number"))
	if err != nil {
		return
	}
	base := pr.BaseSHA
	head := pr.HeadSHA
	// open 状态实时取分支 tip（分支可能继续演进）
	if pr.State == "open" {
		if h, err := gitsvc.RevSHA(owner, name, "refs/heads/"+pr.SourceBranch); err == nil {
			head = h
		}
	}
	files, err := gitsvc.DiffStats(owner, name, base, head)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	patch, _ := gitsvc.DiffPatch(owner, name, base, head)
	writeJSON(w, http.StatusOK, map[string]any{"files": files, "patch": patch, "base_sha": base, "head_sha": head})
}

func (a *API) mergePull(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, true)
	if !ok {
		return
	}
	pr, err := a.getPullOr404(w, owner, name, r.PathValue("number"))
	if err != nil {
		return
	}
	if pr.State != "open" {
		writeCode(w, http.StatusConflict, "pull_not_mergeable", "only open pull requests can be merged")
		return
	}
	var in struct {
		Method string `json:"method"` // ""(fast-forward) | merge | squash
	}
	if r.ContentLength > 0 {
		if rerr := readJSON(w, r, &in); rerr != nil {
			return
		}
	}
	var headSHA string
	switch in.Method {
	case "", "fast-forward":
		h, mErr := gitsvc.MergeFastForward(owner, name, pr.TargetBranch, pr.SourceBranch)
		if mErr != nil {
			writeCode(w, http.StatusConflict, "merge_not_ff",
				"branches diverged; merge with method \"merge\" or \"squash\", or rebase locally: "+mErr.Error())
			return
		}
		headSHA = h
	case "merge", "squash":
		msg := fmt.Sprintf("Merge pull request #%d from %s: %s", pr.Number, pr.SourceBranch, pr.Title)
		h, mErr := gitsvc.MergeNonFF(owner, name, pr.TargetBranch, pr.SourceBranch, msg, userFrom(r), in.Method)
		if mErr != nil {
			writeCode(w, http.StatusConflict, "merge_conflict", mErr.Error())
			return
		}
		headSHA = h
	default:
		writeCode(w, http.StatusBadRequest, "invalid_merge_method", "method must be 'merge' or 'squash'")
		return
	}
	merged, err := a.store.MarkPullMerged(owner, name, pr.Number, headSHA, userFrom(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, merged)
}

func (a *API) setPullState(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, true)
	if !ok {
		return
	}
	pr, err := a.getPullOr404(w, owner, name, r.PathValue("number"))
	if err != nil {
		return
	}
	var in struct {
		State string `json:"state"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	if pr.State == "merged" || (in.State != "open" && in.State != "closed") {
		writeCode(w, http.StatusBadRequest, "invalid_state", "state must be open/closed and not merged")
		return
	}
	updated, err := a.store.SetPullState(owner, name, pr.Number, in.State)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// ---- issue labels & milestones ----

func (a *API) enrichIssues(owner, repo string, issues []store.Issue) []map[string]any {
	numbers := make([]int64, 0, len(issues))
	for _, it := range issues {
		numbers = append(numbers, it.Number)
	}
	labels, _ := a.store.IssueLabels(owner, repo, numbers)
	milestones, _ := a.store.IssueMilestones(owner, repo, numbers)
	out := make([]map[string]any, 0, len(issues))
	for _, it := range issues {
		raw, _ := json.Marshal(it)
		var m map[string]any
		_ = json.Unmarshal(raw, &m)
		ls := []store.Label{}
		if v, ok := labels[it.Number]; ok {
			ls = v
		}
		m["labels"] = ls
		if ms, ok := milestones[it.Number]; ok {
			m["milestone"] = ms
		} else {
			m["milestone"] = nil
		}
		out = append(out, m)
	}
	return out
}

func (a *API) parseRepoNumber(w http.ResponseWriter, r *http.Request) (string, string, int64, bool) {
	owner, name, ok := a.requireAccess(w, r, false)
	if !ok {
		return "", "", 0, false
	}
	n, err := strconv.ParseInt(r.PathValue("number"), 10, 64)
	if err != nil || n < 1 {
		writeCode(w, http.StatusBadRequest, "invalid_issue_number", "invalid issue number")
		return "", "", 0, false
	}
	return owner, name, n, true
}

func (a *API) setIssueLabels(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, true)
	if !ok {
		return
	}
	n, err := strconv.ParseInt(r.PathValue("number"), 10, 64)
	if err != nil || n < 1 {
		writeCode(w, http.StatusBadRequest, "invalid_issue_number", "invalid issue number")
		return
	}
	var in struct {
		LabelIDs []int64 `json:"label_ids"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	err = a.store.SetIssueLabels(owner, name, n, in.LabelIDs)
	if errors.Is(err, store.ErrNotFound) {
		writeCode(w, http.StatusNotFound, "issue_not_found", "issue not found")
		return
	}
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_label", err.Error())
		return
	}
	issue, err := a.store.GetPullIssue(owner, name, n)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, a.enrichIssues(owner, name, []store.Issue{issue})[0])
}

func (a *API) setIssueMilestone(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, true)
	if !ok {
		return
	}
	n, err := strconv.ParseInt(r.PathValue("number"), 10, 64)
	if err != nil || n < 1 {
		writeCode(w, http.StatusBadRequest, "invalid_issue_number", "invalid issue number")
		return
	}
	var in struct {
		MilestoneID int64 `json:"milestone_id"` // 0 = 清除
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	err = a.store.SetIssueMilestone(owner, name, n, in.MilestoneID)
	if errors.Is(err, store.ErrNotFound) {
		writeCode(w, http.StatusNotFound, "issue_not_found", "issue not found")
		return
	}
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_milestone", err.Error())
		return
	}
	issue, err := a.store.GetPullIssue(owner, name, n)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, a.enrichIssues(owner, name, []store.Issue{issue})[0])
}

func (a *API) listLabels(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, false)
	if !ok {
		return
	}
	ls, err := a.store.ListLabels(owner, name)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, ls)
}

var labelColorRe = regexp.MustCompile(`^#?[0-9a-fA-F]{6}$`)

func (a *API) createLabel(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, true)
	if !ok {
		return
	}
	var in struct {
		Name  string `json:"name"`
		Color string `json:"color"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Color = strings.TrimSpace(strings.TrimPrefix(in.Color, "#"))
	if in.Name == "" {
		writeCode(w, http.StatusBadRequest, "label_name_required", "label name is required")
		return
	}
	if len([]rune(in.Name)) > 50 {
		writeCode(w, http.StatusBadRequest, "label_name_required", "label name too long (max 50)")
		return
	}
	if in.Color == "" {
		in.Color = "0366d6"
	}
	if !labelColorRe.MatchString(in.Color) {
		writeCode(w, http.StatusBadRequest, "invalid_color", "color must be a hex value like 'ff0000'")
		return
	}
	l, err := a.store.CreateLabel(owner, name, in.Name, in.Color)
	if errors.Is(err, store.ErrExists) {
		writeCode(w, http.StatusConflict, "label_exists", "label already exists")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, l)
}

func (a *API) updateLabel(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, true)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeCode(w, http.StatusBadRequest, "invalid_id", "invalid id")
		return
	}
	var in struct {
		Name  string `json:"name"`
		Color string `json:"color"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Color = strings.TrimSpace(strings.TrimPrefix(in.Color, "#"))
	if in.Name == "" {
		in.Name = "" // keep
	}
	if in.Color != "" && !labelColorRe.MatchString(in.Color) {
		writeCode(w, http.StatusBadRequest, "invalid_color", "color must be a hex value like 'ff0000'")
		return
	}
	if in.Name == "" && in.Color == "" {
		writeCode(w, http.StatusBadRequest, "label_name_required", "provide name or color")
		return
	}
	cur, err := a.store.ListLabels(owner, name)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	var curLabel *store.Label
	for i := range cur {
		if cur[i].ID == id {
			curLabel = &cur[i]
			break
		}
	}
	if curLabel == nil {
		writeCode(w, http.StatusNotFound, "label_not_found", "label not found")
		return
	}
	nn, cc := curLabel.Name, curLabel.Color
	if in.Name != "" {
		nn = in.Name
	}
	if in.Color != "" {
		cc = in.Color
	}
	upd, err := a.store.UpdateLabel(owner, name, id, nn, cc)
	if errors.Is(err, store.ErrNotFound) {
		writeCode(w, http.StatusNotFound, "label_not_found", "label not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, upd)
}

func (a *API) deleteLabel(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, true)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeCode(w, http.StatusBadRequest, "invalid_id", "invalid id")
		return
	}
	if errors.Is(a.store.DeleteLabel(owner, name, id), store.ErrNotFound) {
		writeCode(w, http.StatusNotFound, "label_not_found", "label not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) listMilestones(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, false)
	if !ok {
		return
	}
	ms, err := a.store.ListMilestones(owner, name)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, ms)
}

func (a *API) createMilestone(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, true)
	if !ok {
		return
	}
	var in struct {
		Title       string `json:"title"`
		Description string `json:"description"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	in.Title = strings.TrimSpace(in.Title)
	if in.Title == "" {
		writeCode(w, http.StatusBadRequest, "milestone_title_required", "milestone title is required")
		return
	}
	m, err := a.store.CreateMilestone(owner, name, in.Title, strings.TrimSpace(in.Description))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, m)
}

func (a *API) updateMilestone(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, true)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeCode(w, http.StatusBadRequest, "invalid_id", "invalid id")
		return
	}
	var in struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		State       string `json:"state"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	in.State = strings.TrimSpace(in.State)
	if in.State != "" && in.State != "open" && in.State != "closed" {
		writeCode(w, http.StatusBadRequest, "invalid_state", "state must be open or closed")
		return
	}
	m, err := a.store.UpdateMilestone(owner, name, id, strings.TrimSpace(in.Title), strings.TrimSpace(in.Description), in.State)
	if errors.Is(err, store.ErrNotFound) {
		writeCode(w, http.StatusNotFound, "milestone_not_found", "milestone not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func (a *API) deleteMilestone(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, true)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeCode(w, http.StatusBadRequest, "invalid_id", "invalid id")
		return
	}
	if errors.Is(a.store.DeleteMilestone(owner, name, id), store.ErrNotFound) {
		writeCode(w, http.StatusNotFound, "milestone_not_found", "milestone not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- collaborators ----

// requireOwner 仅仓库所有者可访问（管理协作者等）。
func (a *API) requireOwner(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	owner, name, ok := a.resolveTarget(w, r)
	if !ok {
		return "", "", false
	}
	if !a.store.IsRepoOwner(owner, userFrom(r)) {
		writeNotFound(w, "repo")
		return "", "", false
	}
	return owner, name, true
}

// ---- orgs (namespace) ----

func (a *API) createOrg(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name    string `json:"name"`
		Display string `json:"display"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	in.Name = strings.ToLower(strings.TrimSpace(in.Name))
	if !usernameRe.MatchString(in.Name) {
		writeCode(w, http.StatusBadRequest, "username_invalid", "org name must be 2-32 chars: lowercase letters, digits, '_' or '-', starting alphanumeric")
		return
	}
	o, err := a.store.CreateOrg(in.Name, strings.TrimSpace(in.Display), userFrom(r))
	if errors.Is(err, store.ErrExists) {
		writeCode(w, http.StatusConflict, "org_exists", "organization name already taken")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, o)
}

func (a *API) listOrgs(w http.ResponseWriter, r *http.Request) {
	orgs, err := a.store.ListMyOrgs(userFrom(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := []map[string]any{}
	for _, o := range orgs {
		out = append(out, map[string]any{
			"name": o.Name, "display": o.Display, "created_at": o.CreatedAt,
			"role": a.store.OrgRole(o.Name, userFrom(r)),
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (a *API) getOrg(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	role := a.store.OrgRole(org, userFrom(r))
	if role == "" {
		writeCode(w, http.StatusNotFound, "org_not_found", "organization not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"name": org, "role": role})
}

func (a *API) deleteOrg(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	if a.store.OrgRole(org, userFrom(r)) != "owner" {
		writeCode(w, http.StatusNotFound, "org_not_found", "organization not found")
		return
	}
	if err := a.store.DeleteOrg(org); err != nil {
		if strings.Contains(err.Error(), "not empty") {
			writeCode(w, http.StatusConflict, "org_not_empty", "delete or move all repositories before deleting the organization")
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) listOrgMembers(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	if a.store.OrgRole(org, userFrom(r)) == "" {
		writeCode(w, http.StatusNotFound, "org_not_found", "organization not found")
		return
	}
	members, err := a.store.OrgMembers(org)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, members)
}

func (a *API) addOrgMember(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	if a.store.OrgRole(org, userFrom(r)) != "owner" {
		writeCode(w, http.StatusNotFound, "org_not_found", "organization not found")
		return
	}
	var in struct {
		Username string `json:"username"`
		Role     string `json:"role"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	in.Username = strings.ToLower(strings.TrimSpace(in.Username))
	if in.Role == "" {
		in.Role = "member"
	}
	if in.Role != "member" && in.Role != "owner" {
		writeCode(w, http.StatusBadRequest, "invalid_permission", "role must be member or owner")
		return
	}
	if _, err := a.store.GetByUsername(in.Username); err != nil {
		writeCode(w, http.StatusNotFound, "user_not_found", "user not found")
		return
	}
	if err := a.store.AddOrgMember(org, in.Username, in.Role); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"org": org, "username": in.Username, "role": in.Role})
}

func (a *API) removeOrgMember(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	me := userFrom(r)
	if a.store.OrgRole(org, me) != "owner" {
		writeCode(w, http.StatusNotFound, "org_not_found", "organization not found")
		return
	}
	target := strings.ToLower(strings.TrimSpace(r.PathValue("username")))
	if target == me {
		writeCode(w, http.StatusBadRequest, "cannot_remove_self", "transfer ownership or delete the organization instead")
		return
	}
	if a.store.OrgRole(org, target) == "owner" {
		// 仅剩一个 owner 时不允许移除 owner
		members, _ := a.store.OrgMembers(org)
		owners := 0
		for _, m := range members {
			if m.Role == "owner" {
				owners++
			}
		}
		if owners <= 1 {
			writeCode(w, http.StatusBadRequest, "last_owner", "organization must keep at least one owner")
			return
		}
	}
	if errors.Is(a.store.RemoveOrgMember(org, target), store.ErrNotFound) {
		writeCode(w, http.StatusNotFound, "member_not_found", "member not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) listOrgRepos(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	me := userFrom(r)
	role := a.store.OrgRole(org, me)
	if role == "" {
		writeCode(w, http.StatusNotFound, "org_not_found", "organization not found")
		return
	}
	rows, err := a.store.QueryOrgRepos(org)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	label := "read"
	if role == "owner" {
		label = "owner"
	} else if role == "member" {
		label = "write"
	}
	out := []store.Repo{}
	for _, rp := range rows {
		out = append(out, rp)
	}
	a.attachStars(out, me)
	writeJSON(w, http.StatusOK, map[string]any{"role": label, "repos": out})
}

func (a *API) setRepoVisibility(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireOwner(w, r)
	if !ok {
		return
	}
	var in struct {
		Private bool `json:"private"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	if err := a.store.SetRepoPrivate(owner, name, in.Private); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	repo, err := a.store.GetRepo(owner, name)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, repo)
}

func (a *API) exploreRepos(w http.ResponseWriter, r *http.Request) {
	repos, err := a.store.ExploreRepos()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	// 过滤掉自己仓库，便于发现他人公开项目
	me := userFrom(r)
	out := []store.Repo{}
	for _, rp := range repos {
		if rp.Owner != me {
			out = append(out, rp)
		}
	}
	a.attachStars(out, me)
	writeJSON(w, http.StatusOK, out)
}

func (a *API) listCollabs(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireOwner(w, r)
	if !ok {
		return
	}
	collabs, err := a.store.ListCollabs(owner, name)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, collabs)
}

func (a *API) addCollab(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireOwner(w, r)
	if !ok {
		return
	}
	var in struct {
		Username   string `json:"username"`
		Permission string `json:"permission"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	in.Username = strings.ToLower(strings.TrimSpace(in.Username))
	if !usernameRe.MatchString(in.Username) {
		writeCode(w, http.StatusBadRequest, "username_invalid", "invalid collaborator username")
		return
	}
	if in.Username == userFrom(r) {
		writeCode(w, http.StatusBadRequest, "owner_as_collab", "owner is already the owner")
		return
	}
	if in.Permission != "read" && in.Permission != "write" {
		writeCode(w, http.StatusBadRequest, "invalid_permission", "permission must be 'read' or 'write'")
		return
	}
	if _, err := a.store.GetByUsername(in.Username); err != nil {
		writeCode(w, http.StatusNotFound, "user_not_found", "user not found")
		return
	}
	if err := a.store.UpsertCollab(owner, name, in.Username, in.Permission); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	collabs, err := a.store.ListCollabs(owner, name)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	for _, c := range collabs {
		if c.Username == in.Username {
			writeJSON(w, http.StatusOK, c)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"username": in.Username, "permission": in.Permission})
}

func (a *API) removeCollab(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireOwner(w, r)
	if !ok {
		return
	}
	username := strings.ToLower(strings.TrimSpace(r.PathValue("username")))
	err := a.store.RemoveCollab(owner, name, username)
	if errors.Is(err, store.ErrNotFound) {
		writeCode(w, http.StatusNotFound, "collab_not_found", "collaborator not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- webhooks ----

func validWebhookURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

func (a *API) listWebhooks(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireOwner(w, r)
	if !ok {
		return
	}
	hooks, err := a.store.ListWebhooks(owner, name)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, hooks)
}

func (a *API) createWebhook(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireOwner(w, r)
	if !ok {
		return
	}
	var in struct {
		URL    string `json:"url"`
		Secret string `json:"secret"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	in.URL = strings.TrimSpace(in.URL)
	in.Secret = strings.TrimSpace(in.Secret)
	if !validWebhookURL(in.URL) {
		writeCode(w, http.StatusBadRequest, "invalid_url", "url must be a valid http(s) url")
		return
	}
	if len([]rune(in.URL)) > 2048 {
		writeCode(w, http.StatusBadRequest, "invalid_url", "url too long (max 2048 chars)")
		return
	}
	if in.Secret != "" && len(in.Secret) < 16 {
		writeCode(w, http.StatusBadRequest, "invalid_secret", "webhook secret must be at least 16 characters")
		return
	}
	hk, err := a.store.CreateWebhook(owner, name, in.URL, in.Secret)
	if errors.Is(err, store.ErrExists) {
		writeCode(w, http.StatusConflict, "webhook_exists", "webhook already registered")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, hk)
}

func (a *API) deleteWebhook(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireOwner(w, r)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeCode(w, http.StatusBadRequest, "invalid_id", "invalid id")
		return
	}
	if errors.Is(a.store.DeleteWebhook(owner, name, id), store.ErrNotFound) {
		writeCode(w, http.StatusNotFound, "webhook_not_found", "webhook not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- ssh keys ----

func (a *API) listKeys(w http.ResponseWriter, r *http.Request) {
	keys, err := a.store.ListKeys(userFrom(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, keys)
}

func (a *API) createKey(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name      string `json:"name"`
		PublicKey string `json:"public_key"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	pub := strings.TrimSpace(in.PublicKey)
	if in.Name == "" || pub == "" {
		writeCode(w, http.StatusBadRequest, "key_name_required", "name and public_key are required")
		return
	}
	key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(pub))
	if err != nil {
		writeCode(w, http.StatusBadRequest, "key_invalid", "invalid public key: "+err.Error())
		return
	}
	pub = strings.Join(strings.Fields(string(bytes.TrimSpace(ssh.MarshalAuthorizedKey(key)))), " ")
	k, err := a.store.CreateKey(userFrom(r), in.Name, pub, ssh.FingerprintSHA256(key))
	if errors.Is(err, store.ErrExists) {
		writeCode(w, http.StatusConflict, "key_exists", "this key is already registered")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, k)
}

func (a *API) deleteKey(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_id", "invalid id")
		return
	}
	if errors.Is(a.store.DeleteKey(userFrom(r), id), store.ErrNotFound) {
		writeNotFound(w, "ssh key")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
