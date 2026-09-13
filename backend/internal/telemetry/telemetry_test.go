package telemetry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDisabledWithoutEndpoint(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	if Enabled() {
		t.Fatal("telemetry should be disabled without endpoint")
	}
	shutdown, err := Setup(context.Background(), "gitdash", "test")
	if err != nil {
		t.Fatalf("no-op setup: %v", err)
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("no-op shutdown: %v", err)
	}
}

func TestMiddlewarePassThrough(t *testing.T) {
	called := false
	h := Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	if !called || rec.Code != http.StatusNoContent {
		t.Fatalf("middleware pass-through failed: called=%v code=%d", called, rec.Code)
	}
}
