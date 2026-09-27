package api

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"gitdash/backend/internal/envx"
	"gitdash/backend/internal/logx"
	"gitdash/backend/internal/store"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"strings"
	"time"
)

// errEmailMFAUnavailable 表示用户使用 email MFA 但 SMTP 未配置，无法下发验证码。
var errEmailMFAUnavailable = errors.New("email mfa requires SMTP to be configured")

func (a *API) rateKey(username, ip string) string { return username + "|" + ip }

// rateLimitDisabled 测试/开发可通过 GITDASH_DISABLE_RATE_LIMIT=1 关闭限流，
// 避免黑盒测试套件（同 IP 大量注册/登录失败）误触发 429。
var rateLimitDisabled = os.Getenv("GITDASH_DISABLE_RATE_LIMIT") == "1"

// registrationDisabled 是否关闭开放注册（GITDASH_DISABLE_REGISTRATION=1/true）。
// 每次读取，便于测试通过 t.Setenv 覆盖，也允许运维运行时切换。
func registrationDisabled() bool {
	return envx.Bool("GITDASH_DISABLE_REGISTRATION", false)
}

// registrationDisabledByAdmin 合并管理端开关与环境变量：任一为真即关闭注册。
// 管理端可在后台关闭自助注册以收敛批量账号滥用（S-03），默认保持开放。
func (a *API) registrationDisabledByAdmin() bool {
	if registrationDisabled() {
		return true
	}
	return a.store.GetSetting("registration_disabled") == "1"
}

// passwordLoginEnabled 账号密码登录是否开放。默认开启；管理端将设置
// password_login_enabled 置为 "0" 后关闭（此时仅剩 OAuth/OIDC 等登录方式）。
// 每次读取，管理端保存后立即生效。
func (a *API) passwordLoginEnabled() bool {
	return a.store.GetSetting("password_login_enabled") != "0"
}

// swaggerEnabled 匿名是否可访问 Swagger UI / OpenAPI 规范。默认开启；管理端将
// 设置 swagger_enabled 置为 "0" 后关闭（Swagger / openapi.json 一律返回 404）。
func (a *API) swaggerEnabled() bool {
	return a.store.GetSetting("swagger_enabled") != "0"
}

// versionVisible 是否在匿名接口（/api/version、/api/instance）暴露版本号。
// 默认开启；管理端将设置 version_visible 置为 "0" 后隐藏（返回空串）。
func (a *API) versionVisible() bool {
	return a.store.GetSetting("version_visible") != "0"
}

// publicVersion 返回对外暴露的版本号；被隐藏时为空串。
func (a *API) publicVersion() string {
	if !a.versionVisible() {
		return ""
	}
	return a.version
}

// 限速记录持久化在 store（login_fails 表）：重启与多实例共享同一窗口，
// 不再有内存 map 的无限增长问题。

func (a *API) rateBlocked(key string) bool {
	if rateLimitDisabled {
		return false
	}
	blocked, err := a.store.RateBlocked(key, loginMaxFails)
	if err != nil {
		logx.Infof("rate check %s: %v", key, err)
		return false
	}
	return blocked
}

func (a *API) rateFail(key string) {
	if rateLimitDisabled {
		return
	}
	if err := a.store.RateFail(key, loginWindow); err != nil {
		logx.Infof("rate fail record %s: %v", key, err)
	}
}

func (a *API) rateReset(key string) {
	if err := a.store.RateReset(key); err != nil {
		logx.Infof("rate reset %s: %v", key, err)
	}
}

// trustedProxies 允许提供 X-Forwarded-For 的反代地址（GITDASH_TRUSTED_PROXIES，逗号分隔 IP/CIDR）。
// 每次读取（便于测试 t.Setenv）；未配置时仅信任回环（单机反代的常见形态）。
func trustedProxies() []netip.Prefix {
	var out []netip.Prefix
	for _, p := range strings.Split(os.Getenv("GITDASH_TRUSTED_PROXIES"), ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if strings.Contains(p, "/") {
			if pr, err := netip.ParsePrefix(p); err == nil {
				out = append(out, pr.Masked())
			}
		} else if a, err := netip.ParseAddr(p); err == nil {
			out = append(out, netip.PrefixFrom(a, a.BitLen()))
		}
	}
	return out
}

