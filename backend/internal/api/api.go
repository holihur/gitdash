package api

import (
	"context"
	"encoding/json"
	"errors"
	"gitdash/backend/internal/gpgsig"
	"gitdash/backend/internal/store"
	"gitdash/backend/internal/webui"
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
)

var shaRe = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)

// pageParams 解析 ?limit / ?offset；默认上限 200，最大 500，防止列表端点全量返回。
func pageParams(r *http.Request) (limit, offset int) {
	limit, _ = strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ = strconv.Atoi(r.URL.Query().Get("offset"))
	if limit <= 0 {
		limit = 200
	}
	if limit > 500 {
		limit = 500
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

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

	gpgMu     sync.Mutex
	gpgKeys   []gpgsig.Key
	gpgKeysAt time.Time // GPG 公钥 TTL 缓存，避免 commits 页每请求全量加载
}

const gpgKeysCacheTTL = 30 * time.Second

// gpgVerifyKeys 带 30s TTL 的全量 GPG 公钥缓存；增删公钥时失效。
func (a *API) gpgVerifyKeys() []gpgsig.Key {
	a.gpgMu.Lock()
	defer a.gpgMu.Unlock()
	if a.gpgKeys != nil && time.Since(a.gpgKeysAt) < gpgKeysCacheTTL {
		return a.gpgKeys
	}
	keys := []gpgsig.Key{}
	if gks, err := a.store.AllGPGKeys(); err == nil {
		for _, k := range gks {
			keys = append(keys, gpgsig.Key{Username: k.Username, Fingerprint: k.Fingerprint, Armor: k.Armor})
		}
	}
	a.gpgKeys = keys
	a.gpgKeysAt = time.Now()
	return keys
}

func (a *API) invalidateGPGKeys() {
	a.gpgMu.Lock()
	a.gpgKeys = nil
	a.gpgMu.Unlock()
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
	mux.HandleFunc("POST /api/me/profile", a.auth(a.updateProfile))
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
	mux.HandleFunc("GET /api/repos/{name}/blame", a.auth(a.blame))
	mux.HandleFunc("GET /api/repos/{name}/commits", a.auth(a.commits))
	// repos（owner 限定版：供协作者 / 跨用户访问，owner 显式声明）
	mux.HandleFunc("GET /api/users/{owner}/repos/{name}", a.auth(a.getRepo))
	mux.HandleFunc("DELETE /api/users/{owner}/repos/{name}", a.auth(a.deleteRepo))
	mux.HandleFunc("GET /api/users/{owner}/repos/{name}/branches", a.auth(a.branches))
	mux.HandleFunc("GET /api/users/{owner}/repos/{name}/tree", a.auth(a.tree))
	mux.HandleFunc("GET /api/users/{owner}/repos/{name}/blob", a.auth(a.blob))
	mux.HandleFunc("GET /api/users/{owner}/repos/{name}/blame", a.auth(a.blame))
	mux.HandleFunc("GET /api/users/{owner}/repos/{name}/commits", a.auth(a.commits))

	// star & fork & import
	mux.HandleFunc("GET /api/starred", a.auth(a.listStarred))
	mux.HandleFunc("PUT /api/users/{owner}/repos/{name}/star", a.auth(a.starRepo))
	mux.HandleFunc("DELETE /api/users/{owner}/repos/{name}/star", a.auth(a.unstarRepo))
	mux.HandleFunc("POST /api/users/{owner}/repos/{name}/fork", a.auth(a.forkRepo))
	mux.HandleFunc("POST /api/imports", a.auth(a.importRepo))

	// watch & inbox（关注仓库 + 收件箱通知）
	mux.HandleFunc("GET /api/watched", a.auth(a.listWatched))
	mux.HandleFunc("PUT /api/users/{owner}/repos/{name}/watch", a.auth(a.watchRepo))
	mux.HandleFunc("DELETE /api/users/{owner}/repos/{name}/watch", a.auth(a.unwatchRepo))
	mux.HandleFunc("GET /api/inbox", a.auth(a.listInbox))
	mux.HandleFunc("GET /api/inbox/unread", a.auth(a.inboxUnread))
	mux.HandleFunc("POST /api/inbox/read", a.auth(a.inboxReadAll))
	mux.HandleFunc("POST /api/inbox/read/{id}", a.auth(a.inboxReadOne))
	mux.HandleFunc("DELETE /api/inbox/{id}", a.auth(a.deleteInbox))

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

	// pipeline（CI）
	mux.HandleFunc("GET /api/users/{owner}/repos/{name}/pipeline", a.auth(a.getPipeline))
	mux.HandleFunc("PUT /api/users/{owner}/repos/{name}/pipeline", a.auth(a.setPipeline))
	mux.HandleFunc("GET /api/users/{owner}/repos/{name}/pipeline/runs", a.auth(a.listPipelineRuns))
	mux.HandleFunc("POST /api/users/{owner}/repos/{name}/pipeline/runs", a.auth(a.createPipelineRun))
	mux.HandleFunc("GET /api/users/{owner}/repos/{name}/pipeline/runs/{id}", a.auth(a.getPipelineRun))

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
	if me == owner {
		return owner, name, true
	}
	// 读路径跳过 CanWrite（其结果会被覆盖）；公开性已由 repo.Private 判定，
	// 只对私有仓库走 CanRead（避免 CanWrite/CanRead 的重复 IsOrg/OrgRole 查询）。
	can := !repo.Private || a.store.CanRead(owner, name, me)
	if write {
		can = a.store.CanWrite(owner, name, me)
	}
	if !can {
		writeNotFound(w, "repo")
		return "", "", false
	}
	return owner, name, true
}

// attachStars 批量填充仓库的 star 数与当前用户是否已 star。

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
