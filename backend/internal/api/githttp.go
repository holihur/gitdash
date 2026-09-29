package api

import (
	"bufio"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"gitdash/backend/internal/gitsvc"
	"gitdash/backend/internal/logx"
)

// Git Smart HTTP：让 `git clone/push https://host/owner/repo.git` 像 GitHub /
// GitLab 一样可用。认证走 HTTP Basic（密码为 PAT 或会话 token，用户名可为任意
// 值）或已有的 Bearer/cookie；鉴权复用 store.CanRead / CanWrite。
//
// 实现上不自己拼 pkt-line，而是把请求转交给 `git http-backend`（CGI），
// 由 git 处理 v0/v1/v2 协议、info/refs、upload-pack、receive-pack 与 push
// 的 hooks（post-receive 照常写 spool → 触发 webhook/CI/语言分析）。
//
// 路由不走 ServeMux 模式：`/{owner}/{repo}/...` 会与 Docker 注册表的 `/v2/`
// 前缀通配冲突，故在 mux 之前按路径形状分发（见 gitHTTPDispatch）。

type gitService string

const (
	gitUploadPack  gitService = "git-upload-pack"
	gitReceivePack gitService = "git-receive-pack"
)

// gitHTTPDispatch 在进入 mux 之前拦截 Git Smart HTTP 请求；非 git 请求原样透传。
func (a *API) gitHTTPDispatch(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if owner, name, svc, ok := matchGitHTTP(r); ok {
			a.serveGitHTTP(w, r, owner, name, svc)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// matchGitHTTP 识别 GitHub/GitLab 风格的 git 路径：
//
//	GET  /{owner}/{repo}.git/info/refs?service=git-upload-pack|git-receive-pack
//	POST /{owner}/{repo}.git/git-upload-pack
//	POST /{owner}/{repo}.git/git-receive-pack
//
// 排除 /api /pages /v2 /login /metrics 等保留前缀，避免误吞其它路由。
func matchGitHTTP(r *http.Request) (owner, name string, svc gitService, ok bool) {
	segs := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(segs) < 3 {
		return "", "", "", false
	}
	switch segs[0] {
	case "api", "pages", "v2", "login", "metrics":
		return "", "", "", false
	}
	switch {
	case r.Method == http.MethodGet && len(segs) == 4 && segs[2] == "info" && segs[3] == "refs":
		svc = gitService(strings.TrimSpace(r.URL.Query().Get("service")))
		if svc != gitUploadPack && svc != gitReceivePack {
			return "", "", "", false
		}
	case r.Method == http.MethodPost && len(segs) == 3 && segs[2] == "git-upload-pack":
		svc = gitUploadPack
	case r.Method == http.MethodPost && len(segs) == 3 && segs[2] == "git-receive-pack":
		svc = gitReceivePack
	default:
		return "", "", "", false
	}
	owner = segs[0]
	name = strings.TrimSuffix(segs[1], ".git")
	if !gitsvc.ValidName(owner) || !gitsvc.ValidName(name) {
		return "", "", "", false
	}
	return owner, name, svc, true
}

// resolveGitUser 解析 HTTPS git 客户端身份。与 resolveUser 的差别：HTTP Basic
// 里用户名可任意（GitHub/GitLab 风格，token 才是身份），只要密码是有效 PAT 即可。
func (a *API) resolveGitUser(r *http.Request) (username string, scopes []string, isPAT bool) {
	if u, s, p := a.resolveUser(r); u != "" {
		return u, s, p
	}
	_, pass, ok := r.BasicAuth()
	if !ok || pass == "" {
		return "", nil, false
	}
	if name, scopes, err := a.store.ValidatePAT(pass, clientIP(r)); err == nil {
		if a.store.IsUserBanned(name) {
			return "", nil, false
		}
		return name, scopes, true
	}
	return "", nil, false
}

func hasScope(scopes []string, want string) bool {
	for _, s := range scopes {
		if s == want {
			return true
		}
	}
	return false
}

func (a *API) serveGitHTTP(w http.ResponseWriter, r *http.Request, owner, name string, svc gitService) {
	username, scopes, isPAT := a.resolveGitUser(r)
	// PAT 需 repo scope；git 全是仓库读写。
	if isPAT && !hasScope(scopes, "repo") {
		writeCode(w, http.StatusForbidden, "insufficient_scope", "token does not have the repo scope")
		return
	}
	if !a.gitAccessAllowed(owner, name, username, svc == gitReceivePack) {
		if username == "" {
			w.Header().Set("WWW-Authenticate", `Basic realm="gitdash"`)
			writeCode(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		} else {
			// 已认证但无权限：404，避免枚举私有仓库。
			writeCode(w, http.StatusNotFound, "not_found", "repository not found")
		}
		return
	}

	// 交给 git http-backend（CGI）。它按 GIT_PROJECT_ROOT + PATH_INFO 定位仓库，
	// 自行处理 v0/v1/v2 协商与 receive-pack 的 hooks。
	env := []string{
		"GIT_PROJECT_ROOT=" + gitsvc.ReposDir(),
		"GIT_HTTP_EXPORT_ALL=1", // 鉴权已在上面完成，这里放开导出检查
		"PATH_INFO=" + r.URL.Path,
		"QUERY_STRING=" + r.URL.RawQuery,
		"REQUEST_METHOD=" + r.Method,
		"SERVER_PROTOCOL=HTTP/1.1",
		"REMOTE_USER=" + username,
		"GITDASH_USER=" + username, // post-receive hook 记录 pusher
	}
	if ct := r.Header.Get("Content-Type"); ct != "" {
		env = append(env, "CONTENT_TYPE="+ct)
	}
	if r.ContentLength >= 0 {
		env = append(env, "CONTENT_LENGTH="+strconv.FormatInt(r.ContentLength, 10))
	}
	// 协议 v2：把客户端的 Git-Protocol 头透传给 backend。
	if gp := r.Header.Get("Git-Protocol"); gp != "" {
		env = append(env, "GIT_PROTOCOL="+gp)
	}

	cmd := exec.CommandContext(r.Context(), "git", "http-backend")
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdin = r.Body
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		internalError(w, err)
		return
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		internalError(w, err)
		return
	}
	streamErr := streamCGI(w, stdout)
	waitErr := cmd.Wait()
	if streamErr != nil {
		logx.Infof("git http-backend %s %s/%s: stream: %v", svc, owner, name, streamErr)
	}
	if waitErr != nil {
		logx.Infof("git http-backend %s %s/%s: %v: %s", svc, owner, name, waitErr, strings.TrimSpace(stderr.String()))
	}
	if svc == gitReceivePack {
		// push 改变了分支/标签集合，失效 15s TTL 缓存（与 SSH 路径一致）。
		gitsvc.InvalidateRefs(owner, name)
	}
}

// gitAccessAllowed 按仓库可见性判定 git 访问：
//
//	write:      需登录且有写权限；
//	read:       anonymous 匿名可读；public 需登录；private 需 CanRead。
//
// 被封禁的仓库/组织一律拒绝。
func (a *API) gitAccessAllowed(owner, name, username string, write bool) bool {
	repo, err := a.store.GetRepo(owner, name)
	if err != nil || repo.Banned || a.store.IsOrgBanned(owner) {
		return false
	}
	if write {
		return username != "" && a.store.CanWrite(owner, name, username)
	}
	vis := repo.Visibility
	if vis == "" {
		if repo.Private {
			vis = "private"
		} else {
			vis = "public"
		}
	}
	switch vis {
	case "anonymous":
		return true
	case "public":
		return username != ""
	default:
		return username != "" && a.store.CanRead(owner, name, username)
	}
}

// streamCGI 解析 git http-backend 的 CGI 响应（Status/头部 + 空行 + body），
// 转发到 ResponseWriter，并在传输过程中 flush（git 进度/大包）。
func streamCGI(w http.ResponseWriter, r io.Reader) error {
	br := bufio.NewReader(r)
	hdr := w.Header()
	status := http.StatusOK
	for {
		line, err := br.ReadString('\n')
		if err != nil && line == "" {
			if err == io.EOF {
				return nil // 空响应
			}
			return err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			break // 头部结束
		}
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if strings.EqualFold(k, "Status") {
			if code, cerr := strconv.Atoi(strings.Fields(v)[0]); cerr == nil {
				status = code
			}
			continue
		}
		hdr.Add(k, v)
	}
	w.WriteHeader(status)
	fw := &flushWriter{w: w}
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
		fw.f = f
	}
	_, err := io.Copy(fw, br)
	return err
}

type flushWriter struct {
	w io.Writer
	f http.Flusher
}

func (fw *flushWriter) Write(p []byte) (int, error) {
	n, err := fw.w.Write(p)
	if fw.f != nil {
		fw.f.Flush()
	}
	return n, err
}
