package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"gitdash/backend/internal/store"
)

func newOAuthAPI(t *testing.T) (*API, *store.Store, store.OAuthAppSecret) {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateUser("alice", "alice-pass-123"); err != nil {
		t.Fatal(err)
	}
	uid, err := s.UserID("alice")
	if err != nil {
		t.Fatal(err)
	}
	app, err := s.CreateOAuthApp(uid, "Test App", "https://app.example", "desc", "https://app.example/cb")
	if err != nil {
		t.Fatal(err)
	}
	return New(s, "test"), s, app
}

func (a *API) sessionCookie(t *testing.T, s *store.Store) *http.Cookie {
	t.Helper()
	uid, err := s.UserID("alice")
	if err != nil {
		t.Fatal(err)
	}
	token, err := newSessionToken()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CreateSession(token, uid); err != nil {
		t.Fatal(err)
	}
	return &http.Cookie{Name: sessionCookie, Value: token}
}

func TestOAuthAuthorizeFullFlow(t *testing.T) {
	a, s, app := newOAuthAPI(t)
	handler := a.Handler("")

	// 1) 未登录 → 回首页
	res := mustGet(t, handler, "/login/oauth/authorize?client_id="+app.ClientID+
		"&redirect_uri="+url.QueryEscape(app.CallbackURL)+"&scope=repo&state=xyz&response_type=code", nil)
	if res.Code != http.StatusFound || !strings.HasSuffix(res.Header().Get("Location"), "/") {
		t.Fatalf("unauthenticated authorize: status=%d loc=%s", res.Code, res.Header().Get("Location"))
	}

	// 2) 已登录 → consent 页
	res = mustGet(t, handler, "/login/oauth/authorize?client_id="+app.ClientID+
		"&redirect_uri="+url.QueryEscape(app.CallbackURL)+"&scope=repo&state=xyz&response_type=code",
		[]*http.Cookie{a.sessionCookie(t, s)})
	if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), "Authorize") {
		t.Fatalf("consent page: status=%d body=%s", res.Code, res.Body.String())
	}

	// 3) 批准 → 回跳 callback 携带 code
	form := url.Values{
		"client_id":     {app.ClientID},
		"redirect_uri":  {app.CallbackURL},
		"scope":         {"repo"},
		"state":         {"xyz"},
		"response_type": {"code"},
		"action":        {"approve"},
	}
	req := httptest.NewRequest(http.MethodPost, "/login/oauth/authorize", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(a.sessionCookie(t, s))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("approve: status=%d body=%s", rr.Code, rr.Body.String())
	}
	loc, _ := url.Parse(rr.Header().Get("Location"))
	if loc.Query().Get("state") != "xyz" || loc.Query().Get("code") == "" {
		t.Fatalf("approve redirect: %s", rr.Header().Get("Location"))
	}
	code := loc.Query().Get("code")

	// 4) code → access_token
	token := exchangeCode(t, handler, app.ClientID, app.ClientSecret, code, app.CallbackURL)

	// 5) access_token 调用受保护 API
	req = httptest.NewRequest(http.MethodGet, "/api/repos", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("repos with oauth token: status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestOAuthAuthorizeRejectsBadInput(t *testing.T) {
	a, _, app := newOAuthAPI(t)
	handler := a.Handler("")

	// response_type 非法 → 回跳 callback 带 error
	res := mustGet(t, handler, "/login/oauth/authorize?client_id="+app.ClientID+
		"&redirect_uri="+url.QueryEscape(app.CallbackURL)+"&response_type=token&state=s", nil)
	loc, _ := url.Parse(res.Header().Get("Location"))
	if res.Code != http.StatusFound || loc.Query().Get("error") != "unsupported_response_type" {
		t.Fatalf("bad response_type: status=%d loc=%s", res.Code, res.Header().Get("Location"))
	}

	// redirect_uri 不匹配 → 400（不回跳到攻击者 URL）
	res = mustGet(t, handler, "/login/oauth/authorize?client_id="+app.ClientID+
		"&redirect_uri=https://evil.example&response_type=code", nil)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("redirect_uri mismatch: status=%d", res.Code)
	}

	// 未知 client_id → 400
	res = mustGet(t, handler, "/login/oauth/authorize?client_id=nope&redirect_uri="+
		url.QueryEscape(app.CallbackURL)+"&response_type=code", nil)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("unknown client: status=%d", res.Code)
	}
}

func TestOAuthAccessTokenValidation(t *testing.T) {
	a, s, app := newOAuthAPI(t)
	handler := a.Handler("")

	// 先走一次完整授权拿到 code
	form := url.Values{
		"client_id":     {app.ClientID},
		"redirect_uri":  {app.CallbackURL},
		"scope":         {"repo"},
		"state":         {"s"},
		"response_type": {"code"},
		"action":        {"approve"},
	}
	req := httptest.NewRequest(http.MethodPost, "/login/oauth/authorize", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(a.sessionCookie(t, s))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	loc, _ := url.Parse(rr.Header().Get("Location"))
	code := loc.Query().Get("code")

	// 错误 client_secret → 401
	req = httptest.NewRequest(http.MethodPost, "/login/oauth/access_token",
		strings.NewReader(url.Values{
			"grant_type":    {"authorization_code"},
			"client_id":     {app.ClientID},
			"client_secret": {"wrong"},
			"code":          {code},
			"redirect_uri":  {app.CallbackURL},
		}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("bad secret: status=%d", rr.Code)
	}

	// code 重放 → invalid_grant
	exchangeCode(t, handler, app.ClientID, app.ClientSecret, code, app.CallbackURL)
	rr = postToken(t, handler, app.ClientID, app.ClientSecret, code, app.CallbackURL)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("replay code: status=%d body=%s", rr.Code, rr.Body.String())
	}
}

// ---- helpers ----

func mustGet(t *testing.T, handler http.Handler, path string, cookies []*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr
}

func exchangeCode(t *testing.T, handler http.Handler, clientID, clientSecret, code, redirectURI string) string {
	t.Helper()
	rr := postToken(t, handler, clientID, clientSecret, code, redirectURI)
	if rr.Code != http.StatusOK {
		t.Fatalf("token exchange: status=%d body=%s", rr.Code, rr.Body.String())
	}
	var out struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		Scope       string `json:"scope"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.AccessToken == "" || out.TokenType != "bearer" || out.Scope != "repo" {
		t.Fatalf("token response: %+v", out)
	}
	return out.AccessToken
}

func postToken(t *testing.T, handler http.Handler, clientID, clientSecret, code, redirectURI string) *httptest.ResponseRecorder {
	t.Helper()
	body := url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"code":          {code},
		"redirect_uri":  {redirectURI},
	}.Encode()
	req := httptest.NewRequest(http.MethodPost, "/login/oauth/access_token", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr
}
