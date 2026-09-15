package api

import (
	"errors"
	"html/template"
	"net/http"
	"net/url"
	"strings"

	"gitdash/backend/internal/store"
)

// ---- OAuth 2.0 设备流（RFC 8628）：供 CLI 等无浏览器环境登录 ----

func (a *API) oauthDeviceCode(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_request", "invalid form body")
		return
	}
	clientID := r.PostFormValue("client_id")
	if clientID != store.OAuthFirstPartyClientID {
		writeCode(w, http.StatusBadRequest, "unauthorized_client", "unsupported client_id")
		return
	}
	scopes, ok := oauthScopeList(r.PostFormValue("scope"))
	if !ok {
		writeCode(w, http.StatusBadRequest, "invalid_scope", "invalid scope")
		return
	}
	deviceCode, userCode, err := a.store.CreateDeviceGrant(clientID, scopes)
	if err != nil {
		internalError(w, err)
		return
	}
	base := reqBase(r)
	verifyURI := base + "/login/oauth/device"
	writeJSON(w, http.StatusOK, map[string]any{
		"device_code":               deviceCode,
		"user_code":                 userCode,
		"verification_uri":          verifyURI,
		"verification_uri_complete": verifyURI + "?user_code=" + url.QueryEscape(userCode),
		"expires_in":                int(oauthDeviceCodeTTLSeconds),
		"interval":                  5,
	})
}

const oauthDeviceCodeTTLSeconds = 15 * 60

