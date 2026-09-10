package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"gitdash/backend/internal/store"
	"net/http"
	"net/url"
	"os"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

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
//
//	@Summary     列出第三方登录方式
//	@Description 返回 github 与 oidc 的启用状态及授权 URL（未启用不暴露配置）。
//	@Tags        auth
//	@Produce     json
//	@Success     200 {object} map[string]any
//	@Router      /auth/providers [get]
func (a *API) providers(w http.ResponseWriter, r *http.Request) {
	resp := map[string]any{
		"github": map[string]any{"enabled": false},
		"google": map[string]any{"enabled": false},
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
	if enabled, id, _ := a.googleSettings(); enabled && id != "" {
		resp["google"] = map[string]any{
			"enabled":       true,
			"name":          "Google",
			"authorize_url": reqBase(r) + "/api/auth/google",
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
		internalError(w, err)
		return
	}
	if err := a.saveOAuthState(state); err != nil {
		internalError(w, err)
		return
	}
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
	if !a.checkOAuthState(state) {
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
	defer func() { _ = tres.Body.Close() }()
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
	defer func() { _ = ures.Body.Close() }()
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
		if errors.Is(err, errOAuthLinkRequired) {
			fail("account_exists_link_required")
		} else {
			fail("account provisioning failed")
		}
		return
	}
	a.oauthIssueSession(w, r, username)
}

// errOAuthLinkRequired 同名本地账号存在但当前请求无法证明其控制权，拒绝自动绑定。
var errOAuthLinkRequired = errors.New("oauth_link_required")

func (a *API) loginOrCreateOAuthUser(r *http.Request, provider, externalID, login string) (string, error) {
	if _, username, err := a.store.OAuthUser(provider, externalID); err == nil {
		return username, nil
	}
	// 未绑定：按 provider login 关联或新建账号
	username := sanitizeOAuthLogin(login, externalID)
	if _, err := a.store.GetByUsername(username); err == nil {
		// 存在同名本地账号：仅当请求携带该账号的有效会话/PAT（即调用者已证明
		// 对该账号的控制权）时才绑定，否则拒绝，防止劫持他人本地账号。
		if me, _, _ := a.resolveUser(r); me != username {
			return "", errOAuthLinkRequired
		}
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
	hash, err := bcrypt.GenerateFromPassword([]byte(randPass), BcryptCost)
	if err != nil {
		return "", err
	}
	u, err := a.store.CreateUser(username, string(hash))
	if err != nil {
		if errors.Is(err, store.ErrExists) { // 竞态：同上，需先证明控制权
			if me, _, _ := a.resolveUser(r); me != username {
				return "", errOAuthLinkRequired
			}
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

func randomPassword() string {
	b := make([]byte, 18)
	if _, err := rand.Read(b); err != nil {
		return "fallback-rand-pass"
	}
	return hex.EncodeToString(b)
}
