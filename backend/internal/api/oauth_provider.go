package api

import (
	"errors"
	"html/template"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"gitdash/backend/internal/store"
)

// ---- OAuth 2.0 provider（gitdash 作为授权服务器，供第三方应用接入）----
//
// 授权码流程：
//   1. 用户在「OAuth 应用」页注册应用，拿到 client_id / client_secret。
//   2. 第三方把用户跳转到 GET /login/oauth/authorize?client_id&redirect_uri&scope&state。
//   3. 用户登录并授权后，gitdash 重定向回 redirect_uri 并携带一次性 code。
//   4. 第三方用 POST /login/oauth/access_token（client_secret + code）换取 access_token。
//   5. access_token 作为 Bearer 调用 /api 下受保护接口（scope 与 PAT 相同：repo/inbox/keys）。

// sessionUser 仅通过会话 cookie 解析登录用户（授权页不认 Bearer/PAT）。
func (a *API) sessionUser(r *http.Request) string {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return ""
	}
	name, err := a.store.GetSession(c.Value)
	if err != nil {
		return ""
	}
	return name
}

// ---- 应用管理（用户侧）----

type createOAuthAppReq struct {
	Name        string `json:"name"`
	Homepage    string `json:"homepage"`
	Description string `json:"description"`
	CallbackURL string `json:"callback_url"`
}

func (a *API) listOAuthApps(w http.ResponseWriter, r *http.Request) {
	uid, err := a.store.UserID(userFrom(r))
	if err != nil {
		internalError(w, err)
		return
	}
	apps, err := a.store.ListOAuthApps(uid)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, apps)
}

func (a *API) createOAuthApp(w http.ResponseWriter, r *http.Request) {
	var in createOAuthAppReq
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Homepage = strings.TrimSpace(in.Homepage)
	in.Description = strings.TrimSpace(in.Description)
	in.CallbackURL = strings.TrimSpace(in.CallbackURL)
	if in.Name == "" {
		writeCode(w, http.StatusBadRequest, "oauth_app_name_required", "name is required")
		return
	}
	if len(in.Name) > 120 {
		writeCode(w, http.StatusBadRequest, "oauth_app_name_too_long", "name must be at most 120 characters")
		return
	}
	if !validCallbackURL(in.CallbackURL) {
		writeCode(w, http.StatusBadRequest, "invalid_callback_url", "callback_url must be an http(s) URL")
		return
	}
	uid, err := a.store.UserID(userFrom(r))
	if err != nil {
		internalError(w, err)
		return
	}
	app, err := a.store.CreateOAuthApp(uid, in.Name, in.Homepage, in.Description, in.CallbackURL)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, app)
}

func (a *API) deleteOAuthApp(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_id", "invalid id")
		return
	}
	uid, err := a.store.UserID(userFrom(r))
	if err != nil {
		internalError(w, err)
		return
	}
	if errors.Is(a.store.DeleteOAuthApp(uid, id), store.ErrNotFound) {
		writeNotFound(w, "oauth_app")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) resetOAuthAppSecret(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_id", "invalid id")
		return
	}
	uid, err := a.store.UserID(userFrom(r))
	if err != nil {
		internalError(w, err)
		return
	}
	secret, err := a.store.ResetOAuthAppSecret(uid, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeNotFound(w, "oauth_app")
			return
		}
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"client_secret": secret})
}

// ---- 已授权应用（签发过的 token）----

func (a *API) listOAuthAuthorizations(w http.ResponseWriter, r *http.Request) {
	uid, err := a.store.UserID(userFrom(r))
	if err != nil {
		internalError(w, err)
		return
	}
	auths, err := a.store.ListOAuthAuthorizations(uid)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, auths)
}

func (a *API) revokeOAuthAuthorization(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_id", "invalid id")
		return
	}
	uid, err := a.store.UserID(userFrom(r))
	if err != nil {
		internalError(w, err)
		return
	}
	if errors.Is(a.store.RevokeOAuthAuthorization(uid, id), store.ErrNotFound) {
		writeNotFound(w, "oauth_authorization")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- 授权端点 ----

func validCallbackURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return false
	}
	return u.Scheme == "http" || u.Scheme == "https"
}

// oauthScopeList 校验 scope（空格分隔）；空 = 默认 repo。返回逗号分隔串与是否合法。
func oauthScopeList(raw string) (string, bool) {
	return store.NormalizePATScopes(strings.Fields(raw))
}