// oauthDeviceVerify 浏览器验证页：输入/确认 user_code 并批准或拒绝。
func (a *API) oauthDeviceVerify(w http.ResponseWriter, r *http.Request) {
	username := a.sessionUser(r)
	if username == "" {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	userCode := strings.TrimSpace(strings.ToUpper(r.FormValue("user_code")))
	if userCode == "" {
		a.renderDevicePage(w, devicePageData{Mode: "entry"})
		return
	}

	grant, err := a.store.GetDeviceGrantByUserCode(userCode)
	if err != nil {
		a.renderDevicePage(w, devicePageData{Mode: "error", Message: "Unknown or expired code."})
		return
	}

	if r.Method == http.MethodPost {
		uid, uerr := a.store.UserID(username)
		if uerr != nil {
			a.renderDevicePage(w, devicePageData{Mode: "error", Message: "Failed to resolve account."})
			return
		}
		if r.FormValue("action") == "deny" {
			_ = a.store.DenyDeviceGrant(userCode)
			a.renderDevicePage(w, devicePageData{Mode: "denied"})
			return
		}
		if err := a.store.ApproveDeviceGrant(userCode, uid); err != nil {
			a.renderDevicePage(w, devicePageData{Mode: "error", Message: "Code is no longer pending."})
			return
		}
		a.renderDevicePage(w, devicePageData{Mode: "approved"})
		return
	}

	// GET：按状态展示
	switch grant.Status {
	case "approved":
		a.renderDevicePage(w, devicePageData{Mode: "approved"})
	case "denied":
		a.renderDevicePage(w, devicePageData{Mode: "denied"})
	default:
		a.renderDevicePage(w, devicePageData{
			Mode:     "consent",
			UserCode: userCode,
			Scopes:   scopeLabels(strings.Fields(strings.ReplaceAll(grant.Scopes, ",", " "))),
		})
	}
}

// oauthDeviceToken 处理 grant_type=urn:ietf:params:oauth:grant-type:device_code 的轮询。
func (a *API) oauthDeviceToken(w http.ResponseWriter, r *http.Request) {
	clientID := r.PostFormValue("client_id")
	deviceCode := r.PostFormValue("device_code")
	if clientID != store.OAuthFirstPartyClientID {
		writeCode(w, http.StatusBadRequest, "unauthorized_client", "unsupported client_id")
		return
	}
	grant, err := a.store.ConsumeDeviceGrant(deviceCode)
	switch {
	case errors.Is(err, store.ErrDevicePending):
		writeCode(w, http.StatusBadRequest, "authorization_pending", "the user has not completed authorization yet")
		return
	case errors.Is(err, store.ErrDeviceDenied):
		writeCode(w, http.StatusBadRequest, "access_denied", "the user denied the request")
		return
	case errors.Is(err, store.ErrDeviceExpired):
		writeCode(w, http.StatusBadRequest, "expired_token", "the device code has expired")
		return
	case err != nil:
		writeCode(w, http.StatusBadRequest, "invalid_grant", "invalid device_code")
		return
	}
	app, err := a.store.GetOAuthAppByClientID(clientID)
	if err != nil {
		internalError(w, err)
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

type devicePageData struct {
	Mode     string // entry | consent | approved | denied | error
	UserCode string
	Scopes   []scopeLabel
	Message  string
}

var deviceTmpl = template.Must(template.New("device").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>gitdash Device Authorization</title>
<style>
  :root { color-scheme: light dark; }
  body { font-family: system-ui, -apple-system, sans-serif; margin: 0; background: #f6f8fa; color: #1f2328; }
  .wrap { max-width: 440px; margin: 8vh auto; padding: 0 16px; }
  .card { background: #fff; border: 1px solid #d0d7de; border-radius: 12px; padding: 24px; box-shadow: 0 1px 3px rgba(0,0,0,.06); }
  h1 { font-size: 20px; margin: 0 0 8px; }
  p { font-size: 14px; color: #59636e; }
  input { width: 100%; box-sizing: border-box; padding: 10px; border-radius: 8px; border: 1px solid #d0d7de; font-size: 16px; text-transform: uppercase; letter-spacing: 2px; }
  .actions { display: flex; gap: 8px; margin-top: 16px; }
  button { flex: 1; padding: 10px; border-radius: 8px; border: 1px solid #d0d7de; background: #f6f8fa; font-size: 14px; cursor: pointer; }
  button.primary { background: #1f883d; border-color: #1f883d; color: #fff; font-weight: 600; }
  ul { list-style: none; padding: 0; }
  li { padding: 6px 0; font-size: 14px; border-bottom: 1px solid #eaeef2; }
  @media (prefers-color-scheme: dark) {
    body { background: #0d1117; color: #e6edf3; }
    .card { background: #161b22; border-color: #30363d; }
    p { color: #8b949e; }
    input, button { background: #21262d; border-color: #30363d; color: #e6edf3; }
    button.primary { background: #238636; border-color: #238636; color: #fff; }
    li { border-color: #21262d; }
  }
</style>
</head>
<body>
<div class="wrap"><div class="card">
{{if eq .Mode "entry"}}
  <h1>Device Authorization</h1>
  <p>Enter the code shown in your terminal.</p>
  <form method="get" action="/login/oauth/device">
    <input name="user_code" placeholder="ABCD-EFGH" autocomplete="off" autofocus>
    <div class="actions"><button type="submit" class="primary">Continue</button></div>
  </form>
{{else if eq .Mode "consent"}}
  <h1>Authorize gitdash CLI</h1>
  <p>This will grant the following access to the device using code <b>{{.UserCode}}</b>:</p>
  <ul>{{range .Scopes}}<li><b>{{.Name}}</b> — {{.Desc}}</li>{{end}}</ul>
  <form method="post" action="/login/oauth/device">
    <input type="hidden" name="user_code" value="{{.UserCode}}">
    <div class="actions">
      <button type="submit" name="action" value="deny">Deny</button>
      <button type="submit" name="action" value="approve" class="primary">Authorize</button>
    </div>
  </form>
{{else if eq .Mode "approved"}}
  <h1>Device authorized</h1>
  <p>You can close this window and return to your terminal.</p>
{{else if eq .Mode "denied"}}
  <h1>Request denied</h1>
  <p>The device was not authorized. You can close this window.</p>
{{else}}
  <h1>Something went wrong</h1>
  <p>{{.Message}}</p>
{{end}}
</div></div>
</body>
</html>
`))

func (a *API) renderDevicePage(w http.ResponseWriter, data devicePageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = deviceTmpl.Execute(w, data)
}
