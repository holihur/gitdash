package copilot

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProviderDefaults(t *testing.T) {
	if got := EffectiveBaseURL("anthropic", ""); got != "https://api.anthropic.com" {
		t.Fatalf("anthropic base = %q", got)
	}
	if got := EffectiveModel("anthropic", ""); got != "claude-sonnet-4-5" {
		t.Fatalf("anthropic model = %q", got)
	}
	if got := EffectiveBaseURL("custom", "https://gw.example.com"); got != "https://gw.example.com" {
		t.Fatalf("explicit base = %q", got)
	}
	if got := EffectiveBaseURL("ollama", ""); got != "http://127.0.0.1:11434" {
		t.Fatalf("ollama base = %q", got)
	}
	if _, ok := Provider("openai"); ok {
		t.Fatal("openai should not be a supported provider")
	}
	if spec, ok := Provider("ollama"); !ok || spec.KeyRequired {
		t.Fatal("ollama key must be optional")
	}
}

func TestTestConnection(t *testing.T) {
	var gotPath, gotKey, gotAuth, gotVersion string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotKey = r.Header.Get("x-api-key")
		gotAuth = r.Header.Get("Authorization")
		gotVersion = r.Header.Get("anthropic-version")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"content":[]}`))
	}))
	defer srv.Close()

	if err := TestConnection(context.Background(), "compatible", srv.URL, "sk-x", "m1"); err != nil {
		t.Fatalf("TestConnection: %v", err)
	}
	if gotPath != "/v1/messages" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotKey != "sk-x" || gotAuth != "Bearer sk-x" {
		t.Fatalf("auth headers = %q / %q", gotKey, gotAuth)
	}
	if gotVersion == "" {
		t.Fatal("missing anthropic-version header")
	}
}

func TestTestConnectionError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"type":"error","error":{"type":"authentication_error","message":"bad key"}}`, http.StatusUnauthorized)
	}))
	defer srv.Close()

	err := TestConnection(context.Background(), "compatible", srv.URL, "sk-bad", "m1")
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("err = %v, want HTTP 401", err)
	}
}

func TestTestConnectionRequiresModel(t *testing.T) {
	err := TestConnection(context.Background(), "compatible", "https://gw.example.com", "sk-x", "")
	if err == nil || !strings.Contains(err.Error(), "model") {
		t.Fatalf("err = %v, want model required", err)
	}
}