// oauthAuthorize 渲染授权确认页（GET）并处理确认/拒绝（POST）。
func (a *API) oauthAuthorize(w http.ResponseWriter, r *http.Request) {
	clientID := r.FormValue("client_id")
	redirectURI := r.FormValue("redirect_uri")
	state := r.FormValue("state")
	rawScope := r.FormValue("scope")

	// 先校验 client 与 redirect_uri，确保后续错误回跳只落到已注册的 callback（防开放重定向）。
	app, err := a.store.GetOAuthAppByClientID(clientID)
	if err != nil {
		writeCode(w, http.StatusBadRequest, "unauthorized_client", "unknown client_id")
		return
	}
	if redirectURI == "" || redirectURI != app.CallbackURL {
		writeCode(w, http.StatusBadRequest, "redirect_uri_mismatch", "redirect_uri does not match the registered callback URL")
		return
	}
	if r.FormValue("response_type") != "code" {
		a.oauthAuthorizeError(w, r, redirectURI, state, "unsupported_response_type", "response_type must be code")
		return
	}
	scopes, ok := oauthScopeList(rawScope)
	if !ok {
		a.oauthAuthorizeError(w, r, redirectURI, state, "invalid_scope", "invalid scope")
		return
	}

	username := a.sessionUser(r)
	if username == "" {
		// 未登录：回到首页登录后再重新发起授权
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	if r.Method == http.MethodPost {
		if r.FormValue("action") == "deny" {
			a.oauthRedirect(w, r, redirectURI, state, "", "access_denied", "")
			return
		}
		uid, err := a.store.UserID(username)
		if err != nil {
			a.oauthAuthorizeError(w, r, redirectURI, state, "server_error", "failed to resolve user")
			return
		}
		code, err := a.store.CreateOAuthGrant(app.ID, uid, scopes, redirectURI)
		if err != nil {
			a.oauthAuthorizeError(w, r, redirectURI, state, "server_error", "failed to issue authorization code")
			return
		}
		a.oauthRedirect(w, r, redirectURI, state, code, "", "")
		return
	}

	a.renderConsentPage(w, consentData{
		AppName:     app.Name,
		AppDesc:     app.Description,
		Scopes:      scopeLabels(strings.Fields(rawScope)),
		ClientID:    clientID,
		RedirectURI: redirectURI,
		RawScope:    rawScope,
		State:       state,
	})
}

// oauthRedirect 重定向回第三方 callback，携带 code 或 error。
func (a *API) oauthRedirect(w http.ResponseWriter, r *http.Request, redirectURI, state, code, errCode, errDesc string) {
	q := url.Values{}
	if state != "" {
		q.Set("state", state)
	}
	if code != "" {
		q.Set("code", code)
	}
	if errCode != "" {
		q.Set("error", errCode)
	}
	if errDesc != "" {
		q.Set("error_description", errDesc)
	}
	sep := "?"
	if strings.Contains(redirectURI, "?") {
		sep = "&"
	}
	http.Redirect(w, r, redirectURI+sep+q.Encode(), http.StatusFound)
}

// oauthAuthorizeError 授权阶段失败：能安全回跳到 callback 就带 error 回跳，否则直接返回错误页。
func (a *API) oauthAuthorizeError(w http.ResponseWriter, r *http.Request, redirectURI, state, code, desc string) {
	if redirectURI != "" && validCallbackURL(redirectURI) {
		a.oauthRedirect(w, r, redirectURI, state, "", code, desc)
		return
	}
	writeCode(w, http.StatusBadRequest, code, desc)
}

// ---- token 端点 ----

func (a *API) oauthAccessToken(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_request", "invalid form body")
		return
	}
	switch r.PostFormValue("grant_type") {
	case "authorization_code":
		a.oauthCodeToken(w, r)
	case "urn:ietf:params:oauth:grant-type:device_code":
		a.oauthDeviceToken(w, r)
	default:
		writeCode(w, http.StatusBadRequest, "unsupported_grant_type", "unsupported grant_type")
	}
}

