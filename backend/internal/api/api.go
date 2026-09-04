package api

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
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

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/crypto/ssh"

	"gitdash/backend/internal/gitsvc"
	"gitdash/backend/internal/store"
	"gitdash/backend/internal/webui"
)

var usernameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{1,31}$`)

type API struct {
	store   *store.Store
	version string
}

func New(s *store.Store, version string) *API {
	return &API{store: s, version: version}
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
	mux.HandleFunc("POST /api/auth/logout", a.auth(a.logout))
	mux.HandleFunc("GET /api/me", a.auth(a.me))

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
	a.startSession(w, http.StatusOK, ua.Username)
}

func (a *API) logout(w http.ResponseWriter, r *http.Request) {
	if tok := bearerToken(r); tok != "" {
		_ = a.store.DeleteSession(tok)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) me(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"username": userFrom(r)})
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
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	owner := userFrom(r)
	in.Name = strings.TrimSpace(in.Name)
	if !gitsvc.ValidName(in.Name) {
		writeCode(w, http.StatusBadRequest, "repo_name_invalid", "invalid name: use letters, digits, '.', '_' or '-' (must start alphanumeric)")
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
	writeJSON(w, http.StatusOK, cs)
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
	writeJSON(w, http.StatusOK, issues)
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
	writeJSON(w, http.StatusCreated, issue)
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
	writeJSON(w, http.StatusOK, issue)
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
