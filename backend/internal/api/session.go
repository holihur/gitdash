package api

import (
	"crypto/rand"
	"encoding/hex"
	"gitdash/backend/internal/store"
	"log"
	"net"
	"net/http"
	"net/netip"
	"os"
	"strings"
)

func (a *API) rateKey(username, ip string) string { return username + "|" + ip }

// rateLimitDisabled 测试/开发可通过 GITDASH_DISABLE_RATE_LIMIT=1 关闭限流，
// 避免黑盒测试套件（同 IP 大量注册/登录失败）误触发 429。
var rateLimitDisabled = os.Getenv("GITDASH_DISABLE_RATE_LIMIT") == "1"

// 限速记录持久化在 store（login_fails 表）：重启与多实例共享同一窗口，
// 不再有内存 map 的无限增长问题。

func (a *API) rateBlocked(key string) bool {
	if rateLimitDisabled {
		return false
	}
	blocked, err := a.store.RateBlocked(key, loginMaxFails)
	if err != nil {
		log.Printf("rate check %s: %v", key, err)
		return false
	}
	return blocked
}

func (a *API) rateFail(key string) {
	if rateLimitDisabled {
		return
	}
	if err := a.store.RateFail(key, loginWindow); err != nil {
		log.Printf("rate fail record %s: %v", key, err)
	}
}

func (a *API) rateReset(key string) {
	if err := a.store.RateReset(key); err != nil {
		log.Printf("rate reset %s: %v", key, err)
	}
}

// clientIP 仅当直连地址是回环/内网（即部署在可信反代之后）时才信任
// X-Forwarded-For，避免客户端伪造头部绕过限流。
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	addr, err := netip.ParseAddr(host)
	if err == nil && (addr.IsLoopback() || addr.IsPrivate()) {
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

// resolveUser 解析请求身份：Bearer/cookie → 登录 session；否则尝试 PAT。
// 返回 (username, scopes, isPAT)；未认证返回 ("", nil, false)。
func (a *API) resolveUser(r *http.Request) (string, []string, bool) {
	tok := bearerToken(r)
	if tok == "" {
		if c, err := r.Cookie(sessionCookie); err == nil {
			tok = c.Value
		}
	}
	if tok == "" {
		return "", nil, false
	}
	username, err := a.store.GetSession(tok)
	if err == nil {
		return username, nil, false
	}
	if name, scopes, err := a.store.ValidatePAT(tok); err == nil {
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
