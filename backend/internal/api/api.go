package api

import (
	"context"
	"encoding/json"
	"errors"
	"gitdash/backend/internal/api/docs"
	"gitdash/backend/internal/copilot"
	"gitdash/backend/internal/envx"
	"gitdash/backend/internal/gpgsig"
	"gitdash/backend/internal/jobs"
	"gitdash/backend/internal/logx"
	"gitdash/backend/internal/metrics"
	"gitdash/backend/internal/notify"
	"gitdash/backend/internal/runner"
	"gitdash/backend/internal/store"
	"gitdash/backend/internal/telemetry"
	"gitdash/backend/internal/webhooks"
	"gitdash/backend/internal/webui"
	"io/fs"
	"net"
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

	httpSwagger "github.com/swaggo/http-swagger"
)

var shaRe = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)

// slowAPIThreshold 默认 1000ms，可用 GITDASH_SLOW_API_MS 覆盖（非法值回退默认）。
var slowAPIThreshold = envx.Millis("GITDASH_SLOW_API_MS", 1000)

// runnerNameRe runner 名（可读字符，2-64 位由 handler 校验长度）
var runnerNameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]+$`)

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

// setTotal 写入 X-Total-Count 响应头（列表分页的总数）。
func setTotal(w http.ResponseWriter, n int) {
	w.Header().Set("X-Total-Count", strconv.Itoa(n))
}

var usernameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{3,31}$`) // 用户名 4-32 位

