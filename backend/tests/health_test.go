package tests

import (
	"encoding/json"
	"net/http"
	"testing"
)

// healthBody 读取 /api/health 的 JSON body 与状态码。
func healthBody(t *testing.T, url string) (int, map[string]string) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	var m map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&m)
	return resp.StatusCode, m
}

func TestHealthReadiness(t *testing.T) {
	env := start(t)

	code, body := healthBody(t, env.BaseURL+"/api/health")
	if code != http.StatusOK || body["status"] != "ok" {
		t.Fatalf("/api/health = %d %v, want 200 ok", code, body)
	}

	// liveness 不依赖 DB，正常时也应 200
	code, body = healthBody(t, env.BaseURL+"/api/health/live")
	if code != http.StatusOK || body["status"] != "ok" {
		t.Fatalf("/api/health/live = %d %v, want 200 ok", code, body)
	}
}

// DB 不可达时 readiness 必须失败（503），liveness 仍存活（200）。
func TestHealthReadinessFailsWhenDBClosed(t *testing.T) {
	env := start(t)

	sqlDB, err := env.Store.DB().DB()
	if err != nil {
		t.Fatalf("get sql.DB: %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}

	code, body := healthBody(t, env.BaseURL+"/api/health")
	if code != http.StatusServiceUnavailable || body["status"] != "unavailable" {
		t.Fatalf("/api/health after db close = %d %v, want 503 unavailable", code, body)
	}

	code, body = healthBody(t, env.BaseURL+"/api/health/live")
	if code != http.StatusOK || body["status"] != "ok" {
		t.Fatalf("/api/health/live after db close = %d %v, want 200 ok", code, body)
	}
}
