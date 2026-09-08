package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// ---- oidc ----

type oidcDiscovery struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	UserinfoEndpoint      string `json:"userinfo_endpoint"`
}

func (a *API) oidcSettings() (enabled bool, name, issuer, clientID, clientSecret string) {
	return a.store.GetSetting("oidc_enabled") == "1",
		defaultStr(a.store.GetSetting("oidc_name"), "OIDC"),
		strings.TrimRight(a.store.GetSetting("oidc_issuer"), "/"),
		a.store.GetSetting("oidc_client_id"),
		a.store.GetSetting("oidc_client_secret")
}

func (a *API) oidcDiscover(issuer string) (*oidcDiscovery, error) {
	u := issuer + "/.well-known/openid-configuration"
	req, _ := http.NewRequest(http.MethodGet, u, nil)
	req.Header.Set("Accept", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("oidc discovery failed: %d", res.StatusCode)
	}
	var d oidcDiscovery
	if err := json.NewDecoder(res.Body).Decode(&d); err != nil {
		return nil, err
	}
	// 防混淆：校验返回 issuer 与配置一致
	if !strings.EqualFold(strings.TrimRight(d.Issuer, "/"), issuer) {
		return nil, fmt.Errorf("oidc issuer mismatch (configured %s, got %s)", issuer, d.Issuer)
	}
	return &d, nil
}

func (a *API) oidcStart(w http.ResponseWriter, r *http.Request) {
	enabled, name, issuer, id, _ := a.oidcSettings()
	if !enabled || issuer == "" || id == "" {
		writeCode(w, http.StatusNotFound, "oidc_disabled", "oidc login is not enabled")
		return
	}
	d, err := a.oidcDiscover(issuer)
	if err != nil {
		writeCode(w, http.StatusBadGateway, "oidc_discovery_failed", err.Error())
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
	q.Set("scope", "openid profile email")
	q.Set("redirect_uri", reqBase(r)+"/api/auth/oidc/callback")
	q.Set("state", state)
	_ = name
	http.Redirect(w, r, d.AuthorizationEndpoint+"?"+q.Encode(), http.StatusFound)
}

func (a *API) oidcCallback(w http.ResponseWriter, r *http.Request) {
	enabled, _, issuer, id, secret := a.oidcSettings()
	fail := func(msg string) {
		http.Redirect(w, r, "/?auth_error="+url.QueryEscape(msg), http.StatusFound)
	}
	if !enabled || issuer == "" || id == "" || secret == "" {
		fail("oidc login is not enabled")
		return
	}
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	if code == "" || state == "" {
		fail("invalid oidc response")
		return
	}
	if !a.checkOAuthState(state) {
		fail("oauth state expired, try again")
		return
	}
	d, err := a.oidcDiscover(issuer)
	if err != nil {
		fail("oidc discovery failed")
		return
	}
	// 换 token
	tf := url.Values{}
	tf.Set("grant_type", "authorization_code")
	tf.Set("code", code)
	tf.Set("redirect_uri", reqBase(r)+"/api/auth/oidc/callback")
	tf.Set("client_id", id)
	tf.Set("client_secret", secret)
	treq, _ := http.NewRequest(http.MethodPost, d.TokenEndpoint, strings.NewReader(tf.Encode()))
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
		fail("token exchange failed")
		return
	}
	// userinfo
	ureq, _ := http.NewRequest(http.MethodGet, d.UserinfoEndpoint, nil)
	ureq.Header.Set("Authorization", "Bearer "+tok.AccessToken)
	ures, err := http.DefaultClient.Do(ureq)
	if err != nil {
		fail("userinfo failed")
		return
	}
	defer func() { _ = ures.Body.Close() }()
	if ures.StatusCode != http.StatusOK {
		fail("userinfo failed")
		return
	}
	var ui struct {
		Sub               string `json:"sub"`
		Email             string `json:"email"`
		PreferredUsername string `json:"preferred_username"`
	}
	if err := json.NewDecoder(ures.Body).Decode(&ui); err != nil || ui.Sub == "" {
		fail("invalid userinfo")
		return
	}
	loginHint := ui.PreferredUsername
	if loginHint == "" && ui.Email != "" {
		loginHint = strings.Split(ui.Email, "@")[0]
	}
	username, err := a.loginOrCreateOAuthUser(r, "oidc", ui.Sub, loginHint)
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