func isTrustedProxy(addr netip.Addr) bool {
	proxies := trustedProxies()
	if len(proxies) == 0 {
		return addr.IsLoopback()
	}
	for _, p := range proxies {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

// trustedProxyRequest 判断直连方是否为受信反代，据此才信任 X-Forwarded-*。
func trustedProxyRequest(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	addr, err := netip.ParseAddr(host)
	return err == nil && isTrustedProxy(addr)
}

// clientIP 仅在直连地址是受信反代时才信任 X-Forwarded-For，避免伪造头部绕过限流。
//
// 注意：这里取 XFF 最左值（首个地址），其语义是“最初的客户端”，但该值由客户端可伪造。
// 因此要求受信反代必须 **重写/剥离** 外部传入的 X-Forwarded-For（例如 nginx 用
// `proxy_set_header X-Forwarded-For $remote_addr;`，而非 `$proxy_add_x_forwarded_for`），
// 只追加真实的直连地址。若代理原样透传外部头部，攻击者可伪造最左 IP，从而绕过
// PAT IP allow-list 与登录限流。详见 README 的 `GITDASH_TRUSTED_PROXIES`。
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if addr, err := netip.ParseAddr(host); err == nil && isTrustedProxy(addr) {
		if h := r.Header.Get("X-Forwarded-For"); h != "" {
			if i := strings.IndexByte(h, ','); i > 0 {
				return strings.TrimSpace(h[:i])
			}
			return strings.TrimSpace(h)
		}
	}
	return host
}

const sessionCookie = "gitdash_session"

// forceSecureCookies 为反向代理终止 TLS 的部署提供的显式开关（此时 r.TLS 为 nil）。
var forceSecureCookies = os.Getenv("GITDASH_SECURE_COOKIES") != ""

func (a *API) setSessionCookie(w http.ResponseWriter, r *http.Request, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil || forceSecureCookies,
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

// rawAuthToken 处理不用 "Bearer " 前缀、直接放裸 token 的客户端（如 cargo 的
// cargo:token 凭据提供者），使其 PAT 能被识别。
func rawAuthToken(r *http.Request) string {
	h := strings.TrimSpace(r.Header.Get("Authorization"))
	if h == "" || strings.HasPrefix(strings.ToLower(h), "bearer ") ||
		strings.HasPrefix(strings.ToLower(h), "basic ") {
		return ""
	}
	return h
}

// requestSessionToken 提取请求携带的凭据（Bearer / 裸 token / 会话 cookie），
// 供「保留当前会话、撤销其它会话」使用（如改密）。
func requestSessionToken(r *http.Request) string {
	if t := bearerToken(r); t != "" {
		return t
	}
	if t := rawAuthToken(r); t != "" {
		return t
	}
	if c, err := r.Cookie(sessionCookie); err == nil {
		return c.Value
	}
	return ""
}

// resolveUser 解析请求身份：Bearer/cookie/裸 token → 登录 session；否则尝试 PAT；
// 再否则尝试 Basic（密码作为 PAT 或 session token，用户名须与 token 归属一致）。
// 返回 (username, scopes, isPAT)；未认证返回 ("", nil, false)。
func (a *API) resolveUser(r *http.Request) (string, []string, bool) {
	tok := bearerToken(r)
	if tok == "" {
		tok = rawAuthToken(r)
	}
	if tok == "" {
		if c, err := r.Cookie(sessionCookie); err == nil {
			tok = c.Value
		}
	}
	// 封禁用户视为未认证：禁止登录与一切 API/SSH 使用（现有会话/PAT 亦失效）。
	banned := func(name string) bool { return name != "" && a.store.IsUserBanned(name) }
	if tok == "" {
		if user, pass, ok := r.BasicAuth(); ok && pass != "" {
			if name, scopes, err := a.store.ValidatePAT(pass, clientIP(r)); err == nil {
				if user == "" || user == name {
					if banned(name) {
						return "", nil, false
					}
					return name, scopes, true
				}
				return "", nil, false
			}
			if name, err := a.store.GetSession(pass); err == nil {
				if user == "" || user == name {
					if banned(name) {
						return "", nil, false
					}
					return name, nil, false
				}
			}
			return "", nil, false
		}
		return "", nil, false
	}
	username, err := a.store.GetSession(tok)
	if err == nil {
		if banned(username) {
			return "", nil, false
		}
		return username, nil, false
	}
	if name, scopes, err := a.store.ValidatePAT(tok, clientIP(r)); err == nil {
		if banned(name) {
			return "", nil, false
		}
		return name, scopes, true
	}
	return "", nil, false
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
		internalError(w, err)
		return
	}
	if ua.Banned {
		writeCode(w, http.StatusForbidden, "account_banned", "account is banned")
		return
	}
	token, err := newSessionToken()
	if err != nil {
		internalError(w, err)
		return
	}
	if err := a.store.CreateSession(token, ua.ID); err != nil {
		internalError(w, err)
		return
	}
	a.setSessionCookie(w, r, token)
	resp := map[string]string{"username": ua.Username}
	// 会话 token 默认经 HttpOnly cookie 下发；仅非浏览器客户端（CLI/测试，
	// 不带 Sec-Fetch-*）或显式请求时回传明文，避免 XSS 从 JSON 响应体绕过
	// HttpOnly 读取 token（安全审计 M2.3）。
	if wantBearerToken(r) {
		resp["token"] = token
	}
	writeJSON(w, status, resp)
}

// wantBearerToken 报告响应是否附带明文会话 token：
//   - 显式请求（X-Gitdash-Return-Token: 1 或 ?return_token=1）→ 是；
//   - 浏览器请求（fetch/XHR 会带 Sec-Fetch-*）→ 否；
//   - 其他（CLI/SDK/测试）→ 是（兼容既有客户端）。
func wantBearerToken(r *http.Request) bool {
	if r.Header.Get("X-Gitdash-Return-Token") == "1" {
		return true
	}
	switch strings.ToLower(r.URL.Query().Get("return_token")) {
	case "1", "true", "yes":
		return true
	}
	if r.Header.Get("Sec-Fetch-Site") != "" || r.Header.Get("Sec-Fetch-Mode") != "" {
		return false
	}
	return true
}

// beginMFAChallenge 在用户启用 MFA 时创建一个一次性挑战。required=false 表示
// 无需二次验证，调用方应直接签发会话。这是社交登录 / passkey 登录与密码登录
// 共用的 MFA 入口，避免“旁路登录跳过 MFA”（安全审计 A2）。
func (a *API) beginMFAChallenge(username string) (token, method string, required bool, err error) {
	ua, err := a.store.GetByUsername(username)
	if err != nil {
		return "", "", false, err
	}
	if !ua.MFAEnabled {
		return "", "", false, nil
	}
	method = ua.MFAMethod
	if method == "" {
		method = "totp"
	}
	token, err = newSessionToken()
	if err != nil {
		return "", "", false, err
	}
	expires := time.Now().Add(10 * time.Minute).UTC().Format(time.RFC3339)
	if err := a.store.PutMFAChallenge(token, ua.Username, expires); err != nil {
		return "", "", false, err
	}
	if method == "email" {
		if !a.emailReady() {
			_ = a.store.DeleteMFAChallenge(token)
			return "", "", false, errEmailMFAUnavailable
		}
		if err := a.issueEmailMFACode(token, ua.Username, ua.Email, "gitdash: sign-in verification code"); err != nil {
			_ = a.store.DeleteMFAChallenge(token)
			return "", "", false, err
		}
	}
	return token, method, true, nil
}

// oauthIssueSession 完成社交登录：用户启用 MFA 时必须先完成二次验证，否则
// 只重定向回登录页并带上 mfa_token/mfa_method（由前端继续校验）。
func (a *API) oauthIssueSession(w http.ResponseWriter, r *http.Request, username string) {
	ua, err := a.store.GetByUsername(username)
	if err != nil || ua.Banned {
		http.Redirect(w, r, "/login?error=oauth_failed", http.StatusFound)
		return
	}
	token, method, required, err := a.beginMFAChallenge(username)
	if err != nil {
		reason := "oauth_failed"
		if errors.Is(err, errEmailMFAUnavailable) {
			reason = "mfa_unavailable"
		}
		http.Redirect(w, r, "/login?error="+reason, http.StatusFound)
		return
	}
	if required {
		q := url.Values{}
		q.Set("mfa_token", token)
		q.Set("mfa_method", method)
		http.Redirect(w, r, "/login?"+q.Encode(), http.StatusFound)
		return
	}
	sess, err := newSessionToken()
	if err == nil {
		if err := a.store.CreateSession(sess, ua.ID); err == nil {
			a.setSessionCookie(w, r, sess)
		}
	}
	http.Redirect(w, r, "/", http.StatusFound)
}

// issueSessionOrMFA 供 JSON 登录流程（如 passkey）使用：MFA 已启用则返回
// mfa_required 挑战，否则直接签发会话。
func (a *API) issueSessionOrMFA(w http.ResponseWriter, r *http.Request, status int, username string) {
	token, method, required, err := a.beginMFAChallenge(username)
	if err != nil {
		if errors.Is(err, errEmailMFAUnavailable) {
			writeCode(w, http.StatusServiceUnavailable, "mfa_unavailable", "email mfa requires SMTP to be configured")
			return
		}
		internalError(w, err)
		return
	}
	if required {
		writeJSON(w, status, map[string]any{"mfa_required": true, "mfa_token": token, "mfa_method": method})
		return
	}
	a.startSession(w, r, status, username)
}

// ---- admin & oauth providers ----

const adminCookie = "gitdash_admin"