func (a *API) oauthCodeToken(w http.ResponseWriter, r *http.Request) {
	clientID := r.PostFormValue("client_id")
	clientSecret := r.PostFormValue("client_secret")
	code := r.PostFormValue("code")
	redirectURI := r.PostFormValue("redirect_uri")

	app, err := a.store.GetOAuthAppByClientID(clientID)
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_client", "unknown client_id")
		return
	}
	if !a.store.VerifyOAuthAppSecret(app, clientSecret) {
		writeCode(w, http.StatusUnauthorized, "invalid_client", "invalid client_secret")
		return
	}
	grant, err := a.store.ConsumeOAuthGrant(code)
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_grant", "invalid or expired code")
		return
	}
	if grant.AppID != app.ID {
		writeCode(w, http.StatusBadRequest, "invalid_grant", "code was not issued to this client")
		return
	}
	if grant.RedirectURI != redirectURI {
		writeCode(w, http.StatusBadRequest, "invalid_grant", "redirect_uri mismatch")
		return
	}
	token, _, err := a.store.CreateOAuthPAT(grant.UserID, app.ID, "OAuth: "+app.Name, grant.Scopes)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"access_token": token,
		"token_type":   "bearer",
		"scope":        grant.Scopes,
	})
}

// ---- consent 页（服务端渲染，无需前端路由）----

type consentData struct {
	AppName     string
	AppDesc     string
	Scopes      []scopeLabel
	ClientID    string
	RedirectURI string
	RawScope    string
	State       string
}

type scopeLabel struct {
	Name string
	Desc string
}

func scopeLabels(scopes []string) []scopeLabel {
	if len(scopes) == 0 {
		scopes = []string{"repo"}
	}
	out := make([]scopeLabel, 0, len(scopes))
	for _, s := range scopes {
		switch s {
		case "repo":
			out = append(out, scopeLabel{"repo", "Read and write access to your repositories"})
		case "inbox":
			out = append(out, scopeLabel{"inbox", "Read and manage your inbox notifications"})
		case "keys":
			out = append(out, scopeLabel{"keys", "Manage your SSH and GPG keys"})
		}
	}
	return out
}

var consentTmpl = template.Must(template.New("consent").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Authorize {{.AppName}}</title>
<style>
  :root { color-scheme: light dark; }
  body { font-family: system-ui, -apple-system, sans-serif; margin: 0; background: #f6f8fa; color: #1f2328; }
  .wrap { max-width: 480px; margin: 8vh auto; padding: 0 16px; }
  .card { background: #fff; border: 1px solid #d0d7de; border-radius: 12px; padding: 24px; box-shadow: 0 1px 3px rgba(0,0,0,.06); }
  h1 { font-size: 20px; margin: 0 0 8px; }
  .muted { color: #59636e; font-size: 14px; }
  .scopes { list-style: none; padding: 0; margin: 16px 0; }
  .scopes li { padding: 8px 0; border-bottom: 1px solid #eaeef2; font-size: 14px; }
  .scopes b { display: block; }
  .actions { display: flex; gap: 8px; margin-top: 20px; }
  button { flex: 1; padding: 10px 12px; border-radius: 8px; border: 1px solid #d0d7de; background: #f6f8fa; font-size: 14px; cursor: pointer; }
  button.primary { background: #1f883d; border-color: #1f883d; color: #fff; font-weight: 600; }
  @media (prefers-color-scheme: dark) {
    body { background: #0d1117; color: #e6edf3; }
    .card { background: #161b22; border-color: #30363d; }
    .muted { color: #8b949e; }
    .scopes li { border-color: #21262d; }
    button { background: #21262d; border-color: #30363d; color: #e6edf3; }
    button.primary { background: #238636; border-color: #238636; color: #fff; }
  }
</style>
</head>
<body>
<div class="wrap">
  <div class="card">
    <h1>Authorize {{.AppName}}</h1>
    {{if .AppDesc}}<p class="muted">{{.AppDesc}}</p>{{end}}
    <p class="muted">This application wants to:</p>
    <ul class="scopes">
      {{range .Scopes}}<li><b>{{.Name}}</b><span class="muted">{{.Desc}}</span></li>{{end}}
    </ul>
    <form method="post" action="/login/oauth/authorize">
      <input type="hidden" name="client_id" value="{{.ClientID}}">
      <input type="hidden" name="redirect_uri" value="{{.RedirectURI}}">
      <input type="hidden" name="scope" value="{{.RawScope}}">
      <input type="hidden" name="state" value="{{.State}}">
      <input type="hidden" name="response_type" value="code">
      <div class="actions">
        <button type="submit" name="action" value="deny">Deny</button>
        <button type="submit" name="action" value="approve" class="primary">Authorize</button>
      </div>
    </form>
  </div>
</div>
</body>
</html>
`))

func (a *API) renderConsentPage(w http.ResponseWriter, data consentData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = consentTmpl.Execute(w, data)
}