// orgNameRe 组织名规则（2-32 位，与用户名共用命名空间但长度下限不同）。
var orgNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{1,31}$`)

type API struct {
	store   *store.Store
	version string

	// runnerHub runner WS Hub（main 注入；nil = runner 功能未启用）
	runnerHub *runner.Hub

	// copilotMgr BYOK copilot 会话的 Docker 编排器（main 注入）
	copilotMgr *copilot.Manager

	// jobsMgr 异步任务管理器（导入 / 镜像 / webhook 投递；main 注入）
	jobsMgr *jobs.Manager

	// sshPort SSH 服务端口（clone 地址展示用；main 启动时注入，默认 2222）
	sshPort string

	// Publish 由 main 注入：把 issue/pull/评论事件写入 API 侧 webhook spool
	Publish func(webhooks.Event)

	// EmailSender 由 main 注入（nil = SMTP 未配置，邮箱验证降级为直接视为已验证）
	emailSender *notify.Sender

	// MFA challenge 与 OAuth/OIDC state 均存 settings 表
	// （PutMFAChallenge/PutOAuthState），重启与多实例下均有效

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

// saveOAuthState 写入新的 state（10 分钟有效）。
func (a *API) saveOAuthState(state string) error {
	return a.store.PutOAuthState(state, time.Now().Add(10*time.Minute).UTC().Format(time.RFC3339))
}

// checkOAuthState 一次性校验并消费 state；不存在或已过期返回 false。
func (a *API) checkOAuthState(state string) bool {
	ok, err := a.store.TakeOAuthState(state, time.Now().UTC().Format(time.RFC3339))
	return err == nil && ok
}

// 登录限速：15 分钟窗口内最多 5 次失败

const (
	loginMaxFails = 5
	loginWindow   = 15 * time.Minute
)

func New(s *store.Store, version string) *API {
	return &API{
		store:   s,
		version: version,
		sshPort: "2222",
	}
}

// SetEmailSender 注入 SMTP 发送器（nil = 未配置）。
func (a *API) SetEmailSender(s *notify.Sender) { a.emailSender = s }

// SetJobsManager 注入异步任务管理器（导入 / 镜像 / webhook 投递）。
func (a *API) SetJobsManager(m *jobs.Manager) { a.jobsMgr = m }

// enqueueImport 通过注入的任务队列排队导入；队列未启用时返回错误。
func (a *API) enqueueImport(owner, repo, url, privateKey string) error {
	if a.jobsMgr == nil {
		return errors.New("task queue not configured")
	}
	return a.jobsMgr.EnqueueImport(owner, repo, url, privateKey)
}

// enqueueMirror 通过注入的任务队列排队镜像推送；队列未启用时返回错误。
func (a *API) enqueueMirror(owner, repo, url, privateKey string) error {
	if a.jobsMgr == nil {
		return errors.New("task queue not configured")
	}
	return a.jobsMgr.EnqueueMirror(owner, repo, url, privateKey)
}

// SetSSHPort 注入 SSH 监听端口（clone 地址展示用）。
func (a *API) SetSSHPort(addr string) {
	if _, port, err := net.SplitHostPort(addr); err == nil && port != "" {
		a.sshPort = port
	}
}

// instance 公开实例信息（clone 地址需要真实 SSH 端口）。
//
//	@Summary     实例信息
//	@Description 返回版本与 SSH 端口（前端 clone 地址展示用）。
//	@Tags        misc
//	@Produce     json
//	@Success     200 {object} object
//	@Router      /instance [get]
func (a *API) instance(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"version": a.version, "ssh_port": a.sshPort})
}

type ctxUser struct{}
type ctxPatScopes struct{}

func userFrom(r *http.Request) string {
	v, _ := r.Context().Value(ctxUser{}).(string)
	return v
}

func (a *API) Handler(staticDir string) http.Handler {
	mux := http.NewServeMux()

	// auth providers (public) & github oauth
	mux.HandleFunc("GET /api/auth/providers", a.providers)
	mux.HandleFunc("GET /api/instance", a.instance)
	mux.HandleFunc("POST /api/me/email/verify", a.auth(a.verifyEmail))
	mux.HandleFunc("POST /api/me/email/resend", a.auth(a.resendEmailVerification))
	mux.HandleFunc("GET /api/auth/github", a.githubStart)
	mux.HandleFunc("GET /api/auth/github/callback", a.githubCallback)
	mux.HandleFunc("GET /api/auth/google", a.googleStart)
	mux.HandleFunc("GET /api/auth/google/callback", a.googleCallback)
	mux.HandleFunc("GET /api/auth/oidc/start", a.oidcStart)
	mux.HandleFunc("GET /api/auth/oidc/callback", a.oidcCallback)

	// OAuth 2.0 provider（gitdash 作为授权服务器，供第三方应用接入）
	mux.HandleFunc("GET /api/applications", a.auth(a.listOAuthApps))
	mux.HandleFunc("POST /api/applications", a.auth(a.createOAuthApp))
	mux.HandleFunc("DELETE /api/applications/{id}", a.auth(a.deleteOAuthApp))
	mux.HandleFunc("POST /api/applications/{id}/reset_secret", a.auth(a.resetOAuthAppSecret))
	mux.HandleFunc("GET /api/applications/authorizations", a.auth(a.listOAuthAuthorizations))
	mux.HandleFunc("DELETE /api/applications/authorizations/{id}", a.auth(a.revokeOAuthAuthorization))
	mux.HandleFunc("GET /login/oauth/authorize", a.oauthAuthorize)
	mux.HandleFunc("POST /login/oauth/authorize", a.oauthAuthorize)
	mux.HandleFunc("POST /login/oauth/access_token", a.oauthAccessToken)
	// 设备流（RFC 8628）：CLI 登录用
	mux.HandleFunc("POST /login/oauth/device/code", a.oauthDeviceCode)
	mux.HandleFunc("GET /login/oauth/device", a.oauthDeviceVerify)
	mux.HandleFunc("POST /login/oauth/device", a.oauthDeviceVerify)

	// admin（默认未启用：未引导时一律 404）
	mux.HandleFunc("POST /api/admin/login", a.adminLogin)
	mux.HandleFunc("POST /api/admin/logout", a.adminAuth(a.adminLogout))
	mux.HandleFunc("GET /api/admin/me", a.adminAuth(a.adminMe))
	mux.HandleFunc("GET /api/admin/settings", a.adminAuth(a.adminSettings))
	mux.HandleFunc("POST /api/admin/settings", a.adminAuth(a.adminSaveSettings))
	mux.HandleFunc("POST /api/admin/password", a.adminAuth(a.adminChangePassword))
	mux.HandleFunc("GET /api/admin/quota", a.adminAuth(a.adminGetQuota))
	mux.HandleFunc("POST /api/admin/quota", a.adminAuth(a.adminSaveQuotaDefault))
	mux.HandleFunc("PUT /api/admin/quota/{scope}/{name}", a.adminAuth(a.adminSaveQuotaOverride))
	mux.HandleFunc("DELETE /api/admin/quota/{scope}/{name}", a.adminAuth(a.adminDeleteQuotaOverride))
	mux.HandleFunc("GET /api/admin/runners", a.adminAuth(a.adminListRunners))
	mux.HandleFunc("POST /api/admin/runners/registration-token", a.adminAuth(a.createGlobalRunnerToken))
	mux.HandleFunc("DELETE /api/admin/runners/{name}", a.adminAuth(a.adminDeleteRunner))
	// 用户管理
	mux.HandleFunc("GET /api/admin/users", a.adminAuth(a.adminListUsers))
	mux.HandleFunc("POST /api/admin/users", a.adminAuth(a.adminCreateUser))
	mux.HandleFunc("POST /api/admin/users/{username}/reset_password", a.adminAuth(a.adminResetPassword))
	mux.HandleFunc("POST /api/admin/users/{username}/ban", a.adminAuth(a.adminBanUser))
	mux.HandleFunc("DELETE /api/admin/users/{username}", a.adminAuth(a.adminDeleteUser))
	// 仓库 / 组织管理（含封禁）
	mux.HandleFunc("GET /api/admin/repos", a.adminAuth(a.adminListRepos))
	mux.HandleFunc("POST /api/admin/repos/{owner}/{name}/ban", a.adminAuth(a.adminBanRepo))
	mux.HandleFunc("GET /api/admin/orgs", a.adminAuth(a.adminListOrgs))
	mux.HandleFunc("POST /api/admin/orgs/{name}/ban", a.adminAuth(a.adminBanOrg))
	// auth
	mux.HandleFunc("POST /api/auth/register", a.register)
	mux.HandleFunc("POST /api/auth/login", a.login)
	mux.HandleFunc("POST /api/auth/mfa-verify", a.mfaVerify)
	mux.HandleFunc("POST /api/auth/mfa-email/resend", a.mfaEmailResend)
	mux.HandleFunc("POST /api/auth/logout", a.auth(a.logout))
	mux.HandleFunc("GET /api/me", a.auth(a.me))
	mux.HandleFunc("GET /api/me/export", a.auth(a.exportMe))
	mux.HandleFunc("DELETE /api/me", a.auth(a.deleteMe))

	// user profile & mfa
	mux.HandleFunc("POST /api/me/password", a.auth(a.changePassword))
	mux.HandleFunc("POST /api/me/profile", a.auth(a.updateProfile))
	mux.HandleFunc("GET /api/me/mfa", a.auth(a.mfaStatus))
	mux.HandleFunc("POST /api/me/mfa/enroll", a.auth(a.mfaEnroll))
	mux.HandleFunc("POST /api/me/mfa/activate", a.auth(a.mfaActivate))
	mux.HandleFunc("POST /api/me/mfa/disable", a.auth(a.mfaDisable))
	// email MFA（向已验证邮箱发送验证码的第二因素方式）
	mux.HandleFunc("POST /api/me/mfa/email/enroll", a.auth(a.mfaEmailEnroll))
	mux.HandleFunc("POST /api/me/mfa/email/activate", a.auth(a.mfaEmailActivate))
	mux.HandleFunc("POST /api/me/mfa/email/send", a.auth(a.mfaEmailSend))

	// byok（bring your own key：用户自带 LLM 密钥）
	mux.HandleFunc("GET /api/me/byok", a.auth(a.listByok))
	mux.HandleFunc("POST /api/me/byok", a.auth(a.createByok))
	mux.HandleFunc("POST /api/me/byok/test", a.auth(a.testByok))
	mux.HandleFunc("PUT /api/me/byok/{id}", a.auth(a.updateByok))
	mux.HandleFunc("DELETE /api/me/byok/{id}", a.auth(a.deleteByok))

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
	mux.HandleFunc("GET /api/repos/{name}/search", a.auth(a.search))
	mux.HandleFunc("POST /api/repos/{name}/gc", a.auth(a.gcRepo))
	// repos（owner 限定版：供协作者 / 跨用户访问，owner 显式声明）
	mux.HandleFunc("GET /api/users/{owner}/repos/{name}", a.auth(a.getRepo))
	mux.HandleFunc("DELETE /api/users/{owner}/repos/{name}", a.auth(a.deleteRepo))
	mux.HandleFunc("GET /api/users/{owner}/repos/{name}/branches", a.auth(a.branches))
	mux.HandleFunc("GET /api/users/{owner}/repos/{name}/tree", a.auth(a.tree))
	mux.HandleFunc("GET /api/users/{owner}/repos/{name}/blob", a.auth(a.blob))
	mux.HandleFunc("GET /api/users/{owner}/repos/{name}/blame", a.auth(a.blame))
	mux.HandleFunc("GET /api/users/{owner}/repos/{name}/commits", a.auth(a.commits))
	mux.HandleFunc("GET /api/users/{owner}/repos/{name}/search", a.auth(a.search))
	mux.HandleFunc("POST /api/users/{owner}/repos/{name}/gc", a.auth(a.gcRepo))

	// user page & follow
	mux.HandleFunc("GET /api/users/{username}", a.auth(a.getUserProfile))
	mux.HandleFunc("GET /api/users/{username}/avatar", a.auth(a.getAvatar))
	mux.HandleFunc("POST /api/me/avatar", a.auth(a.uploadAvatar))
	mux.HandleFunc("DELETE /api/me/avatar", a.auth(a.deleteAvatar))
	mux.HandleFunc("POST /api/users/{username}/follow", a.auth(a.followUser))
	mux.HandleFunc("DELETE /api/users/{username}/follow", a.auth(a.unfollowUser))
	mux.HandleFunc("GET /api/users/{username}/followers", a.auth(a.listFollowers))
	mux.HandleFunc("GET /api/users/{username}/following", a.auth(a.listFollowing))

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
	mux.HandleFunc("PATCH /api/repos/{name}/issues/{number}", a.auth(a.updateIssue))
	mux.HandleFunc("DELETE /api/repos/{name}/issues/{number}", a.auth(a.deleteIssue))
	mux.HandleFunc("GET /api/users/{owner}/repos/{name}/issues", a.auth(a.listIssues))
	mux.HandleFunc("POST /api/users/{owner}/repos/{name}/issues", a.auth(a.createIssue))
	mux.HandleFunc("PATCH /api/users/{owner}/repos/{name}/issues/{number}", a.auth(a.updateIssue))
	mux.HandleFunc("DELETE /api/users/{owner}/repos/{name}/issues/{number}", a.auth(a.deleteIssue))
	mux.HandleFunc("POST /api/users/{owner}/repos/{name}/issues/{number}/labels", a.auth(a.setIssueLabels))
	mux.HandleFunc("POST /api/users/{owner}/repos/{name}/issues/{number}/milestone", a.auth(a.setIssueMilestone))

	// comments（issue 与 PR 共用，kind 由路由闭包区分）
	issueList, issueAdd := a.issueOrPullComments("issue")
	pullList, pullAdd := a.issueOrPullComments("pull")
	mux.HandleFunc("GET /api/users/{owner}/repos/{name}/issues/{number}/comments", a.auth(issueList))
	mux.HandleFunc("POST /api/users/{owner}/repos/{name}/issues/{number}/comments", a.auth(issueAdd))
	mux.HandleFunc("GET /api/repos/{name}/issues/{number}/comments", a.auth(issueList))
	mux.HandleFunc("POST /api/repos/{name}/issues/{number}/comments", a.auth(issueAdd))
	mux.HandleFunc("GET /api/users/{owner}/repos/{name}/pulls/{number}/comments", a.auth(pullList))
	mux.HandleFunc("POST /api/users/{owner}/repos/{name}/pulls/{number}/comments", a.auth(pullAdd))
	mux.HandleFunc("DELETE /api/users/{owner}/repos/{name}/comments/{id}", a.auth(a.deleteComment))

	// issue labels & milestones
	mux.HandleFunc("GET /api/users/{owner}/repos/{name}/labels", a.auth(a.listLabels))
	mux.HandleFunc("POST /api/users/{owner}/repos/{name}/labels", a.auth(a.createLabel))
	mux.HandleFunc("PATCH /api/users/{owner}/repos/{name}/labels/{id}", a.auth(a.updateLabel))
	mux.HandleFunc("DELETE /api/users/{owner}/repos/{name}/labels/{id}", a.auth(a.deleteLabel))
	mux.HandleFunc("GET /api/users/{owner}/repos/{name}/milestones", a.auth(a.listMilestones))
	mux.HandleFunc("POST /api/users/{owner}/repos/{name}/milestones", a.auth(a.createMilestone))
	mux.HandleFunc("PATCH /api/users/{owner}/repos/{name}/milestones/{id}", a.auth(a.updateMilestone))
	mux.HandleFunc("DELETE /api/users/{owner}/repos/{name}/milestones/{id}", a.auth(a.deleteMilestone))

	// projects（看板 + 泳道）
	mux.HandleFunc("GET /api/users/{owner}/repos/{name}/projects", a.auth(a.listProjects))
	mux.HandleFunc("POST /api/users/{owner}/repos/{name}/projects", a.auth(a.createProject))
	mux.HandleFunc("PATCH /api/users/{owner}/repos/{name}/projects/{id}", a.auth(a.updateProject))
	mux.HandleFunc("DELETE /api/users/{owner}/repos/{name}/projects/{id}", a.auth(a.deleteProject))
	mux.HandleFunc("GET /api/users/{owner}/repos/{name}/projects/{id}/board", a.auth(a.getProjectBoard))
	mux.HandleFunc("GET /api/users/{owner}/repos/{name}/projects/{id}/columns", a.auth(a.listProjectColumns))
	mux.HandleFunc("POST /api/users/{owner}/repos/{name}/projects/{id}/columns", a.auth(a.createProjectColumn))
	mux.HandleFunc("PATCH /api/users/{owner}/repos/{name}/projects/{id}/columns/{cid}", a.auth(a.updateProjectColumn))
	mux.HandleFunc("DELETE /api/users/{owner}/repos/{name}/projects/{id}/columns/{cid}", a.auth(a.deleteProjectColumn))
	mux.HandleFunc("GET /api/users/{owner}/repos/{name}/projects/{id}/swimlanes", a.auth(a.listProjectSwimlanes))
	mux.HandleFunc("POST /api/users/{owner}/repos/{name}/projects/{id}/swimlanes", a.auth(a.createProjectSwimlane))
	mux.HandleFunc("PATCH /api/users/{owner}/repos/{name}/projects/{id}/swimlanes/{lid}", a.auth(a.updateProjectSwimlane))
	mux.HandleFunc("DELETE /api/users/{owner}/repos/{name}/projects/{id}/swimlanes/{lid}", a.auth(a.deleteProjectSwimlane))
	mux.HandleFunc("GET /api/users/{owner}/repos/{name}/projects/{id}/cards", a.auth(a.listProjectCards))
	mux.HandleFunc("POST /api/users/{owner}/repos/{name}/projects/{id}/cards", a.auth(a.createProjectCard))
	mux.HandleFunc("PATCH /api/users/{owner}/repos/{name}/projects/{id}/cards/{card}", a.auth(a.updateProjectCard))
	mux.HandleFunc("DELETE /api/users/{owner}/repos/{name}/projects/{id}/cards/{card}", a.auth(a.deleteProjectCard))

	mux.HandleFunc("GET /api/users/{owner}/repos/{name}/commits/{sha}/diff", a.auth(a.commitDiff))
	mux.HandleFunc("GET /api/users/{owner}/repos/{name}/compare", a.auth(a.compareRefs))
	mux.HandleFunc("POST /api/users/{owner}/repos/{name}/commits/{sha}/revert", a.auth(a.revertCommit))
	mux.HandleFunc("POST /api/users/{owner}/repos/{name}/commits", a.auth(a.writeCommit))
	mux.HandleFunc("GET /api/users/{owner}/repos/{name}/tags", a.auth(a.listTags))
	mux.HandleFunc("POST /api/users/{owner}/repos/{name}/refs", a.auth(a.createRef))
	mux.HandleFunc("DELETE /api/users/{owner}/repos/{name}/refs/{kind}/{refname}", a.auth(a.deleteRef))

	// releases & assets
	mux.HandleFunc("GET /api/repos/{name}/releases", a.auth(a.listReleases))
	mux.HandleFunc("POST /api/repos/{name}/releases", a.auth(a.createRelease))
	mux.HandleFunc("GET /api/repos/{name}/releases/{tag}", a.auth(a.getRelease))
	mux.HandleFunc("DELETE /api/repos/{name}/releases/{tag}", a.auth(a.deleteRelease))
	mux.HandleFunc("GET /api/users/{owner}/repos/{name}/releases", a.auth(a.listReleases))
	mux.HandleFunc("POST /api/users/{owner}/repos/{name}/releases", a.auth(a.createRelease))
	mux.HandleFunc("GET /api/users/{owner}/repos/{name}/releases/{tag}", a.auth(a.getRelease))
	mux.HandleFunc("DELETE /api/users/{owner}/repos/{name}/releases/{tag}", a.auth(a.deleteRelease))
	mux.HandleFunc("POST /api/users/{owner}/repos/{name}/releases/{tag}/assets", a.auth(a.uploadAsset))
	mux.HandleFunc("GET /api/users/{owner}/repos/{name}/releases/{tag}/assets", a.auth(a.listAssets))
	mux.HandleFunc("GET /api/users/{owner}/repos/{name}/releases/{tag}/assets/{filename}", a.auth(a.downloadAsset))
	mux.HandleFunc("DELETE /api/users/{owner}/repos/{name}/releases/{tag}/assets/{filename}", a.auth(a.deleteAsset))

	// pull requests
	mux.HandleFunc("GET /api/users/{owner}/repos/{name}/pulls", a.auth(a.listPulls))
	mux.HandleFunc("POST /api/users/{owner}/repos/{name}/pulls", a.auth(a.createPull))
	mux.HandleFunc("GET /api/users/{owner}/repos/{name}/pulls/{number}", a.auth(a.getPull))
	mux.HandleFunc("GET /api/users/{owner}/repos/{name}/pulls/{number}/diff", a.auth(a.pullDiff))
	mux.HandleFunc("POST /api/users/{owner}/repos/{name}/pulls/{number}/merge", a.auth(a.mergePull))
	mux.HandleFunc("POST /api/users/{owner}/repos/{name}/pulls/{number}/state", a.auth(a.setPullState))

	// branch protection
	mux.HandleFunc("GET /api/users/{owner}/repos/{name}/branch-protections", a.auth(a.listBranchProtections))
	mux.HandleFunc("PUT /api/users/{owner}/repos/{name}/branch-protections/{branch}", a.auth(a.setBranchProtection))
	mux.HandleFunc("DELETE /api/users/{owner}/repos/{name}/branch-protections/{branch}", a.auth(a.deleteBranchProtection))
	mux.HandleFunc("GET /api/users/{owner}/repos/{name}/pulls/{number}/reviews", a.auth(a.listReviews))
	mux.HandleFunc("POST /api/users/{owner}/repos/{name}/pulls/{number}/reviews", a.auth(a.createReview))

	// collaborators
	mux.HandleFunc("POST /api/users/{owner}/repos/{name}/visibility", a.auth(a.setRepoVisibility))
	mux.HandleFunc("POST /api/users/{owner}/repos/{name}/template", a.auth(a.setRepoTemplate))
	mux.HandleFunc("POST /api/users/{owner}/repos/{name}/default-branch", a.auth(a.setRepoDefaultBranch))
	mux.HandleFunc("POST /api/users/{owner}/repos/{name}/issues-enabled", a.auth(a.setRepoIssues))
	mux.HandleFunc("POST /api/users/{owner}/repos/{name}/description", a.auth(a.setRepoDescription))
	mux.HandleFunc("PUT /api/users/{owner}/repos/{name}/topics", a.auth(a.setRepoTops))
	mux.HandleFunc("GET /api/topics", a.auth(a.listTopics))
	mux.HandleFunc("GET /api/templates", a.auth(a.listTemplateRepos))
	mux.HandleFunc("GET /api/explore/repos", a.auth(a.exploreRepos))
	mux.HandleFunc("GET /api/search", a.auth(a.globalSearch))

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
	mux.HandleFunc("GET /api/users/{owner}/repos/{name}/webhooks/{id}/deliveries", a.auth(a.listWebhookDeliveries))
	mux.HandleFunc("GET /api/webhook-events", a.auth(a.listWebhookEvents))

	// incoming webhook（入站：外部系统凭 token 创建 issue；不经过登录鉴权）
	mux.HandleFunc("GET /api/users/{owner}/repos/{name}/incoming-webhook", a.auth(a.getIncomingWebhook))
	mux.HandleFunc("POST /api/users/{owner}/repos/{name}/incoming-webhook", a.auth(a.setIncomingWebhook))
	mux.HandleFunc("DELETE /api/users/{owner}/repos/{name}/incoming-webhook", a.auth(a.deleteIncomingWebhook))
	mux.HandleFunc("POST /api/hooks/incoming/{owner}/{repo}", a.createIssueFromIncomingWebhook)

	// pipeline（CI）
	mux.HandleFunc("GET /api/users/{owner}/repos/{name}/pipeline", a.auth(a.getPipeline))
	mux.HandleFunc("GET /api/users/{owner}/repos/{name}/pipeline/graph", a.auth(a.getPipelineGraph))
	mux.HandleFunc("PUT /api/users/{owner}/repos/{name}/pipeline", a.auth(a.setPipeline))
	mux.HandleFunc("GET /api/users/{owner}/repos/{name}/pipeline/runs", a.auth(a.listPipelineRuns))
	mux.HandleFunc("POST /api/users/{owner}/repos/{name}/pipeline/runs", a.auth(a.createPipelineRun))
	mux.HandleFunc("POST /api/users/{owner}/repos/{name}/pipeline/runs/{id}/rerun", a.auth(a.rerunPipelineRun))
	mux.HandleFunc("POST /api/users/{owner}/repos/{name}/pipeline/dispatch", a.auth(a.dispatchPipelineRun))
	mux.HandleFunc("GET /api/users/{owner}/repos/{name}/pipeline/runs/{id}", a.auth(a.getPipelineRun))
	mux.HandleFunc("POST /api/users/{owner}/repos/{name}/pipeline/runs/{id}/cancel", a.auth(a.cancelPipelineRun))
	mux.HandleFunc("GET /api/users/{owner}/repos/{name}/env", a.auth(a.listRepoEnvVars))
	mux.HandleFunc("PUT /api/users/{owner}/repos/{name}/env", a.auth(a.setRepoEnvVar))
	mux.HandleFunc("DELETE /api/users/{owner}/repos/{name}/env/{key}", a.auth(a.deleteRepoEnvVar))

	// runners（自托管 CI agent）
	mux.HandleFunc("POST /api/runners/registration-token", a.auth(a.createRunnerToken))
	mux.HandleFunc("GET /api/runners", a.auth(a.listRunners))
	mux.HandleFunc("DELETE /api/runners/{name}", a.auth(a.deleteRunner))
	mux.HandleFunc("POST /api/runner/register", a.registerRunner)
	mux.HandleFunc("GET /api/runner/ws", a.runnerWS)

	// copilot（BYOK AI copilot 会话，嵌入式 agent + 双向聊天 + 自动提交推送闭环）
	mux.HandleFunc("GET /api/users/{owner}/repos/{name}/copilots", a.auth(a.listCopilots))
	mux.HandleFunc("POST /api/users/{owner}/repos/{name}/copilots", a.auth(a.createCopilot))
	mux.HandleFunc("GET /api/users/{owner}/repos/{name}/copilots/{id}", a.auth(a.getCopilot))
	mux.HandleFunc("GET /api/users/{owner}/repos/{name}/copilots/{id}/messages", a.auth(a.copilotMessages))
	mux.HandleFunc("GET /api/users/{owner}/repos/{name}/copilots/{id}/chat", a.auth(a.copilotChat))
	mux.HandleFunc("POST /api/users/{owner}/repos/{name}/copilots/{id}/stop", a.auth(a.stopCopilot))
	mux.HandleFunc("DELETE /api/users/{owner}/repos/{name}/copilots/{id}", a.auth(a.deleteCopilot))

	// ssh keys
	mux.HandleFunc("GET /api/keys", a.auth(a.listKeys))
	mux.HandleFunc("POST /api/keys", a.auth(a.createKey))
	mux.HandleFunc("DELETE /api/keys/{id}", a.auth(a.deleteKey))

	// gpg keys
	mux.HandleFunc("GET /api/gpg", a.auth(a.listGPGKeys))
	mux.HandleFunc("POST /api/gpg", a.auth(a.addGPGKey))
	mux.HandleFunc("DELETE /api/gpg/{id}", a.auth(a.deleteGPGKey))

	// personal access tokens
	mux.HandleFunc("GET /api/tokens", a.auth(a.listTokens))
	mux.HandleFunc("POST /api/tokens", a.auth(a.createTokens))
	mux.HandleFunc("DELETE /api/tokens/{id}", a.auth(a.deleteToken))

	// packages（私有包注册表：npm / composer / pypi / rubygems / go / cargo / maven）
	// 读：任意已认证用户（关联仓库时跟随其可见性）；写：owner 本人或组织 owner 角色成员
	mux.HandleFunc("GET /api/packages/{owner}", a.auth(a.listPackagesUI))
	mux.HandleFunc("GET /api/packages/{owner}/{type}", a.auth(a.listPackagesUI))
	mux.HandleFunc("GET /api/packages/{owner}/audit", a.auth(a.listPackageAudit))
	mux.HandleFunc("DELETE /api/packages/{type}/{owner}/{name...}", a.auth(a.deletePackageUI))

	// npm
	mux.HandleFunc("PUT /api/packages/npm/{owner}/{rest...}", a.auth(a.npmPublish))
	mux.HandleFunc("DELETE /api/packages/npm/{owner}/{name...}", a.auth(a.npmDelete))
	mux.HandleFunc("GET /api/packages/npm/{owner}/{rest...}", a.auth(a.npmGet))

	// pypi（twine）
	mux.HandleFunc("POST /api/packages/pypi/{owner}", a.auth(a.pypiUpload))
	mux.HandleFunc("POST /api/packages/pypi/{owner}/", a.auth(a.pypiUpload))
	mux.HandleFunc("GET /api/packages/pypi/{owner}/pypi/{name}/json", a.auth(a.pypiJSON))
	mux.HandleFunc("GET /api/packages/pypi/{owner}/simple", a.auth(a.pypiSimpleIndex))
	mux.HandleFunc("GET /api/packages/pypi/{owner}/simple/", a.auth(a.pypiSimpleIndex))
	mux.HandleFunc("GET /api/packages/pypi/{owner}/simple/{name}", a.auth(a.pypiSimpleProject))
	mux.HandleFunc("GET /api/packages/pypi/{owner}/simple/{name}/", a.auth(a.pypiSimpleProject))
	mux.HandleFunc("GET /api/packages/pypi/{owner}/download/{name}/{version}/{filename}", a.auth(a.pypiDownload))

	// go (GOPROXY)
	mux.HandleFunc("PUT /api/packages/go/{owner}/{rest...}", a.auth(a.goPut))
	mux.HandleFunc("GET /api/packages/go/{owner}/{rest...}", a.auth(a.goRoute))

	// cargo
	mux.HandleFunc("GET /api/packages/cargo/{owner}/config.json", a.auth(a.cargoConfig))
	mux.HandleFunc("PUT /api/packages/cargo/{owner}/api/v1/crates/new", a.auth(a.cargoPublish))
	mux.HandleFunc("DELETE /api/packages/cargo/{owner}/api/v1/crates/{crate}/{version}/yank", a.auth(a.cargoYank))
	mux.HandleFunc("PUT /api/packages/cargo/{owner}/api/v1/crates/{crate}/{version}/yank", a.auth(a.cargoYank))
	mux.HandleFunc("GET /api/packages/cargo/{owner}/index/{rest...}", a.auth(a.cargoIndex))
	mux.HandleFunc("GET /api/packages/cargo/{owner}/dl/{crate}/{version}/{filename}", a.auth(a.cargoDownload))

	// rubygems
	mux.HandleFunc("POST /api/packages/rubygems/{owner}/api/v1/gems", a.auth(a.gemPush))
	mux.HandleFunc("GET /api/packages/rubygems/{owner}/gems/{filename}", a.auth(a.gemDownload))

	// composer
	mux.HandleFunc("PUT /api/packages/composer/{owner}/{vendor}/{name}", a.auth(a.composerUpload))
	mux.HandleFunc("GET /api/packages/composer/{owner}/packages.json", a.auth(a.composerPackagesJSON))
	mux.HandleFunc("GET /api/packages/composer/{owner}/p2/{vendor}/{name}", a.auth(a.composerP2))
	mux.HandleFunc("GET /api/packages/composer/{owner}/download/{vendor}/{name}/{version}/{filename}", a.auth(a.composerDownload))

	// maven
	mux.HandleFunc("PUT /api/packages/maven/{owner}/{rest...}", a.auth(a.mavenUpload))
	mux.HandleFunc("GET /api/packages/maven/{owner}/{rest...}", a.auth(a.mavenGet))
	mux.HandleFunc("HEAD /api/packages/maven/{owner}/{rest...}", a.auth(a.mavenGet))

	// Docker / OCI 私有注册表（Distribution spec，统一在 /v2/ 下按路径分派）
	mux.HandleFunc("/v2/", a.registryAuth(a.registryHandler))

	// swagger（OpenAPI 文档 + 内置 Swagger UI）
	docs.SwaggerInfo.BasePath = "/api"
	docs.SwaggerInfo.Host = ""
	mux.HandleFunc("GET /api/openapi.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(docs.SwaggerInfo.ReadDoc()))
	})
	mux.Handle("GET /api/swagger", http.RedirectHandler("/api/swagger/", http.StatusMovedPermanently))
	mux.Handle("GET /api/swagger/", httpSwagger.WrapHandler)

	// public
	// prometheus metrics
	mux.Handle("GET /metrics", metrics.Handler())
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		// readiness：确认数据库可达（2s 超时），失败返回 503；liveness 见 /api/health/live。
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := a.store.Ping(ctx); err != nil {
			logx.Warn("health check failed", "err", err)
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	// liveness：进程存活即返回 200，不依赖下游（用于 restart 判定，避免 DB 抖动触发重启）。
	mux.HandleFunc("GET /api/health/live", func(w http.ResponseWriter, r *http.Request) {
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

	return telemetry.Middleware(secureHeaders(logMiddleware(csrfGuard(mux))))
}

// csrfGuard 校验跨站请求：带 Origin 的非安全方法必须与本站同源（cookie 会话的 CSRF 防线）。
// 叠加浏览器强信号：Sec-Fetch-Site 为 cross-site 直接拒绝；无 Origin 时退回校验 Referer。
func csrfGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		if site := r.Header.Get("Sec-Fetch-Site"); site == "cross-site" {
			writeCode(w, http.StatusForbidden, "invalid_origin", "cross-origin request rejected")
			return
		}
		if !sameOriginRequest(r) {
			writeCode(w, http.StatusForbidden, "invalid_origin", "cross-origin request rejected")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// sameOriginRequest 校验 Origin（优先）或 Referer 与请求 Host 同源；
// 两者都缺失时放行（非浏览器客户端如 curl/CI，不构成 CSRF）。
func sameOriginRequest(r *http.Request) bool {
	match := func(raw string) bool {
		if raw == "" || raw == "null" {
			return true
		}
		u, err := url.Parse(raw)
		if err != nil || u.Host == "" {
			return false
		}
		return strings.EqualFold(u.Host, r.Host)
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		return match(origin)
	}
	if referer := r.Header.Get("Referer"); referer != "" {
		return match(referer)
	}
	return true
}

// secureHeaders 基础安全响应头（CSP 允许内联主题脚本，其 sha256 随 index.html 保持同步）。
// swagger UI 页面自带库生成的内联初始化脚本（内容随配置变化，无法预置 hash），
// 对其单独放行 'unsafe-inline'（仅限该路径下的同源文档界面）。

const (
	cspDefault = "default-src 'self'; script-src 'self' 'sha256-3ErQTYhfRUcdQMKUwZWjeUj+0gLQwEdW3gtvOjOALlg=' 'sha256-5N7k7wNTDShVptRxTM9+DDLf2WYyHnUno1d06dT7Cic='; " +
		"style-src 'self' 'unsafe-inline'; img-src 'self' data: blob: https: http:; " +
		"font-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'"
	cspSwagger = "default-src 'self'; script-src 'self' 'unsafe-inline'; " +
		"style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; " +
		"font-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'"
)

func secureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/swagger") {
			w.Header().Set("Content-Security-Policy", cspSwagger)
		} else {
			w.Header().Set("Content-Security-Policy", cspDefault)
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if r.TLS != nil || forceSecureCookies { // HTTPS（含反代）时启用 HSTS
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

// ---- auth ----

func (a *API) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		username, scopes, isPAT := a.resolveUser(r)
		if username == "" {
			writeCode(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
			return
		}
		if isPAT && !patAllowed(r.URL.Path, scopes) {
			writeCode(w, http.StatusForbidden, "insufficient_scope", "token does not have the required scope")
			return
		}
		ctx := context.WithValue(r.Context(), ctxUser{}, username)
		if isPAT {
			ctx = context.WithValue(ctx, ctxPatScopes{}, scopes)
		}
		next(w, r.WithContext(ctx))
	}
}

// patAllowed 判定 PAT 是否覆盖该路径所需 scope：
// /api/admin* 一律拒绝；/api/tokens* 管理自身放行；/api/inbox* 需 inbox；
// /api/keys* 与 /api/gpg* 需 keys；其余需 repo。
func patAllowed(path string, scopes []string) bool {
	if strings.HasPrefix(path, "/api/admin") {
		return false
	}
	if strings.HasPrefix(path, "/api/tokens") {
		return true
	}
	required := "repo"
	switch {
	case strings.HasPrefix(path, "/api/inbox"):
		required = "inbox"
	case strings.HasPrefix(path, "/api/keys"), strings.HasPrefix(path, "/api/gpg"):
		required = "keys"
	}
	for _, s := range scopes {
		if s == required {
			return true
		}
	}
	return false
}

func logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rw := &statusWriter{ResponseWriter: w, status: 200}
		start := time.Now()
		next.ServeHTTP(rw, r)
		dur := time.Since(start)
		// 结构化访问日志（字段可被日志采集器直接索引）
		logx.Info("http request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rw.status,
			"duration_ms", float64(dur.Microseconds())/1000.0,
			"remote", r.RemoteAddr,
		)
		if r.URL.Path != "/metrics" {
			metrics.Observe(r.Method, r.URL.Path, rw.status, dur)
		}
		// 慢接口告警：跳过 /metrics 与 /api/health（避免健康检查自激）。
		if r.URL.Path != "/metrics" && r.URL.Path != "/api/health" && dur > slowAPIThreshold {
			logx.Warn("slow api",
				"method", r.Method,
				"path", r.URL.Path,
				"status", rw.status,
				"duration_ms", float64(dur.Microseconds())/1000.0,
				"threshold_ms", float64(slowAPIThreshold.Microseconds())/1000.0,
			)
			metrics.ObserveSlow(r.Method, r.URL.Path)
		}
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

// Unwrap 支持 http.ResponseController / WebSocket 升级（Hijack/Flush 穿透内层 writer）。
func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

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

// internalError 记录内部错误细节到日志，对外只返回通用文案，避免
// GORM/驱动/文件系统错误（表结构、路径等）泄漏进 HTTP 响应。
// 请求上下文（方法/路径/状态码）由 logMiddleware 统一记录。
func internalError(w http.ResponseWriter, err error) {
	var qe *store.QuotaError
	if errors.As(err, &qe) {
		writeCode(w, http.StatusForbidden, "quota_exceeded", qe.Error())
		return
	}
	logx.Infof("internal error: %v", err)
	writeCode(w, http.StatusInternalServerError, "internal_error", "internal server error")
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
				internalError(w, err)
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
	if repo.Banned || a.store.IsOrgBanned(owner) {
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
	if a.store.IsRepoBanned(owner, name) {
		writeNotFound(w, "repo")
		return "", "", false
	}
	return owner, name, true
}

// ---- orgs (namespace) ----
