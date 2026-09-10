package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"strings"
)

// Google 登录（OAuth 2.0 + OpenID Connect）：由管理员在管理面板开启并填入
// OAuth 客户端 ID / Secret。端点默认指向 Google 官方地址，可用环境变量覆盖
// 以便测试指向本地假 IdP（GITDASH_GOOGLE_AUTH_URL / _TOKEN_URL / _USERINFO_URL）。

const (
	googleAuthURLDefault     = "https://accounts.google.com/o/oauth2/v2/auth"
	googleTokenURLDefault    = "https://oauth2.googleapis.com/token"
	googleUserinfoURLDefault = "https://openidconnect.googleapis.com/v1/userinfo"
)

func envOr(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func googleAuthURL() string  { return envOr("GITDASH_GOOGLE_AUTH_URL", googleAuthURLDefault) }
func googleTokenURL() string { return envOr("GITDASH_GOOGLE_TOKEN_URL", googleTokenURLDefault) }
func googleUserinfoURL() string {
	return envOr("GITDASH_GOOGLE_USERINFO_URL", googleUserinfoURLDefault)
}

func (a *API) googleSettings() (enabled bool, clientID, clientSecret string) {
	return a.store.GetSetting("google_oauth_enabled") == "1",
		a.store.GetSetting("google_client_id"),
		a.store.GetSetting("google_client_secret")
}

func (a *API) googleStart(w http.ResponseWriter, r *http.Request) {
	enabled, id, _ := a.googleSettings()
	if !enabled || id == "" {
		writeCode(w, http.StatusNotFound, "oauth_disabled", "google login is not enabled")
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
	q.Set("response_type", "code")
	q.Set("scope", "openid email profile")
	q.Set("redirect_uri", reqBase(r)+"/api/auth/google/callback")
	q.Set("state", state)
	// 已登录多个 Google 账号时让用户选择
	q.Set("prompt", "select_account")
	http.Redirect(w, r, googleAuthURL()+"?"+q.Encode(), http.StatusFound)
}

type googleUser struct {
	Sub           string `json:"sub"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Name          string `json:"name"`
}

func (a *API) googleCallback(w http.ResponseWriter, r *http.Request) {
	enabled, id, secret := a.googleSettings()
	fail := func(msg string) {
		http.Redirect(w, r, "/?auth_error="+url.QueryEscape(msg), http.StatusFound)
	}
	if !enabled || id == "" || secret == "" {
		fail("google login is not enabled")
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
	tf := url.Values{}
	tf.Set("grant_type", "authorization_code")
	tf.Set("code", code)
	tf.Set("redirect_uri", reqBase(r)+"/api/auth/google/callback")
	tf.Set("client_id", id)
	tf.Set("client_secret", secret)
	treq, _ := http.NewRequest(http.MethodPost, googleTokenURL(), strings.NewReader(tf.Encode()))
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
	// 拉取用户资料（Google 的 sub 为稳定唯一 ID）
	ureq, _ := http.NewRequest(http.MethodGet, googleUserinfoURL(), nil)
	ureq.Header.Set("Authorization", "Bearer "+tok.AccessToken)
	ures, err := http.DefaultClient.Do(ureq)
	if err != nil {
		fail("fetch google user failed")
		return
	}
	defer func() { _ = ures.Body.Close() }()
	if ures.StatusCode != http.StatusOK {
		fail("google user fetch failed")
		return
	}
	var gu googleUser
	if err := json.NewDecoder(ures.Body).Decode(&gu); err != nil || gu.Sub == "" {
		fail("invalid google user")
		return
	}
	loginHint := gu.Email
	if loginHint == "" {
		loginHint = gu.Name
	}
	username, err := a.loginOrCreateOAuthUser(r, "google", gu.Sub, loginHint)
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

// sanitizeOAuthLogin 由第三方资料（邮箱 / 昵称）推断合法用户名：
// 取邮箱 @ 前的部分，非法字符替换为 '-'，仍不合法时回退为 g<externalID>。
func sanitizeOAuthLogin(login, externalID string) string {
	u := strings.ToLower(strings.TrimSpace(login))
	if i := strings.IndexByte(u, '@'); i > 0 {
		u = u[:i]
	}
	var b strings.Builder
	for _, r := range u {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	u = strings.Trim(b.String(), "-_")
	if len(u) > 32 {
		u = u[:32]
	}
	u = strings.TrimRight(u, "-_")
	if !usernameRe.MatchString(u) {
		u = "g" + externalID
		if len(u) > 32 {
			u = u[:32]
		}
	}
	return u
}
