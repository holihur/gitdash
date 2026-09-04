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

	// issues
	mux.HandleFunc("GET /api/repos/{name}/issues", a.auth(a.listIssues))
	mux.HandleFunc("POST /api/repos/{name}/issues", a.auth(a.createIssue))
	mux.HandleFunc("PATCH /api/repos/{name}/issues/{number}", a.auth(a.setIssueState))

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
			writeErr(w, http.StatusUnauthorized, "unauthorized")
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
		writeErr(w, http.StatusBadRequest, "用户名需 2-32 位小写字母/数字，可含 _ -，字母或数字开头")
		return
	}
	if len(in.Password) < 8 {
		writeErr(w, http.StatusBadRequest, "密码至少 8 位")
		return
	}
	if _, err := a.store.GetByUsername(username); err == nil {
		writeErr(w, http.StatusConflict, "用户名已被注册")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	u, err := a.store.CreateUser(username, string(hash))
	if errors.Is(err, store.ErrExists) {
		writeErr(w, http.StatusConflict, "用户名已被注册")
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
		writeErr(w, http.StatusUnauthorized, "用户名或密码错误")
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(ua.PasswordHash), []byte(in.Password)) != nil {
		writeErr(w, http.StatusUnauthorized, "用户名或密码错误")
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

func readJSON(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(v); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json body")
		return err
	}
	return nil
}

// ---- repos ----

func (a *API) listRepos(w http.ResponseWriter, r *http.Request) {
	repos, err := a.store.ListRepos(userFrom(r))
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
		writeErr(w, http.StatusBadRequest, "invalid name: use letters, digits, '.', '_' or '-' (must start alphanumeric)")
		return
	}
	if _, err := a.store.GetRepo(owner, in.Name); err == nil || gitsvc.Exists(owner, in.Name) {
		writeErr(w, http.StatusConflict, "repo already exists")
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
	repo, err := a.store.GetRepo(userFrom(r), r.PathValue("name"))
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "repo not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, repo)
}

func (a *API) deleteRepo(w http.ResponseWriter, r *http.Request) {
	if err := a.store.DeleteRepo(userFrom(r), r.PathValue("name")); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = gitsvc.Delete(userFrom(r), r.PathValue("name"))
	w.WriteHeader(http.StatusNoContent)
}

// ---- git browsing ----

func (a *API) branches(w http.ResponseWriter, r *http.Request) {
	owner, name := userFrom(r), r.PathValue("name")
	if !gitsvc.Exists(owner, name) {
		writeErr(w, http.StatusNotFound, "repo not found")
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
	owner, name := userFrom(r), r.PathValue("name")
	if !gitsvc.Exists(owner, name) {
		writeErr(w, http.StatusNotFound, "repo not found")
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
	owner, name := userFrom(r), r.PathValue("name")
	if !gitsvc.Exists(owner, name) {
		writeErr(w, http.StatusNotFound, "repo not found")
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
	owner, name := userFrom(r), r.PathValue("name")
	if !gitsvc.Exists(owner, name) {
		writeErr(w, http.StatusNotFound, "repo not found")
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

// requireRepo 校验仓库存在且归当前用户所有。
func (a *API) requireRepo(w http.ResponseWriter, r *http.Request) bool {
	_, err := a.store.GetRepo(userFrom(r), r.PathValue("name"))
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "repo not found")
		return false
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return false
	}
	return true
}

func (a *API) listIssues(w http.ResponseWriter, r *http.Request) {
	if !a.requireRepo(w, r) {
		return
	}
	issues, err := a.store.ListIssues(userFrom(r), r.PathValue("name"))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, issues)
}

func (a *API) createIssue(w http.ResponseWriter, r *http.Request) {
	if !a.requireRepo(w, r) {
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
		writeErr(w, http.StatusBadRequest, "title is required")
		return
	}
	if len([]rune(title)) > 200 {
		writeErr(w, http.StatusBadRequest, "title too long (max 200 chars)")
		return
	}
	if len([]rune(in.Body)) > 10000 {
		writeErr(w, http.StatusBadRequest, "body too long (max 10000 chars)")
		return
	}
	issue, err := a.store.CreateIssue(userFrom(r), r.PathValue("name"), userFrom(r), title, in.Body)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, issue)
}

func (a *API) setIssueState(w http.ResponseWriter, r *http.Request) {
	if !a.requireRepo(w, r) {
		return
	}
	number, err := strconv.ParseInt(r.PathValue("number"), 10, 64)
	if err != nil || number < 1 {
		writeErr(w, http.StatusBadRequest, "invalid issue number")
		return
	}
	var in struct {
		State string `json:"state"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	if in.State != "open" && in.State != "closed" {
		writeErr(w, http.StatusBadRequest, "state must be 'open' or 'closed'")
		return
	}
	issue, err := a.store.SetIssueState(userFrom(r), r.PathValue("name"), number, in.State)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "issue not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, issue)
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
		writeErr(w, http.StatusBadRequest, "name and public_key are required")
		return
	}
	key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(pub))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid public key: "+err.Error())
		return
	}
	pub = strings.Join(strings.Fields(string(bytes.TrimSpace(ssh.MarshalAuthorizedKey(key)))), " ")
	k, err := a.store.CreateKey(userFrom(r), in.Name, pub, ssh.FingerprintSHA256(key))
	if errors.Is(err, store.ErrExists) {
		writeErr(w, http.StatusConflict, "this key is already registered")
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
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	if errors.Is(a.store.DeleteKey(userFrom(r), id), store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "key not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
