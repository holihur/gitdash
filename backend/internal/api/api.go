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
	"net/http"
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
}

type mfaChallenge struct {
	username string
	expires  time.Time
}

func New(s *store.Store, version string) *API {
	return &API{
		store:      s,
		version:    version,
		mfaPending: map[string]mfaChallenge{},
	}
}

type ctxUser struct{}

func userFrom(r *http.Request) string {
	v, _ := r.Context().Value(ctxUser{}).(string)
	return v
}

func (a *API) Handler(staticDir string) http.Handler {
	mux := http.NewServeMux()

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

	return logMiddleware(mux)
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

func (a *API) startSession(w http.ResponseWriter, status int, username string) {
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
	a.startSession(w, http.StatusCreated, u.Username)
}

func (a *API) login(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	ua, err := a.store.GetByUsername(strings.ToLower(strings.TrimSpace(in.Username)))
	if err != nil {
		writeCode(w, http.StatusUnauthorized, "invalid_credentials", "invalid username or password")
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(ua.PasswordHash), []byte(in.Password)) != nil {
		writeCode(w, http.StatusUnauthorized, "invalid_credentials", "invalid username or password")
		return
	}
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
	a.startSession(w, http.StatusOK, ua.Username)
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
		writeCode(w, http.StatusUnauthorized, "invalid_mfa_code", "invalid authenticator code")
		return
	}
	// 校验通过：令牌一次性作废并签发正式会话
	a.mu.Lock()
	delete(a.mfaPending, in.MFAToken)
	a.mu.Unlock()
	a.startSession(w, http.StatusOK, ua.Username)
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
	if tok := bearerToken(r); tok != "" {
		_ = a.store.DeleteSession(tok)
	}
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
		http.ServeFile(w, r, filepath.Join(dir, "index.html"))
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
		// SPA fallback
		index, err := fs.ReadFile(fsys, "index.html")
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
func (a *API) requireAccess(w http.ResponseWriter, r *http.Request, write bool) (string, string, bool) {
	owner, name, ok := a.resolveTarget(w, r)
	if !ok {
		return "", "", false
	}
	me := userFrom(r)
	can := a.store.CanWrite(owner, name, me)
	if !write {
		can = a.store.CanRead(owner, name, me)
	}
	if !can {
		writeNotFound(w, "repo")
		return "", "", false
	}
	return owner, name, true
}

func (a *API) listRepos(w http.ResponseWriter, r *http.Request) {
	repos, err := a.store.AccessibleRepos(userFrom(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, repos)
}

func (a *API) createRepo(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Template    string `json:"template"` // "" = 空仓库；"readme" = 默认模版（README.md）
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	owner := userFrom(r)
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
	repo, err := a.store.CreateRepo(owner, in.Name, strings.TrimSpace(in.Description))
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
	writeJSON(w, http.StatusOK, repo)
}

func (a *API) deleteRepo(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, false)
	if !ok {
		return
	}
	if userFrom(r) != owner { // 仅仓库所有者可删除
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
	writeJSON(w, http.StatusOK, map[string]any{"path": dir, "entries": entries})
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
	if userFrom(r) != owner {
		writeNotFound(w, "repo")
		return "", "", false
	}
	return owner, name, true
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
		URL string `json:"url"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	in.URL = strings.TrimSpace(in.URL)
	if !validWebhookURL(in.URL) {
		writeCode(w, http.StatusBadRequest, "invalid_url", "url must be a valid http(s) url")
		return
	}
	if len([]rune(in.URL)) > 2048 {
		writeCode(w, http.StatusBadRequest, "invalid_url", "url too long (max 2048 chars)")
		return
	}
	hk, err := a.store.CreateWebhook(owner, name, in.URL)
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
