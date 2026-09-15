package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"gitdash/backend/internal/store"
)

func TestOAuthDeviceFlow(t *testing.T) {
	a, s, _ := newOAuthAPI(t)
	handler := a.Handler("")

	// 1) 申请设备码
	req := httptest.NewRequest(http.MethodPost, "/login/oauth/device/code",
		strings.NewReader(url.Values{"client_id": {store.OAuthFirstPartyClientID}, "scope": {"repo"}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("device/code: status=%d body=%s", rr.Code, rr.Body.String())
	}
	var dc struct {
		DeviceCode string `json:"device_code"`
		UserCode   string `json:"user_code"`
		Interval   int    `json:"interval"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &dc); err != nil {
		t.Fatal(err)
	}
	if dc.DeviceCode == "" || dc.UserCode == "" || dc.Interval <= 0 {
		t.Fatalf("unexpected device code response: %+v", dc)
	}

	// 2) 未授权时轮询 → authorization_pending
	rr = pollDevice(t, handler, dc.DeviceCode)
	if rr.Code != http.StatusBadRequest || !strings.Contains(rr.Body.String(), "authorization_pending") {
		t.Fatalf("pending poll: status=%d body=%s", rr.Code, rr.Body.String())
	}

	// 3) 用户在浏览器确认
	req = httptest.NewRequest(http.MethodPost, "/login/oauth/device",
		strings.NewReader(url.Values{"user_code": {dc.UserCode}, "action": {"approve"}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(a.sessionCookie(t, s))
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "authorized") {
		t.Fatalf("approve: status=%d body=%s", rr.Code, rr.Body.String())
	}

	// 4) 轮询拿到 token
	rr = pollDevice(t, handler, dc.DeviceCode)
	if rr.Code != http.StatusOK {
		t.Fatalf("poll after approve: status=%d body=%s", rr.Code, rr.Body.String())
	}
	var tok struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &tok); err != nil {
		t.Fatal(err)
	}
	if tok.AccessToken == "" || tok.TokenType != "bearer" {
		t.Fatalf("token response: %+v", tok)
	}

	// 5) token 可用
	req = httptest.NewRequest(http.MethodGet, "/api/repos", nil)
	req.Header.Set("Authorization", "Bearer "+tok.AccessToken)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("repos with device token: status=%d body=%s", rr.Code, rr.Body.String())
	}

	// 6) 重复轮询 → invalid_grant（一次性）
	rr = pollDevice(t, handler, dc.DeviceCode)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("replay device_code: status=%d", rr.Code)
	}
}

func TestOAuthDeviceDeny(t *testing.T) {
	a, s, _ := newOAuthAPI(t)
	handler := a.Handler("")

	req := httptest.NewRequest(http.MethodPost, "/login/oauth/device/code",
		strings.NewReader(url.Values{"client_id": {store.OAuthFirstPartyClientID}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	var dc struct {
		DeviceCode string `json:"device_code"`
		UserCode   string `json:"user_code"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &dc)

	req = httptest.NewRequest(http.MethodPost, "/login/oauth/device",
		strings.NewReader(url.Values{"user_code": {dc.UserCode}, "action": {"deny"}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(a.sessionCookie(t, s))
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	rr = pollDevice(t, handler, dc.DeviceCode)
	if rr.Code != http.StatusBadRequest || !strings.Contains(rr.Body.String(), "access_denied") {
		t.Fatalf("denied poll: status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func pollDevice(t *testing.T, handler http.Handler, deviceCode string) *httptest.ResponseRecorder {
	t.Helper()
	body := url.Values{
		"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
		"client_id":   {store.OAuthFirstPartyClientID},
		"device_code": {deviceCode},
	}.Encode()
	req := httptest.NewRequest(http.MethodPost, "/login/oauth/access_token", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr
}
